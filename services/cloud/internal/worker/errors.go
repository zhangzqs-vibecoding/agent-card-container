package worker

import (
	"errors"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

func isRetryableJobError(err error) bool {
	if errors.Is(err, publish.ErrRetryable) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary()) {
		return true
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return false
	}
	return strings.HasPrefix(postgresError.Code, "08") ||
		postgresError.Code == "40001" ||
		postgresError.Code == "40P01" ||
		postgresError.Code == "53300" ||
		postgresError.Code == "57P03"
}

func jobRetryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Second
	}
	return 2 * time.Second
}
