package bootstrap

import (
	"context"
	"errors"
	"time"
)

var (
	errDatabaseUnavailable    = errors.New("database unavailable")
	errObjectStoreUnavailable = errors.New("object store unavailable")
)

type dependencyReadiness struct {
	database func(context.Context) error
	objects  func(context.Context) error
	timeout  time.Duration
}

func (checker dependencyReadiness) Ready(ctx context.Context) error {
	timeout := checker.timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		dependency error
		err        error
	}
	results := make(chan result, 2)
	go func() {
		results <- result{dependency: errDatabaseUnavailable, err: checker.database(ctx)}
	}()
	go func() {
		results <- result{dependency: errObjectStoreUnavailable, err: checker.objects(ctx)}
	}()

	var unavailable error
	for range 2 {
		result := <-results
		if result.err == nil {
			continue
		}
		if errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded) {
			return result.err
		}
		if unavailable == nil {
			unavailable = result.dependency
		}
	}
	return unavailable
}

type readyRuntime struct{}

func (readyRuntime) Ready(context.Context) error { return nil }
