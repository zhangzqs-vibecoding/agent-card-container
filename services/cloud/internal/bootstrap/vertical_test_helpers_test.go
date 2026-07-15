package bootstrap_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/httpapi"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
	"github.com/zzq/agent-card-container/services/cloud/internal/worker"
)

const (
	verticalUserID     = "vertical-test-user"
	verticalSigningKey = "p0a-vertical-test-key"
)

var verticalNow = time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

type verticalCase struct {
	ID       string
	Prompt   string
	Messages [2]string
}

type verticalHarness struct {
	server      *httptest.Server
	client      *http.Client
	runner      *worker.Worker
	generations *generation.Service
	versions    *publish.MemoryVersionRepository
	objects     *verticalObjectStore
	publicKey   ed25519.PublicKey
	keyID       string
	bearer      string
}

type verticalCaseResult struct {
	evalCase        verticalCase
	confirmed       generation.RequirementSnapshot
	ready           generation.Session
	outcome         worker.Outcome
	card            publish.CardSummary
	detail          publish.CardDetail
	download        publish.Download
	storedVersion   publish.CardVersion
	artifactArchive []byte
}

type verticalEvidence struct {
	attempts       int
	artifactSHA256 string
}

type verticalFailureDiagnostic struct {
	errorCode string
	category  string
}

type verticalObjectStore struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

type verticalStaticProvider struct{}

func verticalCases() []verticalCase {
	return []verticalCase{
		{
			ID:     "timer",
			Prompt: "生成一个完全离线的番茄钟卡片，显示剩余时间并提供基本控制。",
			Messages: [2]string{
				"补充：显示专注次数和剩余时间。",
				"补充：提供开始、暂停和重置按钮。",
			},
		},
		{
			ID:     "dashboard",
			Prompt: "生成一个使用固定示例数据的离线学习进度面板。",
			Messages: [2]string{
				"补充：展示进度、关键数值和当前状态。",
				"补充：所有交互和数据都保留在卡片内部。",
			},
		},
		{
			ID:     "form-list",
			Prompt: "生成一个离线任务表单和清单卡片。",
			Messages: [2]string{
				"补充：包含输入、选择、勾选和列表展示。",
				"补充：提供保存和重置操作，不访问网络。",
			},
		},
	}
}

func (verticalStaticProvider) Generate(
	context.Context,
	modelprovider.Request,
) (modelprovider.Response, error) {
	return modelprovider.Response{
		Content:      `{"schemaVersion":1,"initialState":{"remaining":1500},"root":{"id":"root","type":"Column","props":{"spacing":8},"children":[{"id":"time","type":"Text","props":{"text":"25:00","style":"display"}},{"id":"actions","type":"Row","props":{"spacing":8},"children":[{"id":"start","type":"Button","props":{"label":"开始"}},{"id":"pause","type":"Button","props":{"label":"暂停"}},{"id":"reset","type":"Button","props":{"label":"重置"}}]}]}}`,
		InputTokens:  11,
		OutputTokens: 17,
	}, nil
}

