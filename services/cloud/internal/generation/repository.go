package generation

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("generation session not found")
	ErrConflict = errors.New("generation conflict")
)

type Repository interface {
	Create(context.Context, *Session) error
	Get(context.Context, string, string) (*Session, error)
	Update(context.Context, string, string, func(*Session) error) (*Session, error)
	GetSystem(context.Context, string) (*Session, error)
	UpdateSystem(context.Context, string, func(*Session) error) (*Session, error)
}

type AtomicConfirmationRepository interface {
	ConfirmAndEnqueue(
		context.Context,
		string,
		string,
		func(*Session) error,
		string,
		time.Time,
	) (*Session, error)
}

func (repository *MemoryRepository) GetSystem(ctx context.Context, sessionID string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	session, exists := repository.sessions[sessionID]
	if !exists {
		return nil, ErrNotFound
	}
	return cloneSession(session), nil
}

type MemoryRepository struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func (repository *MemoryRepository) UpdateSystem(
	ctx context.Context,
	sessionID string,
	change func(*Session) error,
) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	current, exists := repository.sessions[sessionID]
	if !exists {
		return nil, ErrNotFound
	}
	candidate := cloneSession(current)
	if err := change(candidate); err != nil {
		return nil, err
	}
	repository.sessions[sessionID] = candidate
	return cloneSession(candidate), nil
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{sessions: make(map[string]*Session)}
}

func (repository *MemoryRepository) Create(ctx context.Context, session *Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if _, exists := repository.sessions[session.ID]; exists {
		return ErrConflict
	}
	repository.sessions[session.ID] = cloneSession(session)
	return nil
}

func (repository *MemoryRepository) Get(ctx context.Context, userID, sessionID string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	session, exists := repository.sessions[sessionID]
	if !exists || session.UserID != userID {
		return nil, ErrNotFound
	}
	return cloneSession(session), nil
}

func (repository *MemoryRepository) Update(
	ctx context.Context,
	userID string,
	sessionID string,
	change func(*Session) error,
) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	current, exists := repository.sessions[sessionID]
	if !exists || current.UserID != userID {
		return nil, ErrNotFound
	}
	candidate := cloneSession(current)
	if err := change(candidate); err != nil {
		return nil, err
	}
	repository.sessions[sessionID] = candidate
	return cloneSession(candidate), nil
}

func cloneSession(session *Session) *Session {
	cloned := *session
	cloned.Events = append([]Event(nil), session.Events...)
	cloned.Messages = append([]Message(nil), session.Messages...)
	cloned.Summary.Constraints = append([]string(nil), session.Summary.Constraints...)
	if session.ConfirmedRequirement != nil {
		requirement := *session.ConfirmedRequirement
		if session.ConfirmedRequirement.AdditionalMessages != nil {
			requirement.AdditionalMessages = make([]Message, len(session.ConfirmedRequirement.AdditionalMessages))
			copy(requirement.AdditionalMessages, session.ConfirmedRequirement.AdditionalMessages)
		}
		if session.ConfirmedRequirement.AllowedCapabilities != nil {
			requirement.AllowedCapabilities = make([]string, len(session.ConfirmedRequirement.AllowedCapabilities))
			copy(requirement.AllowedCapabilities, session.ConfirmedRequirement.AllowedCapabilities)
		}
		cloned.ConfirmedRequirement = &requirement
	}
	return &cloned
}
