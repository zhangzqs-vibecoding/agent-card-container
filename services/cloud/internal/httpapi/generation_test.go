package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/httpapi"
)

func TestGenerationAPIRequiresBearerAndUsesStableErrors(t *testing.T) {
	t.Parallel()

	handler := generationHandler()
	request := httptest.NewRequest(http.MethodPost, "/v1/generations", strings.NewReader(`{"prompt":"做一个番茄钟"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"requestId"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "UNAUTHENTICATED" || body.Error.RequestID == "" {
		t.Fatalf("error = %#v", body.Error)
	}
}

func TestGenerationAPICreateMessageConfirmGetAndSSEReplay(t *testing.T) {
	t.Parallel()

	handler := generationHandler()
	created := doJSON(t, handler, http.MethodPost, "/v1/generations", "token-owner", map[string]any{
		"prompt": "做一个离线番茄钟",
		"target": "auto",
		"locale": "zh-CN",
	}, http.StatusCreated)
	sessionID := created["id"].(string)
	if created["status"] != "awaiting_confirmation" {
		t.Fatalf("status = %v", created["status"])
	}

	doJSON(t, handler, http.MethodPost, "/v1/generations/"+sessionID+"/messages", "token-owner", map[string]any{
		"content": "增加暂停按钮",
	}, http.StatusOK)
	confirmed := doJSON(t, handler, http.MethodPost, "/v1/generations/"+sessionID+"/confirm", "token-owner", nil, http.StatusOK)
	if confirmed["status"] != "queued" {
		t.Fatalf("status = %v, want queued", confirmed["status"])
	}

	other := httptest.NewRequest(http.MethodGet, "/v1/generations/"+sessionID, nil)
	other.Header.Set("Authorization", "Bearer token-other")
	otherResponse := httptest.NewRecorder()
	handler.ServeHTTP(otherResponse, other)
	if otherResponse.Code != http.StatusNotFound {
		t.Fatalf("other user status = %d, want 404", otherResponse.Code)
	}

	eventsRequest := httptest.NewRequest(http.MethodGet, "/v1/generations/"+sessionID+"/events", nil)
	eventsRequest.Header.Set("Authorization", "Bearer token-owner")
	eventsRequest.Header.Set("Last-Event-ID", "1")
	eventsResponse := httptest.NewRecorder()
	handler.ServeHTTP(eventsResponse, eventsRequest)
	if eventsResponse.Code != http.StatusOK {
		t.Fatalf("events status = %d", eventsResponse.Code)
	}
	if contentType := eventsResponse.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	body := eventsResponse.Body.String()
	if !strings.Contains(body, "id: 2\n") || !strings.Contains(body, "id: 3\n") {
		t.Fatalf("SSE body = %q", body)
	}
	if strings.Contains(body, "id: 1\n") {
		t.Fatalf("SSE replay included old event: %q", body)
	}
}

func TestGenerationAPIRejectsUnknownJSONFields(t *testing.T) {
	t.Parallel()

	handler := generationHandler()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/generations",
		strings.NewReader(`{"prompt":"卡片","secret":"no"}`),
	)
	request.Header.Set("Authorization", "Bearer token-owner")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func generationHandler() http.Handler {
	service := generation.NewService(
		generation.NewMemoryRepository(),
		func() string { return "gen_01" },
		func() time.Time { return time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC) },
	)
	return httpapi.NewGenerationHandler(httpapi.GenerationHandlerConfig{
		ServiceName: "agent-card-cloud",
		Service:     service,
		Authenticator: httpapi.StaticBearerAuthenticator{
			"token-owner": "user-owner",
			"token-other": "user-other",
		},
		NewRequestID: func() string { return "req_01" },
	})
}

func doJSON(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	token string,
	input any,
	wantStatus int,
) map[string]any {
	t.Helper()
	var body bytes.Buffer
	if input != nil {
		if err := json.NewEncoder(&body).Encode(input); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, response.Code, wantStatus, response.Body.String())
	}
	var output map[string]any
	if err := json.NewDecoder(response.Body).Decode(&output); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	return output
}
