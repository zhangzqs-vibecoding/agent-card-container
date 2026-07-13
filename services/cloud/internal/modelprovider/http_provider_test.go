package modelprovider_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

func TestHTTPProviderTransportsConfiguredRequest(t *testing.T) {
	t.Parallel()

	var authorization string
	var path string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		path = request.URL.Path
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writeCompletion(writer, "stop", `{"schemaVersion":1}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	response, err := provider.Generate(context.Background(), modelprovider.Request{
		SystemPrompt: "system-transport-marker",
		UserPrompt:   "user-transport-marker",
		JSONOutput:   true,
		MaxTokens:    8192,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if path != "/v1/chat/completions" || authorization != "Bearer test-secret" {
		t.Fatalf("path=%q Authorization=%q", path, authorization)
	}
	if body["model"] != "test-model" || body["max_tokens"] != float64(8192) || body["temperature"] != 0.2 {
		t.Fatalf("request body = %#v", body)
	}
	messages := body["messages"].([]any)
	if messages[0].(map[string]any)["content"] != "system-transport-marker" ||
		messages[1].(map[string]any)["content"] != "user-transport-marker" {
		t.Fatalf("messages = %#v", messages)
	}
	if body["response_format"].(map[string]any)["type"] != "json_object" {
		t.Fatalf("response_format = %#v", body["response_format"])
	}
	if body["thinking"].(map[string]any)["type"] != "disabled" {
		t.Fatalf("thinking = %#v", body["thinking"])
	}
	for _, legacy := range []string{"sessionId", "prompt", "locale", "runtime", "attempt", "validationError"} {
		if _, exists := body[legacy]; exists {
			t.Fatalf("transport body contains legacy field %q: %#v", legacy, body)
		}
	}
	if response.Content != `{"schemaVersion":1}` || response.InputTokens != 12 || response.OutputTokens != 8 {
		t.Fatalf("response = %#v", response)
	}
}

func TestHTTPProviderOmitsJSONResponseFormatForTextOutput(t *testing.T) {
	t.Parallel()

	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		writeCompletion(writer, "stop", "plain text")
	}))
	defer server.Close()

	_, err := newTestProvider(t, server.URL).Generate(context.Background(), modelprovider.Request{
		SystemPrompt: "system",
		UserPrompt:   "user",
		MaxTokens:    32,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := body["response_format"]; exists {
		t.Fatalf("text request contains response_format: %#v", body)
	}
	if body["thinking"].(map[string]any)["type"] != "disabled" {
		t.Fatalf("thinking = %#v", body["thinking"])
	}
}

func TestHTTPProviderNormalizesBaseURLWithoutDuplicateV1(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		suffix   string
		wantPath string
	}{
		{name: "root", suffix: "", wantPath: "/v1/chat/completions"},
		{name: "root slash", suffix: "/", wantPath: "/v1/chat/completions"},
		{name: "v1", suffix: "/v1", wantPath: "/v1/chat/completions"},
		{name: "v1 slash", suffix: "/v1/", wantPath: "/v1/chat/completions"},
		{name: "proxy prefix", suffix: "/proxy", wantPath: "/proxy/v1/chat/completions"},
		{name: "proxy v1", suffix: "/proxy/v1", wantPath: "/proxy/v1/chat/completions"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var path string
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				path = request.URL.Path
				writeCompletion(writer, "stop", "ok")
			}))
			defer server.Close()
			_, err := newTestProvider(t, server.URL+test.suffix).Generate(context.Background(), validRequest())
			if err != nil {
				t.Fatal(err)
			}
			if path != test.wantPath {
				t.Fatalf("request path = %q, want %q", path, test.wantPath)
			}
		})
	}
}

func TestHTTPProviderRejectsUnsafeBaseURLMetadata(t *testing.T) {
	t.Parallel()

	for _, baseURL := range []string{
		"https://user@model.example",
		"https://model.example?token=query-secret",
		"https://model.example#fragment-secret",
	} {
		if _, err := modelprovider.NewHTTPProvider(modelprovider.HTTPConfig{
			BaseURL: baseURL,
			APIKey:  "secret",
			Model:   "model",
		}); err == nil {
			t.Fatalf("NewHTTPProvider() accepted unsafe URL %q", baseURL)
		} else if strings.Contains(err.Error(), "query-secret") || strings.Contains(err.Error(), "fragment-secret") {
			t.Fatalf("unsafe URL error leaked metadata: %v", err)
		}
	}
}

func TestHTTPProviderRejectsNonPositiveMaxTokensBeforeRequest(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeCompletion(writer, "stop", "ok")
	}))
	defer server.Close()
	provider := newTestProvider(t, server.URL)
	for _, maxTokens := range []int{0, -1} {
		request := validRequest()
		request.MaxTokens = maxTokens
		if _, err := provider.Generate(context.Background(), request); err == nil {
			t.Fatalf("Generate() accepted MaxTokens=%d", maxTokens)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("upstream calls = %d, want 0", calls.Load())
	}
}

func TestHTTPProviderRejectsEveryNonStopFinishReasonWithoutEcho(t *testing.T) {
	t.Parallel()

	for _, finishReason := range []string{
		"length", "content_filter", "tool_calls", "insufficient_system_resource", "", "unknown-sensitive-finish",
	} {
		t.Run(finishReason, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeCompletion(writer, finishReason, "sensitive-model-content")
			}))
			defer server.Close()
			_, err := newTestProvider(t, server.URL).Generate(context.Background(), modelprovider.Request{
				SystemPrompt: "sensitive-system-prompt",
				UserPrompt:   "sensitive-user-prompt",
				JSONOutput:   true,
				MaxTokens:    8192,
			})
			if err == nil {
				t.Fatalf("Generate() accepted finish_reason=%q", finishReason)
			}
			for _, secret := range []string{finishReason, "sensitive-model-content", "sensitive-system-prompt", "sensitive-user-prompt", "test-secret"} {
				if secret != "" && strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaked upstream value %q: %v", secret, err)
				}
			}
		})
	}
}

func TestHTTPProviderReturnsDecodedUsageForContentErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body map[string]any
	}{
		{
			name: "no choices",
			body: map[string]any{"choices": []any{}},
		},
		{
			name: "length",
			body: completionBody("length", "truncated-sensitive-content"),
		},
		{
			name: "content filter",
			body: completionBody("content_filter", "filtered-sensitive-content"),
		},
		{
			name: "empty content",
			body: completionBody("stop", " \t\n"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.body["usage"] = map[string]any{
				"prompt_tokens":     21,
				"completion_tokens": 34,
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(test.body)
			}))
			defer server.Close()

			response, err := newTestProvider(t, server.URL).Generate(context.Background(), validRequest())
			if err == nil {
				t.Fatal("Generate() error = nil")
			}
			if response.Content != "" || response.InputTokens != 21 || response.OutputTokens != 34 {
				t.Fatalf("error response = %#v", response)
			}
		})
	}
}

func TestHTTPProviderRejectsTrailingResponseJSONWithoutEcho(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(
			`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],` +
				`"usage":{"prompt_tokens":55,"completion_tokens":89}}` +
				`{"trailing":"sensitive-trailing-body"}`,
		))
	}))
	defer server.Close()
	response, err := newTestProvider(t, server.URL).Generate(context.Background(), modelprovider.Request{
		SystemPrompt: "sensitive-system",
		UserPrompt:   "sensitive-user",
		MaxTokens:    32,
	})
	if err == nil {
		t.Fatal("Generate() accepted trailing response JSON")
	}
	if response != (modelprovider.Response{}) {
		t.Fatalf("trailing JSON response = %#v, want zero value", response)
	}
	for _, secret := range []string{"sensitive-trailing-body", "sensitive-system", "sensitive-user", "test-secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("trailing response error leaked %q: %v", secret, err)
		}
	}
}

func TestHTTPProviderPreservesTwoMiBResponseLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeCompletion(writer, "stop", strings.Repeat("x", 2*1024*1024))
	}))
	defer server.Close()
	response, err := newTestProvider(t, server.URL).Generate(context.Background(), validRequest())
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("Generate() oversized response error = %v", err)
	}
	if response != (modelprovider.Response{}) {
		t.Fatalf("oversized response = %#v, want zero value", response)
	}
}

func TestHTTPProviderReturnsZeroResponseForMalformedJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"usage":{"prompt_tokens":55,"completion_tokens":89},`))
	}))
	defer server.Close()

	response, err := newTestProvider(t, server.URL).Generate(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Generate() accepted malformed JSON")
	}
	if response != (modelprovider.Response{}) {
		t.Fatalf("malformed JSON response = %#v, want zero value", response)
	}
}

