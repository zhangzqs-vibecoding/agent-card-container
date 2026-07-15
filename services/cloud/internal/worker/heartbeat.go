package worker

import (
	"context"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
)

func runLeaseHeartbeat(
	ctx context.Context,
	cancelWork context.CancelFunc,
	store jobs.Store,
	jobID, workerID string,
	lease time.Duration,
	now func() time.Time,
	ticks <-chan time.Time,
	done chan<- error,
) {
	for {
		select {
		case <-ctx.Done():
			done <- nil
			return
		case <-ticks:
			if err := store.ExtendLease(ctx, jobID, workerID, now().UTC(), lease); err != nil {
				if ctx.Err() != nil {
					done <- nil
					return
				}
				cancelWork()
				done <- err
				return
			}
		}
	}
}

type leaseHeartbeat struct {
	workContext context.Context
	cancelWork  context.CancelFunc
	cancel      context.CancelFunc
	ticker      *time.Ticker
	done        <-chan error
}

func startLeaseHeartbeat(
	parent context.Context,
	store jobs.Store,
	jobID, workerID string,
	lease, interval time.Duration,
	now func() time.Time,
) *leaseHeartbeat {
	workContext, cancelWork := context.WithCancel(parent)
	heartbeatContext, cancelHeartbeat := context.WithCancel(parent)
	ticker := time.NewTicker(interval)
	done := make(chan error, 1)
	go runLeaseHeartbeat(
		heartbeatContext,
		cancelWork,
		store,
		jobID,
		workerID,
		lease,
		now,
		ticker.C,
		done,
	)
	return &leaseHeartbeat{
		workContext: workContext,
		cancelWork:  cancelWork,
		cancel:      cancelHeartbeat,
		ticker:      ticker,
		done:        done,
	}
}

func (heartbeat *leaseHeartbeat) Stop() error {
	heartbeat.ticker.Stop()
	heartbeat.cancel()
	err := <-heartbeat.done
	heartbeat.cancelWork()
	return err
}