func newVerticalHarness(t *testing.T, provider modelprovider.Provider) *verticalHarness {
	t.Helper()

	seed := sha256.Sum256([]byte("p0a-deepseek-vertical-signing-key-v1"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)
	bearer := verticalTestBearer()
	generationRepository := generation.NewMemoryRepository()
	jobStore := jobs.NewMemoryStore(sequentialID("job_vertical_"))
	versions := publish.NewMemoryVersionRepository()
	objects := &verticalObjectStore{objects: make(map[string][]byte)}
	service := generation.NewService(
		generationRepository,
		sequentialID("gen_vertical_"),
		func() time.Time { return verticalNow },
		generation.WithJobQueue(jobs.NewGenerationQueue(jobStore)),
	)
	publisher := publish.NewPublisher(
		artifact.NewBuilder(verticalSigningKey, privateKey),
		objects,
		versions,
	)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	codingAgent := agent.NewCodingAgent(
		provider,
		agent.NewNativeValidator(),
		agent.WithLogger(logger),
		agent.WithModelName("vertical-test-model"),
	)
	runner := worker.New(worker.Config{
		WorkerID:     "worker-vertical",
		Jobs:         jobStore,
		Generations:  service,
		Agent:        codingAgent,
		Publisher:    publisher,
		NewCardID:    sequentialID("card_vertical_"),
		NewVersionID: sequentialID("ver_vertical_"),
		Now:          func() time.Time { return verticalNow },
		Logger:       logger,
	})
	handler := httpapi.NewCloudHandler(httpapi.CloudHandlerConfig{
		ServiceName:   "agent-card-vertical-test",
		Generations:   service,
		Publisher:     publisher,
		Authenticator: httpapi.StaticBearerAuthenticator{bearer: verticalUserID},
		NewRequestID:  sequentialID("req_vertical_"),
		Logger:        logger,
		Now:           func() time.Time { return verticalNow },
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &verticalHarness{
		server:      server,
		client:      &http.Client{Timeout: 15 * time.Second},
		runner:      runner,
		generations: service,
		versions:    versions,
		objects:     objects,
		publicKey:   publicKey,
		keyID:       verticalSigningKey,
		bearer:      bearer,
	}
}

func runVerticalCase(
	t *testing.T,
	ctx context.Context,
	harness *verticalHarness,
	evalCase verticalCase,
) verticalCaseResult {
	t.Helper()

	var created generation.Session
	harness.requestJSON(t, ctx, http.MethodPost, "/v1/generations", map[string]any{
		"prompt": evalCase.Prompt,
		"target": generation.TargetNative,
		"locale": "zh-CN",
	}, &created)
	for _, message := range evalCase.Messages {
		var updated generation.Session
		harness.requestJSON(t, ctx, http.MethodPost, "/v1/generations/"+created.ID+"/messages", map[string]any{
			"content": message,
		}, &updated)
	}
	var confirmed generation.Session
	harness.requestJSON(t, ctx, http.MethodPost, "/v1/generations/"+created.ID+"/confirm", nil, &confirmed)
	if !verticalSnapshotMatches(confirmed.ConfirmedRequirement, evalCase) {
		t.Fatal("confirmed requirement snapshot is incomplete")
	}
	var frozenBefore generation.Session
	harness.requestJSON(t, ctx, http.MethodGet, "/v1/generations/"+created.ID, nil, &frozenBefore)
	if !verticalSnapshotMatches(frozenBefore.ConfirmedRequirement, evalCase) {
		t.Fatal("fetched requirement snapshot is incomplete")
	}

	outcome, err := harness.runner.RunOnce(ctx)
	if err != nil {
		diagnostic := verticalWorkerDiagnostic(ctx, harness, created.ID, err)
		t.Fatal(verticalWorkerFailureMessage(diagnostic))
	}
	var ready generation.Session
	harness.requestJSON(t, ctx, http.MethodGet, "/v1/generations/"+created.ID, nil, &ready)
	if ready.ConfirmedRequirement == nil ||
		!verticalSnapshotsEqual(*frozenBefore.ConfirmedRequirement, *ready.ConfirmedRequirement) {
		t.Fatal("confirmed requirement snapshot changed after worker execution")
	}

	var listed struct {
		Cards []publish.CardSummary `json:"cards"`
	}
	harness.requestJSON(t, ctx, http.MethodGet, "/v1/cards", nil, &listed)
	var card publish.CardSummary
	for _, candidate := range listed.Cards {
		if candidate.LatestVersion.VersionID == outcome.VersionID {
			card = candidate
			break
		}
	}
	if card.CardID == "" {
		t.Fatal("published card is missing from cards API")
	}
	var detail publish.CardDetail
	harness.requestJSON(t, ctx, http.MethodGet, "/v1/cards/"+card.CardID, nil, &detail)
	var download publish.Download
	harness.requestJSON(
		t,
		ctx,
		http.MethodGet,
		"/v1/cards/"+card.CardID+"/versions/"+outcome.VersionID+"/artifact",
		nil,
		&download,
	)
	storedVersion, err := harness.versions.Find(ctx, verticalUserID, card.CardID, outcome.VersionID)
	if err != nil {
		t.Fatal("published version is unavailable")
	}
	archive, err := harness.objects.get(ctx, storedVersion.ArtifactKey)
	if err != nil {
		t.Fatal("published artifact object is unavailable")
	}
	return verticalCaseResult{
		evalCase:        evalCase,
		confirmed:       cloneVerticalSnapshot(*frozenBefore.ConfirmedRequirement),
		ready:           ready,
		outcome:         outcome,
		card:            card,
		detail:          detail,
		download:        download,
		storedVersion:   storedVersion,
		artifactArchive: archive,
	}
}

func verticalWorkerDiagnostic(
	ctx context.Context,
	harness *verticalHarness,
	sessionID string,
	cause error,
) verticalFailureDiagnostic {
	errorCode := verticalGenerationErrorCode(harness, sessionID)
	category := verticalWorkerErrorCategory(ctx, cause, errorCode)
	if errorCode == "" {
		if category == "timeout" {
			errorCode = "WORKER_TIMEOUT"
		} else {
			errorCode = "WORKER_FAILED"
		}
	}
	return verticalFailureDiagnostic{errorCode: errorCode, category: category}
}

func verticalGenerationErrorCode(harness *verticalHarness, sessionID string) string {
	if harness == nil || harness.generations == nil || sessionID == "" {
		return ""
	}
	session, err := harness.generations.Get(context.Background(), verticalUserID, sessionID)
	if err != nil {
		return ""
	}
	for index := len(session.Events) - 1; index >= 0; index-- {
		if code := safeVerticalErrorCode(session.Events[index].ErrorCode); code != "" {
			return code
		}
	}
	return ""
}

func safeVerticalErrorCode(code string) string {
	switch code {
	case "VALIDATION_FAILED",
		"ARTIFACT_PUBLISH_FAILED",
		"GENERATION_STATE_INVALID",
		"GENERATION_REQUIREMENTS_MISSING",
		"JOB_ENQUEUE_FAILED",
		"INTERNAL":
		return code
	default:
		return ""
	}
}

func verticalWorkerErrorCategory(ctx context.Context, cause error, errorCode string) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		errors.Is(ctx.Err(), context.Canceled) ||
		errors.Is(cause, context.DeadlineExceeded) ||
		errors.Is(cause, context.Canceled) {
		return "timeout"
	}
	if errors.Is(cause, agent.ErrValidationFailed) {
		return verticalValidationErrorCategory(cause)
	}
	if errors.Is(cause, agent.ErrForbiddenRequirement) ||
		errors.Is(cause, agent.ErrUnsupportedRequirement) {
		return "requirement_rejected"
	}
	if category := verticalProviderErrorCategory(cause); category != "" {
		return category
	}
	switch errorCode {
	case "VALIDATION_FAILED":
		return "provider_error"
	case "ARTIFACT_PUBLISH_FAILED":
		return "publish_error"
	case "JOB_ENQUEUE_FAILED":
		return "queue_error"
	case "GENERATION_STATE_INVALID", "GENERATION_REQUIREMENTS_MISSING":
		return "generation_error"
	default:
		return "internal_error"
	}
}

func verticalValidationErrorCategory(cause error) string {
	message := cause.Error()
	switch {
	case strings.Contains(message, " has unknown prop "):
		return "validation_unknown_prop"
	case strings.Contains(message, "unknown NativeCard component"):
		return "validation_unknown_component"
	case strings.Contains(message, "missing from initialState"),
		strings.Contains(message, "no writable parent in initialState"),
		strings.Contains(message, "incompatible initialState type"):
		return "validation_state_path"
	case strings.Contains(message, "unknown NativeCard action"):
		return "validation_unknown_action"
	case strings.Contains(message, "action ") && strings.Contains(message, " is missing fields:"):
		return "validation_action_missing_field"
	case strings.Contains(message, "action ") && strings.Contains(message, " has unknown fields:"):
		return "validation_action_unknown_field"
	case strings.Contains(message, "action ") && strings.Contains(message, " path is invalid"):
		return "validation_action_path"
	case strings.Contains(message, "action type must be"):
		return "validation_action_contract"
	case strings.Contains(message, "requires prop"),
		strings.Contains(message, "node is missing fields"),
		strings.Contains(message, "NativeCard is missing fields"):
		return "validation_missing_field"
	case strings.Contains(message, "expression"), strings.Contains(message, "bindings"):
		return "validation_expression"
	case strings.Contains(message, "decode NativeCard"),
		strings.Contains(message, "must contain one JSON value"),
		strings.Contains(message, "NativeCard must be"):
		return "validation_json_shape"
	case strings.Contains(message, "value must be"),
		strings.Contains(message, "outside the catalog enum"),
		strings.Contains(message, "value does not match required pattern"):
		return "validation_value_type"
	case strings.Contains(message, "does not support event"),
		strings.Contains(message, "events must be an object"),
		strings.Contains(message, "event ") && strings.Contains(message, " must be an array"),
		strings.Contains(message, "event ") && strings.Contains(message, " exceeds "):
		return "validation_event_contract"
	case strings.Contains(message, " has unknown fields:"):
		return "validation_unknown_field"
	default:
		return "validation_failed"
	}
}

func verticalProviderErrorCategory(cause error) string {
	if cause == nil {
		return ""
	}
	message := cause.Error()
	switch {
	case strings.Contains(message, "model response did not complete successfully"):
		return "provider_incomplete"
	case strings.Contains(message, "model response has no content"):
		return "provider_no_content"
	case strings.Contains(message, "decode model response failed"):
		return "provider_decode_error"
	case strings.Contains(message, "model response exceeds size limit"):
		return "provider_oversize"
	case strings.Contains(message, "read model response failed"):
		return "provider_read_error"
	case strings.Contains(message, "model request failed"):
		return "provider_transport_error"
	}
	const marker = "model provider returned status "
	index := strings.Index(message, marker)
	if index < 0 {
		return ""
	}
	digits := message[index+len(marker):]
	if len(digits) < 3 || digits[0] < '4' || digits[0] > '5' ||
		digits[1] < '0' || digits[1] > '9' || digits[2] < '0' || digits[2] > '9' {
		return ""
	}
	status := int(digits[0]-'0')*100 + int(digits[1]-'0')*10 + int(digits[2]-'0')
	return fmt.Sprintf("provider_status_%d", status)
}

func verticalWorkerFailureMessage(diagnostic verticalFailureDiagnostic) string {
	return fmt.Sprintf(
		"vertical worker stage failed: errorCode=%s category=%s",
		diagnostic.errorCode,
		diagnostic.category,
	)
}

func verifyVerticalResult(t *testing.T, harness *verticalHarness, result verticalCaseResult) verticalEvidence {
	t.Helper()

	if result.ready.Status != generation.StatusReady ||
		result.ready.ID != result.outcome.SessionID ||
		result.ready.VersionID != result.outcome.VersionID {
		t.Fatal("ready generation response is inconsistent")
	}
	if !verticalSnapshotMatches(&result.confirmed, result.evalCase) {
		t.Fatal("frozen snapshot does not contain both additions")
	}
	if result.card.CardID != result.storedVersion.CardID ||
		result.card.LatestVersion.VersionID != result.outcome.VersionID ||
		result.detail.CardID != result.card.CardID ||
		len(result.detail.Versions) != 1 ||
		result.detail.Versions[0].VersionID != result.outcome.VersionID {
		t.Fatal("card API responses are inconsistent")
	}
	if result.download.SHA256 != result.storedVersion.ArtifactSHA256 ||
		result.download.KeyID != harness.keyID ||
		result.storedVersion.KeyID != harness.keyID {
		t.Fatal("artifact download metadata is inconsistent")
	}
	downloadURL, err := url.Parse(result.download.URL)
	if err != nil || downloadURL.Scheme != "memory" {
		t.Fatal("artifact download URL is invalid")
	}
	if err := verifyVerticalArchive(
		result.artifactArchive,
		result.storedVersion,
		harness.keyID,
		harness.publicKey,
	); err != nil {
		t.Fatal("signed NativeCard artifact verification failed")
	}
	attempts, err := verticalValidationAttempts(result.artifactArchive)
	if err != nil {
		t.Fatal("validated artifact evidence is unavailable")
	}
	return verticalEvidence{
		attempts:       attempts,
		artifactSHA256: result.storedVersion.ArtifactSHA256,
	}
}

func verticalValidationAttempts(archive []byte) (int, error) {
	entries, err := readVerticalZIP(archive)
	if err != nil {
		return 0, err
	}
	var report struct {
		Status   string        `json:"status"`
		Runtime  agent.Runtime `json:"runtime"`
		Attempts int           `json:"attempts"`
	}
	if err := decodeVerticalJSON(entries["reports/validation.json"], &report, true); err != nil ||
		report.Status != "passed" || report.Runtime != agent.RuntimeNative ||
		report.Attempts < 1 || report.Attempts > 3 {
		return 0, fmt.Errorf("validation report is invalid")
	}
	return report.Attempts, nil
}

func (harness *verticalHarness) requestJSON(
	t *testing.T,
	ctx context.Context,
	method string,
	requestPath string,
	body any,
	destination any,
) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal("encode vertical API request failed")
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, harness.server.URL+requestPath, reader)
	if err != nil {
		t.Fatal("create vertical API request failed")
	}
	request.Header.Set("Authorization", "Bearer "+harness.bearer)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := harness.client.Do(request)
	if err != nil {
		t.Fatal("vertical API request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("vertical API request failed: method=%s path=%s status=%d", method, requestPath, response.StatusCode)
	}
	if destination == nil {
		return
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1024*1024+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		t.Fatal("decode vertical API response failed")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatal("vertical API response contains trailing data")
	}
}

func verifyVerticalArchive(
	archive []byte,
	version publish.CardVersion,
	keyID string,
	publicKey ed25519.PublicKey,
) error {
	if len(archive) == 0 || len(archive) > 8*1024*1024 {
		return fmt.Errorf("artifact archive size is invalid")
	}
	digest := sha256.Sum256(archive)
	if hex.EncodeToString(digest[:]) != version.ArtifactSHA256 {
		return fmt.Errorf("artifact archive hash mismatch")
	}
	entries, err := readVerticalZIP(archive)
	if err != nil {
		return err
	}
	manifestBytes, hasManifest := entries["manifest.json"]
	nativePayload, hasPayload := entries["payload/native.json"]
	reportBytes, hasReport := entries["reports/validation.json"]
	if !hasManifest || !hasPayload || !hasReport || len(entries) != 3 {
		return fmt.Errorf("artifact file set is invalid")
	}

	var manifest map[string]any
	if err := decodeVerticalJSON(manifestBytes, &manifest, false); err != nil {
		return fmt.Errorf("manifest JSON is invalid")
	}
	signatureText, signatureOK := manifest["signature"].(string)
	manifestKeyID, keyOK := manifest["keyId"].(string)
	if !signatureOK || !keyOK || manifestKeyID != keyID {
		return fmt.Errorf("manifest signing metadata is invalid")
	}
	delete(manifest, "signature")
	signature, err := base64.RawURLEncoding.DecodeString(signatureText)
	if err != nil {
		return fmt.Errorf("manifest signature encoding is invalid")
	}
	canonical, err := artifact.CanonicalJSON(manifest)
	if err != nil || !ed25519.Verify(publicKey, canonical, signature) {
		return fmt.Errorf("manifest signature is invalid")
	}
	delete(manifest, "keyId")
	definitionBytes, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("manifest definition is invalid")
	}
	definition, err := contracts.DecodeCardDefinition(bytes.NewReader(definitionBytes))
	if err != nil {
		return fmt.Errorf("manifest contract is invalid")
	}
	if definition.CardID != version.CardID ||
		definition.VersionID != version.VersionID ||
		definition.Runtime != contracts.CardRuntimeNative ||
		definition.Entrypoint != "payload/native.json" ||
		definition.CatalogVersion != "1" ||
		!slices.Equal(definition.Capabilities, []string{"storage", "window.manageSelf"}) ||
		definition.NetworkPolicy.Mode != "none" ||
		len(definition.NetworkPolicy.Domains) != 0 {
		return fmt.Errorf("manifest definition metadata is inconsistent")
	}
	if len(definition.Files) != 2 {
		return fmt.Errorf("manifest file catalog is incomplete")
	}
	manifestPaths := make(map[string]bool, len(definition.Files))
	for _, manifestFile := range definition.Files {
		if manifestPaths[manifestFile.Path] {
			return fmt.Errorf("manifest file catalog contains duplicate paths")
		}
		manifestPaths[manifestFile.Path] = true
		content, exists := entries[manifestFile.Path]
		if !exists || manifestFile.Size != int64(len(content)) {
			return fmt.Errorf("manifest file size mismatch")
		}
		fileDigest := sha256.Sum256(content)
		if manifestFile.SHA256 != hex.EncodeToString(fileDigest[:]) {
			return fmt.Errorf("manifest file hash mismatch")
		}
	}
	if !manifestPaths["payload/native.json"] || !manifestPaths["reports/validation.json"] {
		return fmt.Errorf("manifest file catalog does not match the NativeCard payload")
	}
	if err := agent.NewNativeValidator().Validate(string(nativePayload), definition.Capabilities...); err != nil {
		return fmt.Errorf("NativeCard payload validation failed")
	}
	var report struct {
		Status   string        `json:"status"`
		Runtime  agent.Runtime `json:"runtime"`
		Attempts int           `json:"attempts"`
	}
	if err := decodeVerticalJSON(reportBytes, &report, true); err != nil ||
		report.Status != "passed" ||
		report.Runtime != agent.RuntimeNative ||
		report.Attempts < 1 || report.Attempts > 3 {
		return fmt.Errorf("validation report is invalid")
	}
	return nil
}

