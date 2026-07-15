package jobs

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

var (
	ErrNoJob    = errors.New("no generation job available")
	ErrNotFound = errors.New("generation job not found")
	ErrConflict = errors.New("generation job conflict")
)

type Job struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"sessionId"`
	Status      Status    `json:"status"`
	Attempts    int       `json:"attempts"`
	MaxAttempts int       `json:"maxAttempts"`
	LeaseOwner  string    `json:"leaseOwner,omitempty"`
	LeaseUntil  time.Time `json:"leaseUntil,omitempty"`
	AvailableAt time.Time `json:"availableAt"`
	CardID      string    `json:"cardId,omitempty"`
	VersionID   string    `json:"versionId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Publication struct {
	CardID    string
	VersionID string
}

type Store interface {
	Enqueue(context.Context, string, time.Time) (*Job, error)
	Claim(context.Context, string, time.Time, time.Duration) (*Job, error)
	Get(context.Context, string) (*Job, error)
	Complete(context.Context, string, string, time.Time) error
	Fail(context.Context, string, string, time.Time) error
	CancelBySession(context.Context, string, time.Time) error
	ExtendLease(context.Context, string, string, time.Time, time.Duration) error
	ReservePublication(context.Context, string, string, string, string, time.Time) (Publication, error)
	Retry(context.Context, string, string, time.Time, time.Time) error
}

type MemoryStore struct {
	mu      sync.Mutex
	newID   func() string
	jobs    map[string]*Job
	session map[string]string
}

func NewMemoryStore(newID func() string) *MemoryStore {
	return &MemoryStore{
		newID:   newID,
		jobs:    make(map[string]*Job),
		session: make(map[string]string),
	}
}

func (store *MemoryStore) Enqueue(ctx context.Context, sessionID string, at time.Time) (*Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if jobID, exists := store.session[sessionID]; exists {
		return clone(store.jobs[jobID]), nil
	}
	at = at.UTC()
	job := &Job{
		ID:          store.newID(),
		SessionID:   sessionID,
		Status:      StatusQueued,
		MaxAttempts: 3,
		CreatedAt:   at,
		UpdatedAt:   at,
		AvailableAt: at,
	}
	store.jobs[job.ID] = job
	store.session[sessionID] = job.ID
	return clone(job), nil
}

func (store *MemoryStore) Claim(
	ctx context.Context,
	worker string,
	now time.Time,
	lease time.Duration,
) (*Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	now = now.UTC()
	candidates := make([]*Job, 0, len(store.jobs))
	for _, job := range store.jobs {
		if (job.Status == StatusQueued && !job.AvailableAt.After(now)) ||
			(job.Status == StatusRunning && !job.LeaseUntil.After(now)) {
			candidates = append(candidates, job)
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].CreatedAt.Equal(candidates[right].CreatedAt) {
			return candidates[left].ID < candidates[right].ID
		}
		return candidates[left].CreatedAt.Before(candidates[right].CreatedAt)
	})
	for _, job := range candidates {
		if job.Attempts >= job.MaxAttempts {
			job.Status = StatusFailed
			job.LeaseOwner = ""
			job.LeaseUntil = time.Time{}
			job.UpdatedAt = now
			continue
		}
		job.Status = StatusRunning
		job.Attempts++
		job.LeaseOwner = worker
		job.LeaseUntil = now.Add(lease)
		job.UpdatedAt = now
		return clone(job), nil
	}
	return nil, ErrNoJob
}

func (store *MemoryStore) ExtendLease(
	ctx context.Context,
	jobID, worker string,
	now time.Time,
	lease time.Duration,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if lease <= 0 {
		return ErrConflict
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	job, exists := store.jobs[jobID]
	now = now.UTC()
	if !exists || job.Status != StatusRunning || job.LeaseOwner != worker || !job.LeaseUntil.After(now) {
		return ErrConflict
	}
	job.LeaseUntil = now.Add(lease)
	job.UpdatedAt = now
	return nil
}

func (store *MemoryStore) ReservePublication(
	ctx context.Context,
	jobID, worker, cardID, versionID string,
	at time.Time,
) (Publication, error) {
	if err := ctx.Err(); err != nil {
		return Publication{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	job, exists := store.jobs[jobID]
	at = at.UTC()
	if !exists || job.Status != StatusRunning || job.LeaseOwner != worker || !job.LeaseUntil.After(at) {
		return Publication{}, ErrConflict
	}
	if job.CardID == "" && job.VersionID == "" {
		if cardID == "" || versionID == "" {
			return Publication{}, ErrConflict
		}
		job.CardID = cardID
		job.VersionID = versionID
		job.UpdatedAt = at
	}
	return Publication{CardID: job.CardID, VersionID: job.VersionID}, nil
}

func (store *MemoryStore) Retry(
	ctx context.Context,
	jobID, worker string,
	at, availableAt time.Time,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	job, exists := store.jobs[jobID]
	at = at.UTC()
	availableAt = availableAt.UTC()
	if !exists || job.Status != StatusRunning || job.LeaseOwner != worker || !job.LeaseUntil.After(at) || availableAt.Before(at) {
		return ErrConflict
	}
	job.Status = StatusQueued
	job.LeaseOwner = ""
	job.LeaseUntil = time.Time{}
	job.AvailableAt = availableAt
	job.UpdatedAt = at
	return nil
}

func (store *MemoryStore) Get(ctx context.Context, jobID string) (*Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	job, exists := store.jobs[jobID]
	if !exists {
		return nil, ErrNotFound
	}
	return clone(job), nil
}

func (store *MemoryStore) Complete(ctx context.Context, jobID, worker string, at time.Time) error {
	return store.finish(ctx, jobID, worker, StatusCompleted, at)
}

func (store *MemoryStore) Fail(ctx context.Context, jobID, worker string, at time.Time) error {
	return store.finish(ctx, jobID, worker, StatusFailed, at)
}

func (store *MemoryStore) finish(
	ctx context.Context,
	jobID string,
	worker string,
	status Status,
	at time.Time,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	job, exists := store.jobs[jobID]
	if !exists {
		return ErrNotFound
	}
	if job.Status == status {
		return nil
	}
	at = at.UTC()
	if job.Status != StatusRunning || job.LeaseOwner != worker || !job.LeaseUntil.After(at) {
		return ErrConflict
	}
	job.Status = status
	job.LeaseOwner = ""
	job.LeaseUntil = time.Time{}
	job.UpdatedAt = at
	return nil
}

func (store *MemoryStore) CancelBySession(ctx context.Context, sessionID string, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	jobID, exists := store.session[sessionID]
	if !exists {
		return nil
	}
	job := store.jobs[jobID]
	if job.Status == StatusCompleted || job.Status == StatusFailed || job.Status == StatusCancelled {
		return nil
	}
	job.Status = StatusCancelled
	job.LeaseOwner = ""
	job.LeaseUntil = time.Time{}
	job.UpdatedAt = at.UTC()
	return nil
}

func clone(job *Job) *Job {
	if job == nil {
		return nil
	}
	cloned := *job
	return &cloned
}
