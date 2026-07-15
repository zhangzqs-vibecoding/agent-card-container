package bootstrap_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

func TestHeadlessVerticalFakeModelProducesVerifiedSignedNativeCard(t *testing.T) {
	t.Parallel()

	harness := newVerticalHarness(t, verticalStaticProvider{})
	result := runVerticalCase(t, context.Background(), harness, verticalCases()[0])
	_ = verifyVerticalResult(t, harness, result)
}

func TestVerticalArtifactVerifierRejectsArchiveAndSignatureTampering(t *testing.T) {
	t.Parallel()

	harness := newVerticalHarness(t, verticalStaticProvider{})
	result := runVerticalCase(t, context.Background(), harness, verticalCases()[0])

	tamperedArchive := append([]byte(nil), result.artifactArchive...)
	tamperedArchive[len(tamperedArchive)-1] ^= 0xff
	if err := verifyVerticalArchive(tamperedArchive, result.storedVersion, harness.keyID, harness.publicKey); err == nil {
		t.Fatal("artifact verifier accepted archive hash tampering")
	}

	entries, err := readVerticalZIP(result.artifactArchive)
	if err != nil {
		t.Fatal("read valid artifact ZIP failed")
	}
	var manifest map[string]any
	if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
		t.Fatal("decode valid manifest failed")
	}
	manifest["signature"] = "invalid-signature"
	entries["manifest.json"], err = json.Marshal(manifest)
	if err != nil {
		t.Fatal("encode tampered manifest failed")
	}
	signatureTampered := verticalTestZIP(t, entries)
	signatureDigest := sha256.Sum256(signatureTampered)
	tamperedVersion := result.storedVersion
	tamperedVersion.ArtifactSHA256 = hex.EncodeToString(signatureDigest[:])
	if err := verifyVerticalArchive(signatureTampered, tamperedVersion, harness.keyID, harness.publicKey); err == nil {
		t.Fatal("artifact verifier accepted manifest signature tampering")
	}
}

func TestVerticalZIPReaderRejectsUnsafeAndDuplicatePaths(t *testing.T) {
	t.Parallel()

	for _, names := range [][]string{{"../escape"}, {"same", "same"}} {
		var archive bytes.Buffer
		writer := zip.NewWriter(&archive)
		for _, name := range names {
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal("create test ZIP entry failed")
			}
			_, _ = entry.Write([]byte("content"))
		}
		if err := writer.Close(); err != nil {
			t.Fatal("close test ZIP failed")
		}
		if _, err := readVerticalZIP(archive.Bytes()); err == nil {
			t.Fatal("ZIP reader accepted unsafe archive")
		}
	}
}

