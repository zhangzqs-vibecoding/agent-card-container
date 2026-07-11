package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zzq/agent-card-container/services/cloud/internal/httpapi"
)

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
