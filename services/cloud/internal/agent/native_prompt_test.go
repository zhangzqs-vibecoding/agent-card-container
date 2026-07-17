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

	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
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

func TestNativePromptExplainsRelativeStatePathsAndExactActionShapes(t *testing.T) {
	t.Parallel()

	prompt := nativeModelRequest(promptRequest(), 1, "").SystemPrompt
	for _, required := range []string{
		"valuePath and action.path are relative paths",
		"must exactly resolve in initialState",
		`{"path":"state.form.title"}`,
		`{"type":"set","path":"saved","value":true}`,
		"Do not add target, payload, args, or state fields to actions",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("system prompt is missing state/action guidance %q", required)
		}
	}
}

func TestNativePromptIncludesMinimalRecipesForSupportedInteractiveCards(t *testing.T) {
	t.Parallel()

	prompt := nativeModelRequest(promptRequest(), 1, "").SystemPrompt
	for _, required := range []string{
		"Keep component types minimal",
		"Countdown recipe",
		"wrap the timer in a Container with a Column child",
		`{"type":"startTimer","path":"seconds","value":{"intervalMs":1000,"delta":-1,"stopAt":0}}`,
		"Static list recipe",
		"List.children",
		"Dashboard recipe",
		"put every requested fixed milestone inside List.children",
		"Form recipe",
		"Slider allows only valuePath, min, and max",
		"Chart recipe",
		`{"values":[1,2,3]}`,
		"For fixed example data, do not use a state path binding",
		"Counter recipe",
		`{"op":"concat","args":[{"path":"state.count"}]}`,
		"represent a requested status with a Badge",
		"Badge.label should usually be a literal status string",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("system prompt is missing supported-card recipe %q", required)
		}
	}
}

func TestNativePromptGivesFormListOneConsistentStateExample(t *testing.T) {
	t.Parallel()

	prompt := nativeModelRequest(promptRequest(), 1, "").SystemPrompt
	for _, required := range []string{
		`"initialState":{"title":"","priority":"中","done":false,"saved":false}`,
		`"valuePath":"title"`,
		`"valuePath":"priority"`,
		`"valuePath":"done"`,
		`{"type":"set","path":"saved","value":true}`,
		"every valuePath and every action.path must use one of those existing paths",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("system prompt is missing consistent form/list state example %q", required)
		}
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
		"Only these generated source paths are allowed: src/card.tsx and src/card.css.",
	) {
		t.Fatalf("web system prompt has no exact source whitelist: %q", request.SystemPrompt)
	}
	if !strings.Contains(request.SystemPrompt, "Do not generate src/card.test.tsx") {
		t.Fatalf("web system prompt does not reserve tests for the platform: %q", request.SystemPrompt)
	}
}

func TestWebPromptDeclaresTemplateAPIAndOfflineSecurityBoundary(t *testing.T) {
	t.Parallel()

	request := webModelRequest(promptRequest(), 1, "")
	for _, required := range []string{
		"Preact",
		`import { agentCard } from "./agentcard"`,
		"self-contained and offline",
		"Do not call fetch",
		"eval",
		"new Function",
		"dynamic import",
		"Do not add dependencies",
	} {
		if !strings.Contains(request.SystemPrompt, required) {
			t.Fatalf("web system prompt is missing %q: %q", required, request.SystemPrompt)
		}
	}
}

func TestIterationPromptIncludesStableReadOnlyNativeBaseline(t *testing.T) {
	t.Parallel()

	request := promptRequest()
	request.Requirement.BaseCardID = "card_base"
	request.Requirement.BaseVersionID = "ver_base"
	request.BaseArtifact = &BaseArtifact{
		Definition: promptBaseDefinition(contracts.CardRuntimeNative),
		Sources: map[string]string{
			"payload/native.json": `{"schemaVersion":1,"initialState":{"count":1},"root":{"id":"root","type":"Text"}}`,
		},
	}
	prompt := nativeModelRequest(request, 1, "").UserPrompt
	for _, required := range []string{
		"Existing signed base version (read-only):",
		`"cardId":"card_base"`,
		`"versionId":"ver_base"`,
		`"runtime":"native"`,
		`"payload/native.json"`,
		`\"count\":1`,
		"Modify the existing card instead of creating an unrelated card.",
		"Do not change runtime, state schema, dependencies, or capability boundaries.",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("iteration prompt is missing %q: %q", required, prompt)
		}
	}
}

