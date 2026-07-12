package generation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

type CreateRequest struct {
	Prompt string
	Target Target
	Locale string
}

type Service struct {
	repository Repository
	newID      func() string
	now        func() time.Time
	jobQueue   JobQueue
	streamMu   sync.Mutex
	streams    map[string]map[*eventStream]struct{}
}

type eventStream struct {
	events chan Event
	cursor int64
}

type JobQueue interface {
	EnqueueGeneration(context.Context, string, time.Time) error
	CancelGeneration(context.Context, string, time.Time) error
}

type externalEventPollingRepository interface {
	RequiresExternalEventPolling() bool
}

type ServiceOption func(*Service)

func WithJobQueue(queue JobQueue) ServiceOption {
	return func(service *Service) {
		service.jobQueue = queue
	}
}

func NewService(
	repository Repository,
	newID func() string,
	now func() time.Time,
	options ...ServiceOption,
) *Service {
	service := &Service{
		repository: repository,
		newID:      newID,
		now:        now,
		streams:    make(map[string]map[*eventStream]struct{}),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (service *Service) Create(ctx context.Context, userID string, request CreateRequest) (*Session, error) {
	createdAt := service.now().UTC()
	session, err := NewSession(CreateInput{
		ID:        service.newID(),
		UserID:    userID,
		Prompt:    request.Prompt,
		Target:    request.Target,
		Locale:    request.Locale,
		CreatedAt: createdAt,
	})
	if err != nil {
		return nil, err
	}
	session.Summary = summarizeRequirement(session.Prompt, session.Messages[1:], session.Locale)
	if _, err := session.Transition(StatusAwaitingConfirmation, Transition{
		Stage:    "requirements",
		Message:  "请确认结构化需求",
		Progress: 0.1,
		At:       createdAt,
	}); err != nil {
		return nil, err
	}
	if err := service.repository.Create(ctx, session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (service *Service) Get(ctx context.Context, userID, sessionID string) (*Session, error) {
	return service.repository.Get(ctx, userID, sessionID)
}

func (service *Service) AddMessage(ctx context.Context, userID, sessionID, content string) (*Session, error) {
	service.streamMu.Lock()
	defer service.streamMu.Unlock()
	session, err := service.repository.Update(ctx, userID, sessionID, func(session *Session) error {
		if session.Status != StatusAwaitingConfirmation {
			return ErrConflict
		}
		if _, err := session.AddMessage(content, service.now()); err != nil {
			return err
		}
		session.Summary = summarizeRequirement(session.Prompt, session.Messages[1:], session.Locale)
		return nil
	})
	service.publishLocked(session, err)
	return session, err
}

func (service *Service) Confirm(ctx context.Context, userID, sessionID string) (*Session, error) {
	service.streamMu.Lock()
	session, err := service.repository.Update(ctx, userID, sessionID, func(session *Session) error {
		if session.Status != StatusAwaitingConfirmation {
			return ErrConflict
		}
		if _, err := session.Confirm(
			[]string{"storage", "window.manageSelf"},
			service.now(),
		); err != nil {
			return err
		}
		session.Summary = summarizeRequirement(
			session.ConfirmedRequirement.InitialPrompt,
			session.ConfirmedRequirement.AdditionalMessages,
			session.ConfirmedRequirement.Locale,
		)
		return nil
	})
	if err != nil {
		service.streamMu.Unlock()
		return nil, err
	}
	service.publishLocked(session, nil)
	service.streamMu.Unlock()
	if service.jobQueue != nil {
		if err := service.jobQueue.EnqueueGeneration(ctx, sessionID, service.now()); err != nil {
			_, _ = service.MarkFailed(ctx, sessionID, "JOB_ENQUEUE_FAILED")
			return nil, err
		}
	}
	return session, nil
}

func (service *Service) Cancel(ctx context.Context, userID, sessionID string) (*Session, error) {
	service.streamMu.Lock()
	session, err := service.repository.Update(ctx, userID, sessionID, func(session *Session) error {
		if session.Status == StatusCancelled {
			return nil
		}
		if session.Status == StatusReady || session.Status == StatusFailed {
			return ErrConflict
		}
		_, err := session.Cancel(service.now())
		return err
	})
	if err != nil {
		service.streamMu.Unlock()
		return nil, err
	}
	service.publishLocked(session, nil)
	service.streamMu.Unlock()
	if service.jobQueue != nil {
		if err := service.jobQueue.CancelGeneration(ctx, sessionID, service.now()); err != nil {
			return nil, err
		}
	}
	return session, nil
}

func (service *Service) EventsAfter(ctx context.Context, userID, sessionID string, eventID int64) ([]Event, error) {
	session, err := service.repository.Get(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0)
	for _, event := range session.Events {
		if event.EventID > eventID {
			events = append(events, event)
		}
	}
	return events, nil
}

func (service *Service) SubscribeEvents(
	ctx context.Context,
	userID string,
	sessionID string,
	eventID int64,
) (<-chan Event, func(), error) {
	service.streamMu.Lock()
	defer service.streamMu.Unlock()
	session, err := service.repository.Get(ctx, userID, sessionID)
	if err != nil {
		return nil, nil, err
	}
	stream := &eventStream{events: make(chan Event, len(session.Events)+64), cursor: eventID}
	for _, event := range session.Events {
		if event.EventID > stream.cursor {
			stream.events <- event
			stream.cursor = event.EventID
		}
	}
	if service.streams[sessionID] == nil {
		service.streams[sessionID] = make(map[*eventStream]struct{})
	}
	service.streams[sessionID][stream] = struct{}{}
	var once sync.Once
	done := make(chan struct{})
	cancel := func() {
		once.Do(func() {
			service.streamMu.Lock()
			defer service.streamMu.Unlock()
			delete(service.streams[sessionID], stream)
			if len(service.streams[sessionID]) == 0 {
				delete(service.streams, sessionID)
			}
			close(stream.events)
			close(done)
		})
	}
	if ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			cancel()
		}()
	}
	if polling, ok := service.repository.(externalEventPollingRepository); ok && polling.RequiresExternalEventPolling() {
		go service.pollExternalEvents(ctx, done, userID, sessionID, stream)
	}
	return stream.events, cancel, nil
}

func (service *Service) pollExternalEvents(
	ctx context.Context,
	done <-chan struct{},
	userID string,
	sessionID string,
	stream *eventStream,
) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			session, err := service.repository.Get(ctx, userID, sessionID)
			if err != nil {
				continue
			}
			service.streamMu.Lock()
			if _, subscribed := service.streams[sessionID][stream]; subscribed {
				service.publishStreamLocked(stream, session.Events)
			}
			service.streamMu.Unlock()
		}
	}
}

