package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/httpapi"
)

func TestGenerationAPISSEStreamsEventsCreatedAfterConnection(t *testing.T) {
	service := generation.NewService(
		generation.NewMemoryRepository(),
		func() string { return "gen_live" },
		time.Now,
	)
	handler := httpapi.NewGenerationHandler(httpapi.GenerationHandlerConfig{
		ServiceName: "agent-card-cloud",
		Service:     service,
		Authenticator: httpapi.StaticBearerAuthenticator{
			"token-owner": "user-owner",
		},
		NewRequestID: func() string { return "req_live" },
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	session, err := service.Create(context.Background(), "user-owner", generation.CreateRequest{
		Prompt: "离线时钟",
		Target: generation.TargetNative,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodGet, server.URL+"/v1/generations/"+session.ID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer token-owner")
	request.Header.Set("Last-Event-ID", "1")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if _, err := service.Confirm(context.Background(), "user-owner", session.ID); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if line != "id: 2\n" {
		t.Fatalf("first streamed line = %q, want event 2", line)
	}
}

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

	streamContext, cancelStream := context.WithCancel(context.Background())
	eventsRequest := httptest.NewRequest(http.MethodGet, "/v1/generations/"+sessionID+"/events", nil).WithContext(streamContext)
	eventsRequest.Header.Set("Authorization", "Bearer token-owner")
	eventsRequest.Header.Set("Last-Event-ID", "1")
	eventsResponse := newStreamingRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(eventsResponse, eventsRequest)
		close(done)
	}()
	for range 3 {
		select {
		case <-eventsResponse.flushed:
		case <-time.After(time.Second):
			t.Fatal("SSE replay was not flushed")
		}
	}
	cancelStream()
	<-done
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

type streamingRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
}

func newStreamingRecorder() *streamingRecorder {
	return &streamingRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		flushed:          make(chan struct{}, 16),
	}
}

func (recorder *streamingRecorder) Flush() {
	recorder.ResponseRecorder.Flush()
	recorder.flushed <- struct{}{}
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
