package generation

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrNotFound = errors.New("generation session not found")
	ErrConflict = errors.New("generation conflict")
)

type Repository interface {
	Create(context.Context, *Session) error
	Get(context.Context, string, string) (*Session, error)
	Update(context.Context, string, string, func(*Session) error) (*Session, error)
}

type MemoryRepository struct {
	mu       sync.RWMutex
	sessions map[string]*Session
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
	return &cloned
}
