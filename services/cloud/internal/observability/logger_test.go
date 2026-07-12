package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
)

func TestJSONLoggerUsesStableTimestampAndFields(t *testing.T) {
	var output bytes.Buffer
	logger := observability.NewJSONLogger(&output)
	logger.Info("http_request", "requestId", "req_01", "status", 200)

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if event["event"] != "http_request" || event["level"] != "INFO" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := time.Parse(time.RFC3339Nano, event["timestamp"].(string)); err != nil {
		t.Fatalf("timestamp = %q: %v", event["timestamp"], err)
	}
	if _, exists := event["msg"]; exists {
		t.Fatalf("unexpected msg field: %#v", event)
	}
}

func TestRequestMetadataIsMutableOnlyThroughSafeHelpers(t *testing.T) {
	metadata := observability.NewRequestMetadata("req_01")
	ctx := observability.WithRequestMetadata(context.Background(), metadata)

	observability.SetAuthenticatedUser(ctx, "user-01")

	snapshot := observability.RequestSnapshot(ctx)
	if snapshot.RequestID != "req_01" || snapshot.UserID != "user-01" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if observability.RequestSnapshot(context.Background()).RequestID != "" {
		t.Fatal("metadata leaked into unrelated context")
	}
}

func TestErrorKindNeverReturnsRawErrorText(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{context.Canceled, "cancelled"},
		{context.DeadlineExceeded, "timeout"},
		{errors.New("secret upstream response"), "internal"},
	}
	for _, test := range cases {
		if got := observability.ErrorKind(test.err); got != test.want {
			t.Fatalf("ErrorKind(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}

func TestSafeEventDoesNotInferSensitiveFields(t *testing.T) {
	var output bytes.Buffer
	logger := observability.NewJSONLogger(&output)
	logger.LogAttrs(context.Background(), slog.LevelWarn, "model_request_failed",
		slog.String("errorKind", observability.ErrorKind(errors.New("Bearer secret-token"))),
	)
	if bytes.Contains(output.Bytes(), []byte("secret-token")) || bytes.Contains(output.Bytes(), []byte("Bearer")) {
		t.Fatalf("sensitive error leaked: %s", output.Bytes())
	}
}
