package bootstrap_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zzq/agent-card-container/services/cloud/internal/bootstrap"
)

func TestProductionRuntimesSharePostgresJobsAndS3Artifacts(t *testing.T) {
	dsn := os.Getenv("AGENTCARD_POSTGRES_TEST_DSN")
	s3Endpoint := os.Getenv("AGENTCARD_S3_TEST_ENDPOINT")
	if dsn == "" || s3Endpoint == "" {
		t.Skip("production persistence test environment is not configured")
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	model := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": `{"schemaVersion":1,"initialState":{"title":"持久化"},"root":{"id":"root","type":"Text","props":{"text":{"path":"state.title"}}}}`,
				},
			}},
		})
	}))
	defer model.Close()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{
		"AGENTCARD_DEV_TOKEN": "dev-token", "AGENTCARD_DEV_USER": "user-01",
		"AGENTCARD_MODEL_BASE_URL": model.URL, "AGENTCARD_MODEL_API_KEY": "model-secret",
		"AGENTCARD_MODEL": "deepseek-v4-flash", "AGENTCARD_MODEL_ALLOW_INSECURE": "true",
		"AGENTCARD_SIGNING_KEY_ID":      "test-key",
		"AGENTCARD_SIGNING_PRIVATE_KEY": base64.RawStdEncoding.EncodeToString(privateKey),
		"AGENTCARD_DATABASE_URL":        dsn, "AGENTCARD_S3_ENDPOINT": s3Endpoint,
		"AGENTCARD_S3_ACCESS_KEY": os.Getenv("AGENTCARD_S3_TEST_ACCESS_KEY"),
		"AGENTCARD_S3_SECRET_KEY": os.Getenv("AGENTCARD_S3_TEST_SECRET_KEY"),
		"AGENTCARD_S3_BUCKET":     os.Getenv("AGENTCARD_S3_TEST_BUCKET"),
		"AGENTCARD_S3_REGION":     "us-east-1", "AGENTCARD_S3_SECURE": "false",
	}
	apiRuntime, err := bootstrap.NewFromEnvironment(environment)
	if err != nil {
		t.Fatal(err)
	}
	defer apiRuntime.Close()
	workerRuntime, err := bootstrap.NewFromEnvironment(environment)
	if err != nil {
		t.Fatal(err)
	}
	defer workerRuntime.Close()

	created := requestJSON(t, apiRuntime.Handler(), http.MethodPost, "/v1/generations", map[string]any{
		"prompt": "生成持久化离线卡片",
		"target": "native",
	})
	sessionID := created["id"].(string)
	requestJSON(t, apiRuntime.Handler(), http.MethodPost, "/v1/generations/"+sessionID+"/confirm", nil)
	if _, err := workerRuntime.RunWorkerOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var ready map[string]any
	for time.Now().Before(deadline) {
		ready = requestJSON(t, apiRuntime.Handler(), http.MethodGet, "/v1/generations/"+sessionID, nil)
		if ready["status"] == "ready" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if ready["status"] != "ready" {
		t.Fatalf("ready = %#v", ready)
	}
	cards := requestJSON(t, apiRuntime.Handler(), http.MethodGet, "/v1/cards", nil)
	if len(cards["cards"].([]any)) != 1 {
		t.Fatalf("cards = %#v", cards)
	}
}

func TestRuntimeConnectsAPIWorkerModelSigningAndCatalog(t *testing.T) {
	t.Parallel()

	model := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "stop",
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
	defer runtime.Close()

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
