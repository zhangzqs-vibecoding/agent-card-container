package bootstrap_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zzq/agent-card-container/services/cloud/internal/bootstrap"
)

func TestRuntimeConnectsAPIWorkerModelSigningAndCatalog(t *testing.T) {
	t.Parallel()

	model := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": `{"schemaVersion":1,"initialState":{"title":"完成"},"root":{"id":"root","type":"Text","props":{"text":{"path":"state.title"}}}}`,
				},
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 10},
		})
	}))
	defer model.Close()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := bootstrap.NewFromEnvironment(map[string]string{
		"AGENTCARD_DEV_TOKEN":            "dev-token",
		"AGENTCARD_DEV_USER":             "user-01",
		"AGENTCARD_MODEL_BASE_URL":       model.URL,
		"AGENTCARD_MODEL_API_KEY":        "model-secret",
		"AGENTCARD_MODEL":                "deepseek-v4-flash",
		"AGENTCARD_MODEL_ALLOW_INSECURE": "true",
		"AGENTCARD_SIGNING_KEY_ID":       "test-key",
		"AGENTCARD_SIGNING_PRIVATE_KEY":  base64.RawStdEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		t.Fatalf("NewFromEnvironment() error = %v", err)
	}

	created := requestJSON(t, runtime.Handler(), http.MethodPost, "/v1/generations", map[string]any{
		"prompt": "做一个离线文本卡片",
		"target": "auto",
	})
	sessionID := created["id"].(string)
	requestJSON(t, runtime.Handler(), http.MethodPost, "/v1/generations/"+sessionID+"/confirm", nil)

	if _, err := runtime.RunWorkerOnce(context.Background()); err != nil {
		t.Fatalf("RunWorkerOnce() error = %v", err)
	}
	ready := requestJSON(t, runtime.Handler(), http.MethodGet, "/v1/generations/"+sessionID, nil)
	if ready["status"] != "ready" || ready["versionId"] == "" {
		t.Fatalf("ready = %#v", ready)
	}
	cards := requestJSON(t, runtime.Handler(), http.MethodGet, "/v1/cards", nil)
	if len(cards["cards"].([]any)) != 1 {
		t.Fatalf("cards = %#v", cards)
	}
}

func TestRuntimeRejectsMissingSecrets(t *testing.T) {
	t.Parallel()

	if _, err := bootstrap.NewFromEnvironment(map[string]string{}); err == nil {
		t.Fatal("NewFromEnvironment() accepted missing auth/model/signing config")
	}
}

func requestJSON(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body map[string]any,
) map[string]any {
	t.Helper()
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &encoded)
	request.Header.Set("Authorization", "Bearer dev-token")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code < 200 || response.Code >= 300 {
		t.Fatalf("%s %s status=%d body=%s", method, path, response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
