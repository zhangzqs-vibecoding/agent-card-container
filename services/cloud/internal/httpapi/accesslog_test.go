package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/httpapi"
	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
)

func TestAccessLogRecordsSafeCorrelatedRequest(t *testing.T) {
	var output bytes.Buffer
	nowValues := []time.Time{
		time.Date(2026, 7, 12, 14, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 12, 14, 0, 0, int(12*time.Millisecond), time.UTC),
	}
	middleware := httpapi.AccessLogMiddleware(httpapi.AccessLogConfig{
		Logger:       observability.NewJSONLogger(&output),
		NewRequestID: func() string { return "req_01" },
		Now: func() time.Time {
			value := nowValues[0]
			nowValues = nowValues[1:]
			return value
		},
	})
	handler := middleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observability.SetAuthenticatedUser(request.Context(), "user-01")
		_, _ = io.Copy(io.Discard, request.Body)
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"token":"response-secret"}`))
	}))
	request := httptest.NewRequest(http.MethodPost, "https://example.test/v1/cards?token=query-secret", strings.NewReader("body-secret"))
	request.RemoteAddr = "192.0.2.10:43125"
	request.Header.Set("Authorization", "Bearer header-secret")
	request.Header.Set("Cookie", "session=cookie-secret")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || response.Header().Get("X-Request-ID") != "req_01" {
		t.Fatalf("response status=%d requestID=%q", response.Code, response.Header().Get("X-Request-ID"))
	}
	event := decodeLog(t, output.Bytes())
	assertFields(t, event, map[string]any{
		"event":      "http_request",
		"requestId":  "req_01",
		"method":     "POST",
		"path":       "/v1/cards",
		"status":     float64(http.StatusCreated),
		"durationMs": float64(12),
		"userId":     "user-01",
		"remoteIp":   "192.0.2.10",
	})
	for _, forbidden := range []string{"query-secret", "header-secret", "cookie-secret", "body-secret", "response-secret", "Authorization", "Cookie"} {
		if bytes.Contains(output.Bytes(), []byte(forbidden)) {
			t.Fatalf("log contains %q: %s", forbidden, output.Bytes())
		}
	}
}

func TestAccessLogDefaultsStatusAndPreservesFlusher(t *testing.T) {
	var output bytes.Buffer
	middleware := httpapi.AccessLogMiddleware(httpapi.AccessLogConfig{
		Logger:       observability.NewJSONLogger(&output),
		NewRequestID: func() string { return "req_flush" },
		Now:          time.Now,
	})
	handler := middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Fatal("wrapped writer does not implement http.Flusher")
		}
		flusher.Flush()
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/events", nil))

	if got := decodeLog(t, output.Bytes())["status"]; got != float64(http.StatusOK) {
		t.Fatalf("status = %#v", got)
	}
}

func TestAccessLogWriterFailureDoesNotChangeResponse(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(failingWriter{}, nil))
	middleware := httpapi.AccessLogMiddleware(httpapi.AccessLogConfig{
		Logger:       logger,
		NewRequestID: func() string { return "req_failure" },
		Now:          time.Now,
	})
	response := httptest.NewRecorder()

	middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestReadinessFailureAccessLogDoesNotExposeDependencyError(t *testing.T) {
	var output bytes.Buffer
	handler := httpapi.AccessLogMiddleware(httpapi.AccessLogConfig{
		Logger:       observability.NewJSONLogger(&output),
		NewRequestID: func() string { return "req_ready" },
		Now:          time.Now,
	})(httpapi.NewReadinessHandler(
		"agent-card-cloud",
		readinessCheckerFunc(func(context.Context) error {
			return errors.New("postgres://user:database-secret@private-host/database")
		}),
	))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.Code)
	}
	if strings.Contains(output.String(), "database-secret") ||
		strings.Contains(response.Body.String(), "database-secret") {
		t.Fatalf("dependency error leaked: response=%q log=%q", response.Body.String(), output.String())
	}
	event := decodeLog(t, output.Bytes())
	assertFields(t, event, map[string]any{
		"path":   "/readyz",
		"status": float64(http.StatusServiceUnavailable),
	})
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func decodeLog(t *testing.T, source []byte) map[string]any {
	t.Helper()
	var event map[string]any
	if err := json.Unmarshal(source, &event); err != nil {
		t.Fatalf("decode log %q: %v", source, err)
	}
	return event
}

func assertFields(t *testing.T, event, expected map[string]any) {
	t.Helper()
	for key, value := range expected {
		if event[key] != value {
			t.Fatalf("%s = %#v, want %#v; event=%#v", key, event[key], value, event)
		}
	}
}