func readVerticalZIP(archive []byte) (map[string][]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("artifact ZIP is invalid")
	}
	entries := make(map[string][]byte, len(reader.File))
	expanded := uint64(0)
	for _, file := range reader.File {
		parts := strings.Split(file.Name, "/")
		if file.Name == "" ||
			strings.HasPrefix(file.Name, "/") ||
			strings.Contains(file.Name, "\\") ||
			path.Clean(file.Name) != file.Name ||
			len(parts) > 8 ||
			file.FileInfo().IsDir() {
			return nil, fmt.Errorf("artifact ZIP path is unsafe")
		}
		for _, part := range parts {
			if part == "" || part == "." || part == ".." {
				return nil, fmt.Errorf("artifact ZIP path is unsafe")
			}
		}
		if _, exists := entries[file.Name]; exists {
			return nil, fmt.Errorf("artifact ZIP contains duplicate paths")
		}
		if file.UncompressedSize64 > 8*1024*1024 {
			return nil, fmt.Errorf("artifact ZIP entry is too large")
		}
		expanded += file.UncompressedSize64
		if expanded > 32*1024*1024 {
			return nil, fmt.Errorf("artifact ZIP expanded size is too large")
		}
		stream, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("artifact ZIP entry cannot be opened")
		}
		content, readErr := io.ReadAll(io.LimitReader(stream, 8*1024*1024+1))
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil || len(content) > 8*1024*1024 || uint64(len(content)) != file.UncompressedSize64 {
			return nil, fmt.Errorf("artifact ZIP entry cannot be read safely")
		}
		entries[file.Name] = content
	}
	return entries, nil
}

