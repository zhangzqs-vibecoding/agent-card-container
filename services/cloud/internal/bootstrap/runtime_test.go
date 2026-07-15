package bootstrap_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zzq/agent-card-container/services/cloud/internal/bootstrap"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
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
	publicKey, privateKey := productionSigningKey(t)
	environment := map[string]string{
		"AGENTCARD_DEV_TOKEN": "dev-token", "AGENTCARD_DEV_USER": "user-01",
		"AGENTCARD_MODEL_BASE_URL": model.URL, "AGENTCARD_MODEL_API_KEY": "model-secret",
		"AGENTCARD_MODEL": "deepseek-v4-flash", "AGENTCARD_MODEL_ALLOW_INSECURE": "true",
		"AGENTCARD_SIGNING_KEY_ID":       "test-key",
		"AGENTCARD_SIGNING_PRIVATE_KEY":  base64.RawStdEncoding.EncodeToString(privateKey),
		"AGENTCARD_PERSISTENCE_REQUIRED": "true",
		"AGENTCARD_DATABASE_URL":         dsn, "AGENTCARD_S3_ENDPOINT": s3Endpoint,
		"AGENTCARD_S3_ACCESS_KEY": os.Getenv("AGENTCARD_S3_TEST_ACCESS_KEY"),
		"AGENTCARD_S3_SECRET_KEY": os.Getenv("AGENTCARD_S3_TEST_SECRET_KEY"),
		"AGENTCARD_S3_BUCKET":     os.Getenv("AGENTCARD_S3_TEST_BUCKET"),
		"AGENTCARD_S3_REGION":     "us-east-1", "AGENTCARD_S3_SECURE": "false",
	}
	apiRuntime, err := bootstrap.NewFromEnvironment(environment)
	if err != nil {
		t.Fatal(err)
	}
	workerRuntime, err := bootstrap.NewFromEnvironment(environment)
	if err != nil {
		t.Fatal(err)
	}

	created := requestJSON(t, apiRuntime.Handler(), http.MethodPost, "/v1/generations", map[string]any{
		"prompt": "生成持久化离线卡片",
		"target": "native",
	})
	sessionID := created["id"].(string)
	requestJSON(t, apiRuntime.Handler(), http.MethodPost, "/v1/generations/"+sessionID+"/confirm", nil)
	jobID, jobStatus := productionJob(t, dsn, sessionID)
	if jobStatus != "queued" {
		t.Fatalf("job status = %q, want queued", jobStatus)
	}
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
	card := cards["cards"].([]any)[0].(map[string]any)
	cardID := card["cardId"].(string)
	latest := card["latestVersion"].(map[string]any)
	versionID := latest["versionId"].(string)
	artifactSHA256 := latest["artifactSha256"].(string)

	if err := workerRuntime.Close(); err != nil {
		t.Fatal(err)
	}
	if err := apiRuntime.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := bootstrap.NewFromEnvironment(environment)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()

	restored := requestJSON(t, restarted.Handler(), http.MethodGet, "/v1/generations/"+sessionID, nil)
	if restored["status"] != "ready" || restored["versionId"] != versionID {
		t.Fatalf("restored session = %#v", restored)
	}
	restoredJobID, restoredJobStatus := productionJob(t, dsn, sessionID)
	if restoredJobID != jobID || restoredJobStatus != "completed" {
		t.Fatalf("restored job = (%q, %q), want (%q, completed)", restoredJobID, restoredJobStatus, jobID)
	}
	detail := requestJSON(t, restarted.Handler(), http.MethodGet, "/v1/cards/"+cardID, nil)
	versions := detail["versions"].([]any)
	if len(versions) != 1 || versions[0].(map[string]any)["versionId"] != versionID {
		t.Fatalf("restored card = %#v", detail)
	}
	download := requestJSON(
		t,
		restarted.Handler(),
		http.MethodGet,
		"/v1/cards/"+cardID+"/versions/"+versionID+"/artifact",
		nil,
	)
	artifactURL := download["url"].(string)
	if strings.HasPrefix(artifactURL, "memory://") {
		t.Fatalf("persistent runtime returned memory URL")
	}
	archive := downloadProductionArtifact(t, artifactURL)
	version := publish.CardVersion{
		CardID:         cardID,
		VersionID:      versionID,
		ArtifactSHA256: artifactSHA256,
	}
	if err := verifyVerticalArchive(archive, version, "test-key", publicKey); err != nil {
		t.Fatalf("verify restored artifact: %v", err)
	}
	t.Logf(
		"P0-B artifact evidence: session=%s job=%s card=%s version=%s sha256=%s keyId=%s",
		sessionID, jobID, cardID, versionID, artifactSHA256, "test-key",
	)
}