func TestHTTPProviderRedactsUpstreamBodyCredentialsAndPrompts(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"usage": map[string]any{"prompt_tokens": 55, "completion_tokens": 89},
			"error": "sensitive-upstream-body test-secret sensitive-system sensitive-user",
		})
	}))
	defer server.Close()
	response, err := newTestProvider(t, server.URL).Generate(context.Background(), modelprovider.Request{
		SystemPrompt: "sensitive-system",
		UserPrompt:   "sensitive-user",
		JSONOutput:   true,
		MaxTokens:    8192,
	})
	if err == nil {
		t.Fatal("Generate() error = nil")
	}
	if response != (modelprovider.Response{}) {
		t.Fatalf("HTTP status response = %#v, want zero value", response)
	}
	for _, secret := range []string{"sensitive-upstream-body", "test-secret", "sensitive-system", "sensitive-user"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked %q: %v", secret, err)
		}
	}
}

func TestHTTPProviderReturnsZeroResponseForTransportFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	provider := newTestProvider(t, server.URL)
	server.Close()

	response, err := provider.Generate(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Generate() transport error = nil")
	}
	if response != (modelprovider.Response{}) {
		t.Fatalf("transport error response = %#v, want zero value", response)
	}
}

func TestHTTPProviderDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	var targetCalls atomic.Int32
	var targetAuthorization atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		targetCalls.Add(1)
		targetAuthorization.Store(request.Header.Get("Authorization"))
		writeCompletion(writer, "stop", "unexpected")
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	response, err := newTestProvider(t, redirect.URL).Generate(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Generate() followed a redirect")
	}
	if response != (modelprovider.Response{}) {
		t.Fatalf("redirect response = %#v, want zero value", response)
	}
	if calls := targetCalls.Load(); calls != 0 {
		t.Fatalf("redirect target calls = %d, Authorization = %q", calls, targetAuthorization.Load())
	}
}

