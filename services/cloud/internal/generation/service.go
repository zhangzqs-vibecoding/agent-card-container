package generation

import (
	"context"
	"errors"
	"strings"
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
}

func NewService(repository Repository, newID func() string, now func() time.Time) *Service {
	return &Service{repository: repository, newID: newID, now: now}
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
	session.Summary = summarize(session.Prompt, session.Locale)
	session.Messages = append(session.Messages, Message{
		Role:      "user",
		Content:   session.Prompt,
		CreatedAt: createdAt,
	})
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
	return service.repository.Update(ctx, userID, sessionID, func(session *Session) error {
		if session.Status != StatusAwaitingConfirmation {
			return ErrConflict
		}
		if _, err := session.AddMessage(content, service.now()); err != nil {
			return err
		}
		session.Summary = summarize(content, session.Locale)
		return nil
	})
}

func (service *Service) Confirm(ctx context.Context, userID, sessionID string) (*Session, error) {
	return service.repository.Update(ctx, userID, sessionID, func(session *Session) error {
		if session.Status != StatusAwaitingConfirmation {
			return ErrConflict
		}
		_, err := session.Transition(StatusQueued, Transition{
			Stage:    "queued",
			Message:  "生成任务已入队",
			Progress: 0.2,
			At:       service.now(),
		})
		return err
	})
}

func (service *Service) Cancel(ctx context.Context, userID, sessionID string) (*Session, error) {
	return service.repository.Update(ctx, userID, sessionID, func(session *Session) error {
		if session.Status == StatusCancelled {
			return nil
		}
		if session.Status == StatusReady || session.Status == StatusFailed {
			return ErrConflict
		}
		_, err := session.Cancel(service.now())
		return err
	})
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

func IsClientError(err error) bool {
	return errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrInvalidTarget) ||
		errors.Is(err, ErrInvalidTransition)
}