func TestProductionPersistenceRestoredSnapshot(t *testing.T) {
	if os.Getenv("AGENTCARD_P0B_RESTORE_VERIFY") != "1" {
		t.Skip("P0-B restored snapshot verification is not enabled")
	}
	dsn := os.Getenv("AGENTCARD_POSTGRES_TEST_DSN")
	s3Endpoint := os.Getenv("AGENTCARD_S3_TEST_ENDPOINT")
	if dsn == "" || s3Endpoint == "" {
		t.Skip("production persistence test environment is not configured")
	}
	publicKey, privateKey := productionSigningKey(t)
	model := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{"choices": []any{}})
	}))
	defer model.Close()
	runtime, err := bootstrap.NewFromEnvironment(productionEnvironment(model.URL, dsn, s3Endpoint, privateKey))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var sessionID, jobID, jobStatus, cardID, versionID, artifactSHA256, keyID string
	if err := database.QueryRow(`
		SELECT session.id, job.id, job.status, version.card_id,
		       version.version_id, version.artifact_sha256, version.key_id
		FROM generation_sessions AS session
		JOIN generation_jobs AS job ON job.session_id = session.id
		JOIN card_versions AS version ON version.version_id = session.version_id
		WHERE session.status = 'ready'
		ORDER BY session.created_at
		LIMIT 1
	`).Scan(&sessionID, &jobID, &jobStatus, &cardID, &versionID, &artifactSHA256, &keyID); err != nil {
		t.Fatal(err)
	}
	if jobID == "" || jobStatus != "completed" {
		t.Fatalf("restored job status = %q", jobStatus)
	}
	session := requestJSON(t, runtime.Handler(), http.MethodGet, "/v1/generations/"+sessionID, nil)
	if session["status"] != "ready" || session["versionId"] != versionID {
		t.Fatalf("restored session = %#v", session)
	}
	detail := requestJSON(t, runtime.Handler(), http.MethodGet, "/v1/cards/"+cardID, nil)
	if len(detail["versions"].([]any)) == 0 {
		t.Fatalf("restored card has no versions")
	}
	download := requestJSON(
		t,
		runtime.Handler(),
		http.MethodGet,
		"/v1/cards/"+cardID+"/versions/"+versionID+"/artifact",
		nil,
	)
	archive := downloadProductionArtifact(t, download["url"].(string))
	if err := verifyVerticalArchive(archive, publish.CardVersion{
		CardID: cardID, VersionID: versionID, ArtifactSHA256: artifactSHA256,
	}, keyID, publicKey); err != nil {
		t.Fatalf("verify restored snapshot artifact: %v", err)
	}
}

func productionSigningKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	encoded := os.Getenv("AGENTCARD_P0B_TEST_SIGNING_PRIVATE_KEY")
	if encoded == "" {
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return publicKey, privateKey
	}
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(encoded)
	}
	if err != nil {
		t.Fatal("decode P0-B test signing key")
	}
	var privateKey ed25519.PrivateKey
	switch len(decoded) {
	case ed25519.SeedSize:
		privateKey = ed25519.NewKeyFromSeed(decoded)
	case ed25519.PrivateKeySize:
		privateKey = ed25519.PrivateKey(decoded)
	default:
		t.Fatal("P0-B test signing key length is invalid")
	}
	return privateKey.Public().(ed25519.PublicKey), privateKey
}

func TestProductionSigningKeyAcceptsStandardBase64Seed(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTCARD_P0B_TEST_SIGNING_PRIVATE_KEY", base64.StdEncoding.EncodeToString(seed))

	publicKey, privateKey := productionSigningKey(t)
	if len(publicKey) != ed25519.PublicKeySize || len(privateKey) != ed25519.PrivateKeySize {
		t.Fatalf("key sizes = (%d, %d)", len(publicKey), len(privateKey))
	}
}

func productionEnvironment(
	modelURL, dsn, s3Endpoint string,
	privateKey ed25519.PrivateKey,
) map[string]string {
	return map[string]string{
		"AGENTCARD_DEV_TOKEN": "dev-token", "AGENTCARD_DEV_USER": "user-01",
		"AGENTCARD_MODEL_BASE_URL": modelURL, "AGENTCARD_MODEL_API_KEY": "model-secret",
		"AGENTCARD_MODEL": "deepseek-v4-flash", "AGENTCARD_MODEL_ALLOW_INSECURE": "true",
		"AGENTCARD_SIGNING_KEY_ID":       "test-key",
		"AGENTCARD_SIGNING_PRIVATE_KEY":  base64.RawStdEncoding.EncodeToString(privateKey),
		"AGENTCARD_PERSISTENCE_REQUIRED": "true",
		"AGENTCARD_DATABASE_URL":         dsn,
		"AGENTCARD_S3_ENDPOINT":          s3Endpoint,
		"AGENTCARD_S3_ACCESS_KEY":        os.Getenv("AGENTCARD_S3_TEST_ACCESS_KEY"),
		"AGENTCARD_S3_SECRET_KEY":        os.Getenv("AGENTCARD_S3_TEST_SECRET_KEY"),
		"AGENTCARD_S3_BUCKET":            os.Getenv("AGENTCARD_S3_TEST_BUCKET"),
		"AGENTCARD_S3_REGION":            "us-east-1",
		"AGENTCARD_S3_SECURE":            "false",
	}
}