func TestVerticalWorkerDiagnosticUsesOnlyStableCodeAndCategory(t *testing.T) {
	t.Parallel()

	harness := newVerticalHarness(t, verticalStaticProvider{})
	tests := []struct {
		name         string
		errorCode    string
		cause        error
		cancel       bool
		wantCode     string
		wantCategory string
	}{
		{
			name:         "provider incomplete",
			errorCode:    "VALIDATION_FAILED",
			cause:        errors.New("provider-sensitive-response-marker: model response did not complete successfully"),
			wantCode:     "VALIDATION_FAILED",
			wantCategory: "provider_incomplete",
		},
		{
			name:         "provider status",
			errorCode:    "VALIDATION_FAILED",
			cause:        errors.New("provider-sensitive-response-marker: model provider returned status 429"),
			wantCode:     "VALIDATION_FAILED",
			wantCategory: "provider_status_429",
		},
		{
			name:         "validator",
			errorCode:    "VALIDATION_FAILED",
			cause:        fmt.Errorf("%w: component TextInput has unknown prop model-sensitive-content", agent.ErrValidationFailed),
			wantCode:     "VALIDATION_FAILED",
			wantCategory: "validation_unknown_prop",
		},
		{
			name:      "validator wrapped state action",
			errorCode: "VALIDATION_FAILED",
			cause: fmt.Errorf(
				"%w: event onPressed action 0: action append path model-sensitive-content is missing from initialState",
				agent.ErrValidationFailed,
			),
			wantCode:     "VALIDATION_FAILED",
			wantCategory: "validation_state_path",
		},
		{
			name:         "validator unknown action",
			errorCode:    "VALIDATION_FAILED",
			cause:        fmt.Errorf("%w: unknown NativeCard action model-sensitive-content", agent.ErrValidationFailed),
			wantCode:     "VALIDATION_FAILED",
			wantCategory: "validation_unknown_action",
		},
		{
			name:         "validator action missing field",
			errorCode:    "VALIDATION_FAILED",
			cause:        fmt.Errorf("%w: action set is missing fields: model-sensitive-content", agent.ErrValidationFailed),
			wantCode:     "VALIDATION_FAILED",
			wantCategory: "validation_action_missing_field",
		},
		{
			name:         "validator action unknown field",
			errorCode:    "VALIDATION_FAILED",
			cause:        fmt.Errorf("%w: action set has unknown fields: model-sensitive-content", agent.ErrValidationFailed),
			wantCode:     "VALIDATION_FAILED",
			wantCategory: "validation_action_unknown_field",
		},
		{
			name:         "validator action path",
			errorCode:    "VALIDATION_FAILED",
			cause:        fmt.Errorf("%w: action set path is invalid model-sensitive-content", agent.ErrValidationFailed),
			wantCode:     "VALIDATION_FAILED",
			wantCategory: "validation_action_path",
		},
		{
			name:         "timeout",
			cause:        fmt.Errorf("request-sensitive-content: %w", context.DeadlineExceeded),
			cancel:       true,
			wantCode:     "WORKER_TIMEOUT",
			wantCategory: "timeout",
		},
		{
			name:         "publish",
			errorCode:    "ARTIFACT_PUBLISH_FAILED",
			cause:        errors.New("artifact-sensitive-content"),
			wantCode:     "ARTIFACT_PUBLISH_FAILED",
			wantCategory: "publish_error",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session, err := harness.generations.Create(context.Background(), verticalUserID, generation.CreateRequest{
				Prompt: "diagnostic-sensitive-prompt",
				Target: generation.TargetNative,
				Locale: "zh-CN",
			})
			if err != nil {
				t.Fatal("create diagnostic session failed")
			}
			if _, err := harness.generations.Confirm(context.Background(), verticalUserID, session.ID); err != nil {
				t.Fatal("confirm diagnostic session failed")
			}
			if test.errorCode != "" {
				if _, err := harness.generations.MarkFailed(context.Background(), session.ID, test.errorCode); err != nil {
					t.Fatal("mark diagnostic session failed")
				}
			}
			diagnosticContext := context.Background()
			if test.cancel {
				cancelled, cancel := context.WithCancel(context.Background())
				cancel()
				diagnosticContext = cancelled
			}
			diagnostic := verticalWorkerDiagnostic(diagnosticContext, harness, session.ID, test.cause)
			message := verticalWorkerFailureMessage(diagnostic)
			if diagnostic.errorCode != test.wantCode || diagnostic.category != test.wantCategory {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if !strings.Contains(message, "errorCode="+test.wantCode) ||
				!strings.Contains(message, "category="+test.wantCategory) {
				t.Fatalf("safe diagnostic message is incomplete: %s", message)
			}
			for _, sensitive := range []string{
				"provider-sensitive-response-marker",
				"model-sensitive-content",
				"request-sensitive-content",
				"artifact-sensitive-content",
				"diagnostic-sensitive-prompt",
			} {
				if strings.Contains(message, sensitive) {
					t.Fatalf("safe diagnostic leaked sensitive marker")
				}
			}
		})
	}
}

func TestDeepSeekVerticalLiveProducesRepresentativeSignedNativeCards(t *testing.T) {
	if os.Getenv("AGENTCARD_DEEPSEEK_LIVE") != "1" {
		t.Skip("paid DeepSeek vertical gate disabled; enable explicitly")
	}
	provider, err := modelprovider.NewHTTPProviderFromEnvironment(map[string]string{
		"AGENTCARD_MODEL_API_KEY":  os.Getenv("AGENTCARD_MODEL_API_KEY"),
		"AGENTCARD_MODEL_BASE_URL": os.Getenv("AGENTCARD_MODEL_BASE_URL"),
		"AGENTCARD_MODEL":          os.Getenv("AGENTCARD_MODEL"),
	})
	if err != nil {
		t.Fatal("model environment configuration is invalid")
	}

	harness := newVerticalHarness(t, provider)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for _, evalCase := range verticalCases() {
		evalCase := evalCase
		if !t.Run(evalCase.ID, func(t *testing.T) {
			startedAt := time.Now()
			result := runVerticalCase(t, ctx, harness, evalCase)
			evidence := verifyVerticalResult(t, harness, result)
			t.Logf(
				"case id=%s status=success durationMs=%d attempts=%d artifactSha256=%s",
				evalCase.ID,
				time.Since(startedAt).Milliseconds(),
				evidence.attempts,
				evidence.artifactSHA256,
			)
		}) {
			break
		}
	}
}

func verticalTestZIP(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal("create test ZIP entry failed")
		}
		if _, err := entry.Write(entries[name]); err != nil {
			t.Fatal("write test ZIP entry failed")
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal("close test ZIP failed")
	}
	return archive.Bytes()
}
