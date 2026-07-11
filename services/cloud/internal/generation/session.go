package generation

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusDraft                Status = "draft"
	StatusAwaitingConfirmation Status = "awaiting_confirmation"
	StatusQueued               Status = "queued"
	StatusGenerating           Status = "generating"
	StatusValidating           Status = "validating"
	StatusReady                Status = "ready"
	StatusFailed               Status = "failed"
	StatusCancelled            Status = "cancelled"
)

type Target string

const (
	TargetAuto   Target = "auto"
	TargetNative Target = "native"
	TargetWeb    Target = "web"
)

var (
	ErrInvalidTarget     = errors.New("invalid generation target")
	ErrInvalidTransition = errors.New("invalid generation transition")
	ErrTerminalSession   = errors.New("generation session is terminal")
)

type CreateInput struct {
	ID        string
	UserID    string
	Prompt    string
	Target    Target
	Locale    string
	CreatedAt time.Time
}

type Transition struct {
	Stage     string
	Message   string
	Progress  float64
	VersionID string
	At        time.Time
}

type Event struct {
	EventID   int64     `json:"eventId"`
	SessionID string    `json:"sessionId"`
	Type      string    `json:"type"`
	Stage     string    `json:"stage"`
	Message   string    `json:"message"`
	Progress  float64   `json:"progress"`
	VersionID string    `json:"versionId,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type RequirementSummary struct {
	Goal        string   `json:"goal"`
	Constraints []string `json:"constraints"`
	Locale      string   `json:"locale"`
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

type Session struct {
	ID        string             `json:"id"`
	UserID    string             `json:"-"`
	Prompt    string             `json:"prompt"`
	Target    Target             `json:"target"`
	Locale    string             `json:"locale"`
	Status    Status             `json:"status"`
	Summary   RequirementSummary `json:"summary"`
	Messages  []Message          `json:"messages"`
	VersionID string             `json:"versionId,omitempty"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
	Events    []Event            `json:"-"`

	nextEventID int64
}

func NewSession(input CreateInput) (*Session, error) {
	if strings.TrimSpace(input.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	if strings.TrimSpace(input.UserID) == "" {
		return nil, fmt.Errorf("userId is required")
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if input.Target == "" {
		input.Target = TargetAuto
	}
	switch input.Target {
	case TargetAuto, TargetNative, TargetWeb:
	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidTarget, input.Target)
	}
	if strings.TrimSpace(input.Locale) == "" {
		input.Locale = "zh-CN"
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	input.CreatedAt = input.CreatedAt.UTC()
	return &Session{
		ID:        input.ID,
		UserID:    input.UserID,
		Prompt:    strings.TrimSpace(input.Prompt),
		Target:    input.Target,
		Locale:    input.Locale,
		Status:    StatusDraft,
		CreatedAt: input.CreatedAt,
		UpdatedAt: input.CreatedAt,
		Events:    make([]Event, 0),
		Messages:  make([]Message, 0),
	}, nil
}

func (session *Session) AddMessage(content string, at time.Time) (Event, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Event{}, fmt.Errorf("message is required")
	}
	if session.IsTerminal() {
		return Event{}, ErrTerminalSession
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	session.Messages = append(session.Messages, Message{
		Role:      "user",
		Content:   content,
		CreatedAt: at.UTC(),
	})
	session.UpdatedAt = at.UTC()
	session.nextEventID++
	event := Event{
		EventID:   session.nextEventID,
		SessionID: session.ID,
		Type:      "message.added",
		Stage:     string(session.Status),
		Message:   "需求已更新",
		Progress:  0,
		Timestamp: at.UTC(),
	}
	session.Events = append(session.Events, event)
	return event, nil
}

func (session *Session) Transition(next Status, input Transition) (Event, error) {
	if !allowedTransitions[session.Status][next] {
		return Event{}, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, session.Status, next)
	}
	if input.Progress < 0 || input.Progress > 1 {
		return Event{}, fmt.Errorf("progress must be between 0 and 1")
	}
	if input.At.IsZero() {
		input.At = time.Now().UTC()
	}
	if input.Stage == "" {
		input.Stage = string(next)
	}
	session.Status = next
	session.UpdatedAt = input.At.UTC()
	if input.VersionID != "" {
		session.VersionID = input.VersionID
	}
	session.nextEventID++
	event := Event{
		EventID:   session.nextEventID,
		SessionID: session.ID,
		Type:      "status.changed",
		Stage:     input.Stage,
		Message:   input.Message,
		Progress:  input.Progress,
		VersionID: input.VersionID,
		Timestamp: input.At.UTC(),
	}
	session.Events = append(session.Events, event)
	return event, nil
}

func (session *Session) Cancel(at time.Time) (Event, error) {
	if session.IsTerminal() {
		return Event{}, ErrTerminalSession
	}
	return session.Transition(StatusCancelled, Transition{
		Stage:    "cancelled",
		Message:  "生成已取消",
		Progress: 0,
		At:       at,
	})
}

func (session *Session) IsTerminal() bool {
	return session.Status == StatusReady ||
		session.Status == StatusFailed ||
		session.Status == StatusCancelled
}

var allowedTransitions = map[Status]map[Status]bool{
	StatusDraft: {
		StatusAwaitingConfirmation: true,
		StatusCancelled:            true,
	},
	StatusAwaitingConfirmation: {
		StatusQueued:    true,
		StatusCancelled: true,
	},
	StatusQueued: {
		StatusGenerating: true,
		StatusCancelled:  true,
		StatusFailed:     true,
	},
	StatusGenerating: {
		StatusValidating: true,
		StatusCancelled:  true,
		StatusFailed:     true,
	},
	StatusValidating: {
		StatusReady:     true,
		StatusCancelled: true,
		StatusFailed:    true,
	},
	StatusReady:     {},
	StatusFailed:    {},
	StatusCancelled: {},
}