func (service *Service) StartGenerating(ctx context.Context, sessionID string) (*Session, error) {
	return service.updateSystem(ctx, sessionID, func(session *Session) error {
		_, err := session.Transition(StatusGenerating, Transition{
			Stage:    "generating",
			Message:  "编码 Agent 正在生成卡片",
			Progress: 0.35,
			At:       service.now(),
		})
		return err
	})
}

func (service *Service) StartValidating(ctx context.Context, sessionID string) (*Session, error) {
	return service.updateSystem(ctx, sessionID, func(session *Session) error {
		_, err := session.Transition(StatusValidating, Transition{
			Stage:    "validating",
			Message:  "正在执行安全和合同验证",
			Progress: 0.75,
			At:       service.now(),
		})
		return err
	})
}

func (service *Service) MarkReady(ctx context.Context, sessionID, versionID string) (*Session, error) {
	return service.updateSystem(ctx, sessionID, func(session *Session) error {
		_, err := session.Transition(StatusReady, Transition{
			Stage:     "ready",
			Message:   "卡片已生成并签名",
			Progress:  1,
			VersionID: versionID,
			At:        service.now(),
		})
		return err
	})
}

func (service *Service) MarkFailed(ctx context.Context, sessionID, code string) (*Session, error) {
	return service.updateSystem(ctx, sessionID, func(session *Session) error {
		if session.IsTerminal() {
			return nil
		}
		_, err := session.Transition(StatusFailed, Transition{
			Stage:     "failed",
			Message:   "生成的卡片未通过处理流程",
			Progress:  1,
			ErrorCode: code,
			At:        service.now(),
		})
		return err
	})
}

func (service *Service) updateSystem(
	ctx context.Context,
	sessionID string,
	change func(*Session) error,
) (*Session, error) {
	service.streamMu.Lock()
	defer service.streamMu.Unlock()
	session, err := service.repository.UpdateSystem(ctx, sessionID, change)
	service.publishLocked(session, err)
	return session, err
}

func (service *Service) publishLocked(session *Session, err error) {
	if err != nil || session == nil {
		return
	}
	for stream := range service.streams[session.ID] {
		service.publishStreamLocked(stream, session.Events)
	}
}

func (service *Service) publishStreamLocked(stream *eventStream, events []Event) {
	for _, event := range events {
		if event.EventID > stream.cursor {
			stream.events <- event
			stream.cursor = event.EventID
		}
	}
}

func summarize(prompt, locale string) RequirementSummary {
	constraints := []string{"卡片必须通过能力代理访问宿主能力"}
	lower := strings.ToLower(prompt)
	if strings.Contains(prompt, "离线") || strings.Contains(lower, "offline") {
		constraints = append(constraints, "纯本地功能必须可离线使用")
	}
	return RequirementSummary{
		Goal:        strings.TrimSpace(prompt),
		Constraints: constraints,
		Locale:      locale,
	}
}

func summarizeRequirement(initialPrompt string, additionalMessages []Message, locale string) RequirementSummary {
	parts := make([]string, 0, len(additionalMessages)+1)
	parts = append(parts, strings.TrimSpace(initialPrompt))
	for _, message := range additionalMessages {
		if content := strings.TrimSpace(message.Content); content != "" {
			parts = append(parts, content)
		}
	}
	return summarize(strings.Join(parts, "\n"), locale)
}

func IsClientError(err error) bool {
	return errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrInvalidTarget) ||
		errors.Is(err, ErrInvalidTransition)
}