func TestIterationPromptIncludesControlledCodeCardSources(t *testing.T) {
	t.Parallel()

	request := promptRequest()
	request.Requirement.Target = generation.TargetWeb
	request.Requirement.BaseCardID = "card_base"
	request.Requirement.BaseVersionID = "ver_base"
	request.BaseArtifact = &BaseArtifact{
		Definition: promptBaseDefinition(contracts.CardRuntimeWeb),
		Sources: map[string]string{
			"src/card.tsx": "export function Card(){return <main/>}",
			"src/card.css": "main{display:block}",
		},
	}
	prompt := webModelRequest(request, 1, "").UserPrompt
	if !strings.Contains(prompt, `"src/card.tsx"`) ||
		!strings.Contains(prompt, "export function Card") ||
		strings.Contains(prompt, "reports/validation.json") {
		t.Fatalf("CodeCard iteration prompt = %q", prompt)
	}
}

func TestCodingAgentRejectsMissingOrInvalidBaselineBeforeProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base *BaseArtifact
	}{
		{name: "missing"},
		{name: "oversized", base: &BaseArtifact{
			Definition: promptBaseDefinition(contracts.CardRuntimeNative),
			Sources:    map[string]string{"payload/native.json": strings.Repeat("a", MaxBaseSourceBytes+1)},
		}},
		{name: "unknown web source", base: &BaseArtifact{
			Definition: promptBaseDefinition(contracts.CardRuntimeWeb),
			Sources:    map[string]string{"src/card.tsx": "export function Card(){return null}", "src/unknown.ts": "x"},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &promptCaptureProvider{outputs: []string{`{"schemaVersion":1}`}}
			request := promptRequest()
			request.Requirement.BaseCardID = "card_base"
			request.Requirement.BaseVersionID = "ver_base"
			request.BaseArtifact = test.base
			if _, err := NewCodingAgent(provider, NewNativeValidator()).Generate(context.Background(), request); err == nil {
				t.Fatal("Generate() error = nil")
			}
			if len(provider.requests) != 0 {
				t.Fatalf("provider calls = %d", len(provider.requests))
			}
		})
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

func promptBaseDefinition(runtime contracts.CardRuntime) contracts.CardDefinition {
	definition := contracts.CardDefinition{
		FormatVersion: 1, MinHostVersion: "1.0.0", CardID: "card_base", VersionID: "ver_base",
		DisplayVersion: "1.0.0", Runtime: runtime, StateSchemaVersion: 1, Title: "基线",
		Entrypoint: "payload/native.json", CatalogVersion: "1",
		MinSize: contracts.Size{Width: 240, Height: 160}, PreferredSize: contracts.Size{Width: 360, Height: 240},
		MaxSize: contracts.Size{Width: 720, Height: 480}, Capabilities: []string{"storage"},
		NetworkPolicy: contracts.NetworkPolicy{Mode: "none", Domains: []string{}},
		CreatedAt:     time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
	}
	if runtime == contracts.CardRuntimeWeb {
		definition.Entrypoint = "payload/web/index.html"
		definition.CatalogVersion = ""
	}
	return definition
}

type promptCaptureProvider struct {
	outputs  []string
	requests []modelprovider.Request
}

func (provider *promptCaptureProvider) Generate(_ context.Context, request modelprovider.Request) (modelprovider.Response, error) {
	provider.requests = append(provider.requests, request)
	return modelprovider.Response{Content: provider.outputs[len(provider.requests)-1]}, nil
}
