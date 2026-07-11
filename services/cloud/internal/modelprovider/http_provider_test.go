package modelprovider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

func TestHTTPProviderCallsConfiguredModelWithoutExposingCredentials(t *testing.T) {
	t.Parallel()

	var gotAuthorization string
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAuthorization = request.Header.Get("Authorization")
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel = body.Model
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"schemaVersion\":1}"}}],"usage":{"prompt_tokens":12,"completion_tokens":8}}`))
	}))
	defer server.Close()

	provider, err := modelprovider.NewHTTPProvider(modelprovider.HTTPConfig{
		BaseURL:       server.URL,
		APIKey:        "test-secret",
		Model:         "test-model",
		AllowInsecure: true,
		Timeout:       time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), modelprovider.Request{
		SessionID: "gen_01",
		Prompt:    "做一个卡片",
		Locale:    "zh-CN",
		Runtime:   "native",
		Attempt:   1,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if gotAuthorization != "Bearer test-secret" {
		t.Fatalf("Authorization = %q", gotAuthorization)
	}
	if gotModel != "test-model" {
		t.Fatalf("Model = %q", gotModel)
	}
	if response.Content != `{"schemaVersion":1}` || response.InputTokens != 12 || response.OutputTokens != 8 {
		t.Fatalf("response = %#v", response)
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
