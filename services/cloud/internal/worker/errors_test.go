package worker

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

type temporaryNetworkError struct{}

func (temporaryNetworkError) Error() string   { return "temporary network failure" }
func (temporaryNetworkError) Timeout() bool   { return true }
func (temporaryNetworkError) Temporary() bool { return true }

func TestRetryableJobErrorClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "publish adapter", err: publish.ErrRetryable, want: true},
		{name: "network timeout", err: temporaryNetworkError{}, want: true},
		{name: "postgres connection", err: &pgconn.PgError{Code: "08006"}, want: true},
		{name: "postgres serialization", err: &pgconn.PgError{Code: "40001"}, want: true},
		{name: "postgres deadlock", err: &pgconn.PgError{Code: "40P01"}, want: true},
		{name: "postgres constraint", err: &pgconn.PgError{Code: "23505"}},
		{name: "validation", err: agent.ErrValidationFailed},
		{name: "version conflict", err: publish.ErrVersionConflict},
		{
			name: "exhausted model provider retries stay within the three-call generation budget",
			err:  modelprovider.ErrRetryable,
		},
		{name: "ordinary error", err: errors.New("permanent")},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isRetryableJobError(testCase.err); got != testCase.want {
				t.Fatalf("isRetryableJobError() = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestJobRetryDelayIsBounded(t *testing.T) {
	t.Parallel()

	if jobRetryDelay(1) != time.Second || jobRetryDelay(2) != 2*time.Second || jobRetryDelay(99) != 2*time.Second {
		t.Fatalf("retry delays are not bounded")
	}
}
