package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zzq/agent-card-container/services/cloud/internal/app"
)

func TestNewPreservesServiceName(t *testing.T) {
	t.Parallel()

	got := app.New("test").Name()

	if got != "test" {
		t.Fatalf("Name() = %q, want %q", got, "test")
	}
}

func TestHandlerServesHealthz(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	app.New("test").Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}
