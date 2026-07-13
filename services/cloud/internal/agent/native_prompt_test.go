package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

func TestNativePromptIncludesGeneratedContractContextAndJSONRules(t *testing.T) {
	t.Parallel()

	request := nativeModelRequest(promptRequest(), 1, "")
	for _, required := range []string{
		nativeContractContextJSON,
		"exactly one JSON object",
		"Markdown fences",
		"nativeSupported",
		"Allowed capabilities",
		`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`,
		"latest validation feedback supersedes",
	} {
		if !strings.Contains(request.SystemPrompt, required) {
			t.Fatalf("system prompt is missing %q", required)
		}
	}
	if !request.JSONOutput || request.MaxTokens != 8192 {
		t.Fatalf("transport options = JSONOutput:%v MaxTokens:%d", request.JSONOutput, request.MaxTokens)
	}
}

func TestNativePromptRendersFrozenRequirementInStableOrder(t *testing.T) {
	t.Parallel()

	request := nativeModelRequest(promptRequest(), 2, "")
	want := strings.Join([]string{
		"Session ID: gen_prompt",
		"Runtime: native",
		"Attempt: 2",
		"Initial requirement:",
		"初始需求",
		"Additional requirements:",
		"1. 第一条补充",
		"2. 第二条补充",
		"Target: native",
		"Locale: zh-TW",
		"Allowed capabilities:",
		"- notification.show",
		"- window.manageSelf",
	}, "\n")
	if request.UserPrompt != want {
		t.Fatalf("user prompt = %q, want %q", request.UserPrompt, want)
	}
	if strings.Contains(request.UserPrompt, "2026-07-13") || strings.Contains(request.UserPrompt, "Validation feedback:") {
		t.Fatalf("user prompt contains unstable metadata: %q", request.UserPrompt)
	}
}

func TestNativePromptRepairUsesOnlyLatestFeedbackAndNeverInvalidBody(t *testing.T) {
	t.Parallel()

	provider := &promptCaptureProvider{outputs: []string{
		`{"schemaVersion":1,"initialState":{"invalid-body-one":"secret"},"root":{"id":"root","type":"FirstBroken"}}`,
		`{"schemaVersion":1,"initialState":{"invalid-body-two":"secret"},"root":{"id":"root","type":"Text","props":{"secondBroken":"secret"}}}`,
		`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`,
	}}
	result, err := NewCodingAgent(provider, NewNativeValidator()).Generate(context.Background(), promptRequest())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Attempts != 3 || len(provider.requests) != 3 {
		t.Fatalf("attempts=%d requests=%d", result.Attempts, len(provider.requests))
	}
	first, second, third := provider.requests[0], provider.requests[1], provider.requests[2]
	if strings.Contains(first.UserPrompt, "Validation feedback:") {
		t.Fatalf("first request contains repair feedback: %q", first.UserPrompt)
	}
	if !strings.Contains(second.UserPrompt, "Validation feedback:") || !strings.Contains(second.UserPrompt, "FirstBroken") {
		t.Fatalf("second request is missing first feedback: %q", second.UserPrompt)
	}
	if strings.Contains(third.UserPrompt, "FirstBroken") || !strings.Contains(third.UserPrompt, "secondBroken") {
		t.Fatalf("third request did not replace old feedback: %q", third.UserPrompt)
	}
	for _, request := range provider.requests {
		for _, leaked := range []string{"invalid-body-one", "invalid-body-two"} {
			if strings.Contains(request.UserPrompt, leaked) {
				t.Fatalf("invalid model body leaked into repair prompt: %q", request.UserPrompt)
			}
		}
		if request.SystemPrompt != first.SystemPrompt || !request.JSONOutput || request.MaxTokens != 8192 {
			t.Fatalf("repair transport drifted: %#v", request)
		}
	}
}

func TestNativePromptBoundsAndNormalizesValidationFeedback(t *testing.T) {
	t.Parallel()

	raw := string([]byte{0xff}) + strings.Repeat("反馈\n\t\x00", 800)
	feedback := stableValidationFeedback(raw)
	if len(feedback) > maxValidationFeedbackBytes {
		t.Fatalf("feedback bytes = %d, want <= %d", len(feedback), maxValidationFeedbackBytes)
	}
	if !utf8.ValidString(feedback) {
		t.Fatalf("feedback is not valid UTF-8: %q", feedback)
	}
	for _, value := range feedback {
		if unicode.IsControl(value) {
			t.Fatalf("feedback contains control character %U", value)
		}
	}
	request := nativeModelRequest(promptRequest(), 2, raw)
	base := nativeModelRequest(promptRequest(), 2, "")
	if len(request.UserPrompt)-len(base.UserPrompt) > maxValidationFeedbackBytes+128 {
		t.Fatalf("repair prompt grew without bound: base=%d repair=%d", len(base.UserPrompt), len(request.UserPrompt))
	}
}

func TestWebPromptDeclaresExactMVPSourceWhitelist(t *testing.T) {
	t.Parallel()

	request := webModelRequest(promptRequest(), 1, "")
	if !strings.Contains(
		request.SystemPrompt,
		"Only these source paths are allowed: src/card.tsx, src/card.css, src/card.test.tsx.",
	) {
		t.Fatalf("web system prompt has no exact source whitelist: %q", request.SystemPrompt)
	}
}

func TestModelProviderSourceContainsNoAgentCardPrompt(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("..", "modelprovider", "http_provider.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"AgentCard", "NativeCard", "CodeCard"} {
		if strings.Contains(string(content), term) {
			t.Fatalf("model provider owns business prompt term %q", term)
		}
	}
}

func promptRequest() Request {
	return Request{
		SessionID: "gen_prompt",
		Requirement: generation.RequirementSnapshot{
			InitialPrompt: "初始需求",
			AdditionalMessages: []generation.Message{
				{Role: "user", Content: "第一条补充"},
				{Role: "user", Content: "第二条补充"},
			},
			Target:              generation.TargetNative,
			Locale:              "zh-TW",
			AllowedCapabilities: []string{"notification.show", "window.manageSelf"},
			ConfirmedAt:         time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		},
	}
}

type promptCaptureProvider struct {
	outputs  []string
	requests []modelprovider.Request
}

func (provider *promptCaptureProvider) Generate(_ context.Context, request modelprovider.Request) (modelprovider.Response, error) {
	provider.requests = append(provider.requests, request)
	return modelprovider.Response{Content: provider.outputs[len(provider.requests)-1]}, nil
}