func decodeVerticalJSON(content []byte, destination any, strict bool) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON contains trailing data")
	}
	return nil
}

func verticalSnapshotMatches(snapshot *generation.RequirementSnapshot, evalCase verticalCase) bool {
	return snapshot != nil &&
		snapshot.InitialPrompt == evalCase.Prompt &&
		snapshot.Target == generation.TargetNative &&
		snapshot.Locale == "zh-CN" &&
		len(snapshot.AdditionalMessages) == len(evalCase.Messages) &&
		snapshot.AdditionalMessages[0].Content == evalCase.Messages[0] &&
		snapshot.AdditionalMessages[1].Content == evalCase.Messages[1] &&
		slices.Equal(snapshot.AllowedCapabilities, []string{"storage", "window.manageSelf"})
}

func verticalSnapshotsEqual(left, right generation.RequirementSnapshot) bool {
	return left.InitialPrompt == right.InitialPrompt &&
		left.Target == right.Target &&
		left.Locale == right.Locale &&
		left.ConfirmedAt.Equal(right.ConfirmedAt) &&
		slices.Equal(left.AllowedCapabilities, right.AllowedCapabilities) &&
		slices.EqualFunc(left.AdditionalMessages, right.AdditionalMessages, func(a, b generation.Message) bool {
			return a.Role == b.Role && a.Content == b.Content && a.CreatedAt.Equal(b.CreatedAt)
		})
}