func TestHTTPProviderPreservesContextCancellationWithoutLeakingRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeCompletion(writer, "stop", "ok")
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response, err := newTestProvider(t, server.URL).Generate(ctx, modelprovider.Request{
		SystemPrompt: "sensitive-system",
		UserPrompt:   "sensitive-user",
		MaxTokens:    1,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate() error = %v, want context.Canceled", err)
	}
	if response != (modelprovider.Response{}) {
		t.Fatalf("cancelled response = %#v, want zero value", response)
	}
	for _, secret := range []string{"sensitive-system", "sensitive-user", "test-secret", server.URL} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("cancellation error leaked %q: %v", secret, err)
		}
	}
}

func TestHTTPProviderRequiresHTTPSAndEnvironmentSecrets(t *testing.T) {
	t.Parallel()

	if _, err := modelprovider.NewHTTPProvider(modelprovider.HTTPConfig{
		BaseURL: "http://model.example",
		APIKey:  "secret",
		Model:   "model",
	}); err == nil {
		t.Fatal("NewHTTPProvider() accepted insecure URL")
	}
	if _, err := modelprovider.NewHTTPProviderFromEnvironment(map[string]string{}); err == nil {
		t.Fatal("NewHTTPProviderFromEnvironment() accepted missing config")
	}
}

func newTestProvider(t *testing.T, baseURL string) *modelprovider.HTTPProvider {
	t.Helper()
	provider, err := modelprovider.NewHTTPProvider(modelprovider.HTTPConfig{
		BaseURL:       baseURL,
		APIKey:        "test-secret",
		Model:         "test-model",
		AllowInsecure: true,
		Timeout:       time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func validRequest() modelprovider.Request {
	return modelprovider.Request{
		SystemPrompt: "system",
		UserPrompt:   "user",
		JSONOutput:   true,
		MaxTokens:    8192,
	}
}

func writeCompletion(writer http.ResponseWriter, finishReason, content string) {
	writer.Header().Set("Content-Type", "application/json")
	body := completionBody(finishReason, content)
	body["usage"] = map[string]any{
		"prompt_tokens":     12,
		"completion_tokens": 8,
		"total_tokens":      20,
	}
	_ = json.NewEncoder(writer).Encode(body)
}

func completionBody(finishReason, content string) map[string]any {
	return map[string]any{
		"id":     "response-1",
		"object": "chat.completion",
		"choices": []any{map[string]any{
			"index":         0,
			"finish_reason": finishReason,
			"message": map[string]any{
				"role":    "assistant",
				"content": content,
			},
		}},
	}
}
