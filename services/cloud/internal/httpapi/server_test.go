package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zzq/agent-card-container/services/cloud/internal/httpapi"
)

type readinessCheckerFunc func(context.Context) error

func (check readinessCheckerFunc) Ready(ctx context.Context) error {
	return check(ctx)
}

func TestHealthzReturnsServiceStatus(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	httpapi.NewHandler("agent-card-cloud").ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %q, want ok", body["status"])
	}
	if body["service"] != "agent-card-cloud" {
		t.Fatalf("service body = %q, want agent-card-cloud", body["service"])
	}
}

func TestHealthzRejectsOtherMethods(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	response := httptest.NewRecorder()

	httpapi.NewHandler("agent-card-cloud").ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"status = %d, want %d",
			response.Code,
			http.StatusMethodNotAllowed,
		)
	}
}

func TestReadyzReturnsStableServiceStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		check      readinessCheckerFunc
		wantStatus int
		wantBody   string
	}{
		{
			name:       "ready",
			check:      func(context.Context) error { return nil },
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name: "dependency unavailable",
			check: func(context.Context) error {
				return errors.New("postgres://secret@database/private")
			},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "unavailable",
		},
		{
			name: "request cancelled",
			check: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "unavailable",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			if testCase.name == "request cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			request := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
			response := httptest.NewRecorder()

			httpapi.NewReadinessHandler("agent-card-cloud", testCase.check).ServeHTTP(response, request)

			if response.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, testCase.wantStatus)
			}
			if got := response.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q", got)
			}
			rawBody := response.Body.Bytes()
			var body map[string]string
			if err := json.Unmarshal(rawBody, &body); err != nil {
				t.Fatal(err)
			}
			if body["status"] != testCase.wantBody || body["service"] != "agent-card-cloud" {
				t.Fatalf("body = %#v", body)
			}
			if string(rawBody) == "postgres://secret@database/private" {
				t.Fatal("response exposed dependency error")
			}
		})
	}
}

func TestReadyzRejectsOtherMethods(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	httpapi.NewReadinessHandler(
		"agent-card-cloud",
		readinessCheckerFunc(func(context.Context) error { return nil }),
	).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/readyz", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}