func cloneVerticalSnapshot(snapshot generation.RequirementSnapshot) generation.RequirementSnapshot {
	snapshot.AllowedCapabilities = append([]string(nil), snapshot.AllowedCapabilities...)
	snapshot.AdditionalMessages = append([]generation.Message(nil), snapshot.AdditionalMessages...)
	return snapshot
}

func sequentialID(prefix string) func() string {
	var next atomic.Uint64
	return func() string {
		return fmt.Sprintf("%s%02d", prefix, next.Add(1))
	}
}

func verticalTestBearer() string {
	digest := sha256.Sum256([]byte("p0a-vertical-bearer-seed-v1"))
	return "vertical-" + hex.EncodeToString(digest[:16])
}

func (store *verticalObjectStore) PutIfAbsent(ctx context.Context, key string, content []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, exists := store.objects[key]; exists {
		if !bytes.Equal(existing, content) {
			return publish.ErrObjectConflict
		}
		return nil
	}
	store.objects[key] = append([]byte(nil), content...)
	return nil
}

func (store *verticalObjectStore) SignedURL(
	ctx context.Context,
	key string,
	ttl time.Duration,
) (string, time.Time, error) {
	if err := ctx.Err(); err != nil {
		return "", time.Time{}, err
	}
	store.mu.RLock()
	_, exists := store.objects[key]
	store.mu.RUnlock()
	if !exists {
		return "", time.Time{}, publish.ErrNotFound
	}
	return (&url.URL{Scheme: "memory", Host: "artifact", Path: "/" + key}).String(), verticalNow.Add(ttl), nil
}

func (store *verticalObjectStore) get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	content, exists := store.objects[key]
	if !exists {
		return nil, publish.ErrNotFound
	}
	return append([]byte(nil), content...), nil
}
