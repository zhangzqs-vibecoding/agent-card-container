package modelprovider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

func TestHTTPProviderClassifiesRetryableHTTPStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status        int
		wantRetryable bool
	}{
		{status: http.StatusRequestTimeout, wantRetryable: true},
		{status: http.StatusTooManyRequests, wantRetryable: true},
		{status: http.StatusInternalServerError, wantRetryable: true},
		{status: http.StatusBadGateway, wantRetryable: true},
		{status: http.StatusServiceUnavailable, wantRetryable: true},
		{status: http.StatusGatewayTimeout, wantRetryable: true},
		{status: 599, wantRetryable: true},
		{status: http.StatusBadRequest, wantRetryable: false},
		{status: http.StatusUnauthorized, wantRetryable: false},
		{status: http.StatusNotFound, wantRetryable: false},
		{status: 499, wantRetryable: false},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("status_%d", test.status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte("sensitive-upstream-body"))
			}))
			defer server.Close()

			_, err := newTestProvider(t, server.URL).Generate(context.Background(), modelprovider.Request{
				SystemPrompt: "sensitive-system-prompt",
				UserPrompt:   "sensitive-user-prompt",
				MaxTokens:    32,
			})
			if err == nil {
				t.Fatal("Generate() error = nil")
			}
			if got := modelprovider.IsRetryable(err); got != test.wantRetryable {
				t.Fatalf("IsRetryable(error) = %t, want %t; error = %v", got, test.wantRetryable, err)
			}
			assertErrorOmits(t, err,
				"sensitive-upstream-body",
				"sensitive-system-prompt",
				"sensitive-user-prompt",
				"test-secret",
			)
		})
	}
}

func TestHTTPProviderClassifiesOnlyResourceExhaustionFinishAsRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		finishReason  string
		wantRetryable bool
	}{
		{finishReason: "insufficient_system_resource", wantRetryable: true},
		{finishReason: "length", wantRetryable: false},
		{finishReason: "content_filter", wantRetryable: false},
		{finishReason: "tool_calls", wantRetryable: false},
		{finishReason: "unknown-sensitive-finish", wantRetryable: false},
		{finishReason: "", wantRetryable: false},
	}
	for _, test := range tests {
		t.Run(test.finishReason, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeCompletion(writer, test.finishReason, "sensitive-model-content")
			}))
			defer server.Close()

			response, err := newTestProvider(t, server.URL).Generate(context.Background(), validRequest())
			if err == nil {
				t.Fatal("Generate() error = nil")
			}
			if got := modelprovider.IsRetryable(err); got != test.wantRetryable {
				t.Fatalf("IsRetryable(error) = %t, want %t; error = %v", got, test.wantRetryable, err)
			}
			if response.Content != "" || response.InputTokens != 12 || response.OutputTokens != 8 {
				t.Fatalf("error response = %#v, want usage-only response", response)
			}
			assertErrorOmits(t, err, test.finishReason, "sensitive-model-content", "test-secret")
		})
	}
}

func TestHTTPProviderClassifiesTransportFailureAsRetryableUnlessContextCancelled(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	provider := newTestProvider(t, server.URL)
	server.Close()

	_, err := provider.Generate(context.Background(), validRequest())
	if err == nil || !modelprovider.IsRetryable(err) {
		t.Fatalf("transport error = %v, IsRetryable = %t", err, modelprovider.IsRetryable(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.Generate(ctx, validRequest())
	if !errors.Is(err, context.Canceled) || modelprovider.IsRetryable(err) {
		t.Fatalf("cancelled error = %v, IsRetryable = %t", err, modelprovider.IsRetryable(err))
	}
}

func TestHTTPProviderClassifiesResponseReadFailureAsRetryable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not support hijacking")
			return
		}
		connection, buffer, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack() error = %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprint(buffer, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 1024\r\n\r\n{sensitive-read-body")
		_ = buffer.Flush()
	}))
	defer server.Close()

	_, err := newTestProvider(t, server.URL).Generate(context.Background(), validRequest())
	if err == nil || !modelprovider.IsRetryable(err) {
		t.Fatalf("read error = %v, IsRetryable = %t", err, modelprovider.IsRetryable(err))
	}
	assertErrorOmits(t, err, "sensitive-read-body", "test-secret", "system", "user")
}

func TestHTTPProviderKeepsResponseReadCancellationNonRetryable(t *testing.T) {
	t.Parallel()

	responseStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Content-Length", "1024")
		writer.WriteHeader(http.StatusOK)
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter does not support flushing")
			return
		}
		flusher.Flush()
		close(responseStarted)
		<-request.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := newTestProvider(t, server.URL)
	type result struct {
		response modelprovider.Response
		err      error
	}
	resultChannel := make(chan result, 1)
	go func() {
		response, err := provider.Generate(ctx, validRequest())
		resultChannel <- result{response: response, err: err}
	}()

	select {
	case <-responseStarted:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("model response did not start")
	}

	select {
	case got := <-resultChannel:
		if !errors.Is(got.err, context.Canceled) || modelprovider.IsRetryable(got.err) {
			t.Fatalf("read cancellation error = %v, IsRetryable = %t", got.err, modelprovider.IsRetryable(got.err))
		}
		if got.response != (modelprovider.Response{}) {
			t.Fatalf("read cancellation response = %#v, want zero value", got.response)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Generate() did not return after cancellation")
	}
}

func TestHTTPProviderKeepsDecodeAndEmptyResponseErrorsNonRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"choices":[`},
		{name: "no choices", body: `{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2}}`},
		{name: "empty content", body: mustEncodeCompletion(t, "stop", " \t\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()

			_, err := newTestProvider(t, server.URL).Generate(context.Background(), validRequest())
			if err == nil {
				t.Fatal("Generate() error = nil")
			}
			if modelprovider.IsRetryable(err) {
				t.Fatalf("IsRetryable(error) = true; error = %v", err)
			}
		})
	}
}

func mustEncodeCompletion(t *testing.T, finishReason, content string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": finishReason,
			"message": map[string]any{
				"role":    "assistant",
				"content": content,
			},
		}},
		"usage": map[string]any{
			"prompt_tokens":     1,
			"completion_tokens": 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func assertErrorOmits(t *testing.T, err error, values ...string) {
	t.Helper()
	for _, value := range values {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Errorf("error leaked %q: %v", value, err)
		}
	}
}
