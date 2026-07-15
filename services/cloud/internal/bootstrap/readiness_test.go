package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestDependencyReadinessRequiresEveryCheck(t *testing.T) {
	t.Parallel()

	databaseErr := errors.New("database unavailable")
	objectErr := errors.New("object store unavailable")
	tests := []struct {
		name   string
		errors []error
		want   error
	}{
		{name: "ready", errors: []error{nil, nil}},
		{name: "database unavailable", errors: []error{databaseErr, nil}, want: errDatabaseUnavailable},
		{name: "object store unavailable", errors: []error{nil, objectErr}, want: errObjectStoreUnavailable},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			checker := dependencyReadiness{
				database: func(context.Context) error { return testCase.errors[0] },
				objects:  func(context.Context) error { return testCase.errors[1] },
			}
			err := checker.Ready(context.Background())
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Ready() error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestDependencyReadinessRunsChecksConcurrently(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	check := func(context.Context) error {
		started <- struct{}{}
		<-release
		return nil
	}
	checker := dependencyReadiness{database: check, objects: check}
	done := make(chan error, 1)
	go func() { done <- checker.Ready(context.Background()) }()

	<-started
	<-started
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDependencyReadinessPropagatesCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var called sync.WaitGroup
	called.Add(2)
	check := func(ctx context.Context) error {
		defer called.Done()
		return ctx.Err()
	}
	checker := dependencyReadiness{database: check, objects: check}

	if err := checker.Ready(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ready() error = %v, want context.Canceled", err)
	}
	called.Wait()
}