func TestProductionReadinessFailureMatrix(t *testing.T) {
	dsn := os.Getenv("AGENTCARD_POSTGRES_TEST_DSN")
	s3Endpoint := os.Getenv("AGENTCARD_S3_TEST_ENDPOINT")
	postgresContainer := os.Getenv("AGENTCARD_POSTGRES_TEST_CONTAINER")
	minioContainer := os.Getenv("AGENTCARD_S3_TEST_CONTAINER")
	if dsn == "" || s3Endpoint == "" || postgresContainer == "" || minioContainer == "" {
		t.Skip("controlled production persistence test environment is not configured")
	}
	model := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{"choices": []any{}})
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
		"AGENTCARD_SIGNING_KEY_ID":       "test-key",
		"AGENTCARD_SIGNING_PRIVATE_KEY":  base64.RawStdEncoding.EncodeToString(privateKey),
		"AGENTCARD_PERSISTENCE_REQUIRED": "true",
		"AGENTCARD_DATABASE_URL":         dsn,
		"AGENTCARD_S3_ENDPOINT":          s3Endpoint,
		"AGENTCARD_S3_ACCESS_KEY":        os.Getenv("AGENTCARD_S3_TEST_ACCESS_KEY"),
		"AGENTCARD_S3_SECRET_KEY":        os.Getenv("AGENTCARD_S3_TEST_SECRET_KEY"),
		"AGENTCARD_S3_BUCKET":            os.Getenv("AGENTCARD_S3_TEST_BUCKET"),
		"AGENTCARD_S3_REGION":            "us-east-1",
		"AGENTCARD_S3_SECURE":            "false",
	}
	runtime, err := bootstrap.NewFromEnvironment(environment)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	t.Cleanup(func() {
		_ = exec.Command("docker", "unpause", postgresContainer).Run()
		_ = exec.Command("docker", "unpause", minioContainer).Run()
	})

	assertRuntimeStatus(t, runtime.Handler(), "/healthz", http.StatusOK)
	assertRuntimeStatus(t, runtime.Handler(), "/readyz", http.StatusOK)

	dockerContainerAction(t, "pause", postgresContainer)
	assertRuntimeStatus(t, runtime.Handler(), "/healthz", http.StatusOK)
	assertRuntimeStatus(t, runtime.Handler(), "/readyz", http.StatusServiceUnavailable)
	dockerContainerAction(t, "unpause", postgresContainer)
	assertRuntimeStatus(t, runtime.Handler(), "/readyz", http.StatusOK)

	dockerContainerAction(t, "pause", minioContainer)
	assertRuntimeStatus(t, runtime.Handler(), "/healthz", http.StatusOK)
	assertRuntimeStatus(t, runtime.Handler(), "/readyz", http.StatusServiceUnavailable)
	dockerContainerAction(t, "unpause", minioContainer)
	assertRuntimeStatus(t, runtime.Handler(), "/readyz", http.StatusOK)
}

func dockerContainerAction(t *testing.T, action, container string) {
	t.Helper()
	if err := exec.Command("docker", action, container).Run(); err != nil {
		t.Fatalf("docker %s test dependency: %v", action, err)
	}
}

func assertRuntimeStatus(t *testing.T, handler http.Handler, path string, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("GET %s status = %d, want %d", path, response.Code, want)
	}
}

func productionJob(t *testing.T, dsn, sessionID string) (string, string) {
	t.Helper()
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var id, status string
	if err := database.QueryRow(
		`SELECT id, status FROM generation_jobs WHERE session_id = $1`,
		sessionID,
	).Scan(&id, &status); err != nil {
		t.Fatal(err)
	}
	return id, status
}

func downloadProductionArtifact(t *testing.T, rawURL string) []byte {
	t.Helper()
	origin, err := url.Parse(rawURL)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" {
		t.Fatalf("artifact URL is invalid")
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			if request.URL.Scheme != origin.Scheme || request.URL.Host != origin.Host {
				return fmt.Errorf("artifact redirect changed origin")
			}
			return nil
		},
	}
	response, err := client.Get(rawURL)
	if err != nil {
		t.Fatalf("download artifact: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("download artifact status = %d", response.StatusCode)
	}
	archive, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil || len(archive) == 0 || len(archive) > 8*1024*1024 {
		t.Fatalf("artifact response size is invalid")
	}
	return archive
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
