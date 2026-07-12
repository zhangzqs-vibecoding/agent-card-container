package jobs

import (
	"context"
	"time"
)

type GenerationQueue struct {
	store Store
}

func NewGenerationQueue(store Store) GenerationQueue {
	return GenerationQueue{store: store}
}

func (queue GenerationQueue) EnqueueGeneration(
	ctx context.Context,
	sessionID string,
	at time.Time,
) error {
	_, err := queue.store.Enqueue(ctx, sessionID, at)
	return err
}

func (queue GenerationQueue) CancelGeneration(
	ctx context.Context,
	sessionID string,
	at time.Time,
) error {
	return queue.store.CancelBySession(ctx, sessionID, at)
}
