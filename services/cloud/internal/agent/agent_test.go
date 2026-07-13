package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

func TestSelectorPrefersNativeAndExplainsWebFallback(t *testing.T) {
	t.Parallel()

	selector := agent.Selector{}
	native, err := selector.Select("做一个离线番茄钟", generation.TargetAuto)
	if err != nil {
		t.Fatal(err)
	}
	if native.Runtime != agent.RuntimeNative || native.Reason == "" {
		t.Fatalf("native decision = %#v", native)
	}

	web, err := selector.Select("做一个可以自由绘制的本地画板", generation.TargetAuto)
	if err != nil {
		t.Fatal(err)
	}
	if web.Runtime != agent.RuntimeWeb || web.Reason == "" {
		t.Fatalf("web decision = %#v", web)
	}
}

func TestSelectorRejectsForbiddenAndImpossibleForcedNativeRequirements(t *testing.T) {
	t.Parallel()

	selector := agent.Selector{}
	for _, prompt := range []string{
		"运行 Shell 命令并控制系统进程",
		"读取任意文件系统路径",
	} {
		if _, err := selector.Select(prompt, generation.TargetAuto); !errors.Is(err, agent.ErrForbiddenRequirement) {
			t.Fatalf("Select(%q) error = %v", prompt, err)
		}
	}
	if _, err := selector.Select("做一个自由绘制画板", generation.TargetNative); !errors.Is(err, agent.ErrUnsupportedRequirement) {
		t.Fatalf("forced native error = %v", err)
	}
}

func TestCodingAgentRejectsEverySelectorAttackBeforeProviderCall(t *testing.T) {
	t.Parallel()

	attacks := []struct {
		name   string
		prompt string
	}{
		{name: "shell", prompt: "运行 Shell 脚本"},
		{name: "command line", prompt: "调用命令行工具"},
		{name: "process control", prompt: "控制系统进程"},
		{name: "arbitrary filesystem", prompt: "访问任意文件系统"},
		{name: "arbitrary file", prompt: "读取任意文件"},
		{name: "global keyboard", prompt: "监听全局键盘监听事件"},
		{name: "browser extension", prompt: "安装浏览器扩展"},
	}
	for _, attack := range attacks {
		attack := attack
		t.Run(attack.name, func(t *testing.T) {
			t.Parallel()
			provider := &fakeProvider{outputs: []string{`{"schemaVersion":1}`}}
			codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())

			result, err := codingAgent.Generate(context.Background(), agent.Request{
				SessionID:   "gen_selector_attack",
				Requirement: confirmedRequirement(attack.prompt, generation.TargetAuto, "zh-CN"),
			})
			if !errors.Is(err, agent.ErrForbiddenRequirement) {
				t.Fatalf("Generate() error = %v, want ErrForbiddenRequirement", err)
			}
			if len(provider.requests) != 0 || result.Attempts != 0 ||
				result.InputTokens != 0 || result.OutputTokens != 0 {
				t.Fatalf("provider calls=%d result=%#v", len(provider.requests), result)
			}
		})
	}
}

func TestCodingAgentUsesConfirmedRequirementSnapshot(t *testing.T) {
	t.Parallel()

	confirmedAt := time.Date(2026, 7, 13, 12, 34, 56, 0, time.UTC)
	provider := &fakeProvider{outputs: []string{
		`{"files":{"src/card.tsx":"export function Card(){return <canvas/>}"}}`,
	}}
	codingAgent := agent.NewCodingAgent(
		provider,
		agent.NewNativeValidator(),
		agent.WithWebBuilder(&fakeWebBuilder{output: map[string][]byte{
			"index.html": []byte("<canvas></canvas>"),
		}}),
	)

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID: "gen_snapshot",
		Requirement: generation.RequirementSnapshot{
			InitialPrompt: "做一个离线文本卡片",
			AdditionalMessages: []generation.Message{
				{Role: "user", Content: "补充：改成自由绘制画板", CreatedAt: confirmedAt.Add(-time.Minute)},
			},
			Target:              generation.TargetAuto,
			Locale:              "zh-TW",
			AllowedCapabilities: []string{"storage"},
			ConfirmedAt:         confirmedAt,
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Runtime != agent.RuntimeWeb {
		t.Fatalf("runtime = %q, want web selected from complete requirement", result.Runtime)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.requests))
	}
	request := provider.requests[0]
	expectedPrompt := strings.Join([]string{
		"Session ID: gen_snapshot",
		"Runtime: web",
		"Attempt: 1",
		"Initial requirement:",
		"做一个离线文本卡片",
		"Additional requirements:",
		"1. 补充：改成自由绘制画板",
		"Target: auto",
		"Locale: zh-TW",
		"Allowed capabilities:",
		"- storage",
	}, "\n")
	if request.UserPrompt != expectedPrompt {
		t.Fatalf("provider prompt = %q, want %q", request.UserPrompt, expectedPrompt)
	}
	if !strings.Contains(request.SystemPrompt, "CodeCard") || !request.JSONOutput || request.MaxTokens != 8192 {
		t.Fatalf("web model request = %#v", request)
	}
	if strings.Contains(request.UserPrompt, confirmedAt.Format(time.RFC3339)) {
		t.Fatalf("confirmed timestamp leaked into provider prompt: %q", request.UserPrompt)
	}
}

func TestCodingAgentRepairsInvalidNativeOutputAtMostThreeTimes(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{outputs: []string{
		`{"schemaVersion":1,"initialState":{},"root":{"id":"x","type":"Unknown"}}`,
		`{"schemaVersion":1,"initialState":{},"root":{"id":"x","type":"Text","props":{"text":"完成"}}}`,
	}}
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_01",
		Requirement: confirmedRequirement("做一个离线文本卡片", generation.TargetAuto, "zh-CN"),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Runtime != agent.RuntimeNative || result.Attempts != 2 {
		t.Fatalf("result = %#v", result)
	}
	if len(provider.requests) != 2 || !strings.Contains(provider.requests[1].UserPrompt, "Validation feedback:") {
		t.Fatalf("repair requests = %#v", provider.requests)
	}
}

func TestCodingAgentAggregatesUsageAcrossNativeRepairAttempts(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{responses: []modelprovider.Response{
		{
			Content:      `{"schemaVersion":1,"initialState":{},"root":{"id":"x","type":"Unknown"}}`,
			InputTokens:  10,
			OutputTokens: 20,
		},
		{
			Content:      `{"schemaVersion":1,"initialState":{},"root":{"id":"x","type":"Text","props":{"text":"完成"}}}`,
			InputTokens:  30,
			OutputTokens: 40,
		},
	}}
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_usage_native",
		Requirement: confirmedRequirement("做一个离线文本卡片", generation.TargetNative, "zh-CN"),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Attempts != 2 || result.InputTokens != 40 || result.OutputTokens != 60 {
		t.Fatalf("result usage = %#v", result)
	}
}

func TestCodingAgentReturnsPartialUsageAfterNativeValidationExhaustion(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{responses: []modelprovider.Response{
		{Content: `{}`, InputTokens: 1, OutputTokens: 2},
		{Content: `{}`, InputTokens: 3, OutputTokens: 4},
		{Content: `{}`, InputTokens: 5, OutputTokens: 6},
	}}
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_usage_exhausted",
		Requirement: confirmedRequirement("做一个离线文本卡片", generation.TargetNative, "zh-CN"),
	})
	if !errors.Is(err, agent.ErrValidationFailed) {
		t.Fatalf("Generate() error = %v, want ErrValidationFailed", err)
	}
	if result.Runtime != agent.RuntimeNative || result.Reason == "" || result.Attempts != 3 ||
		result.InputTokens != 9 || result.OutputTokens != 12 {
		t.Fatalf("partial result = %#v", result)
	}
	if result.Content != "" || result.Files != nil {
		t.Fatalf("partial result exposed invalid output = %#v", result)
	}
}

func TestCodingAgentReturnsPartialUsageWhenNativeProviderFails(t *testing.T) {
	t.Parallel()

	providerFailure := errors.New("provider-sensitive-marker")
	provider := &fakeProvider{
		responses: []modelprovider.Response{
			{Content: `{}`, InputTokens: 7, OutputTokens: 11},
			{Content: "native-provider-error-sensitive-content", InputTokens: 13, OutputTokens: 17},
		},
		errors: []error{nil, providerFailure},
	}
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_usage_provider_error",
		Requirement: confirmedRequirement("做一个离线文本卡片", generation.TargetNative, "zh-CN"),
	})
	if !errors.Is(err, providerFailure) {
		t.Fatalf("Generate() error = %v, want provider failure", err)
	}
	if result.Runtime != agent.RuntimeNative || result.Reason == "" || result.Attempts != 2 ||
		result.InputTokens != 20 || result.OutputTokens != 28 {
		t.Fatalf("partial result = %#v", result)
	}
	if result.Content != "" || result.Files != nil {
		t.Fatalf("partial result exposed invalid output = %#v", result)
	}
}

func TestCodingAgentStopsAfterThreeInvalidOutputs(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{outputs: []string{"{}", "{}", "{}"}}
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())

	_, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_01",
		Requirement: confirmedRequirement("做一个卡片", generation.TargetNative, "zh-CN"),
	})

	if !errors.Is(err, agent.ErrValidationFailed) {
		t.Fatalf("Generate() error = %v, want ErrValidationFailed", err)
	}
	if len(provider.requests) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(provider.requests))
	}
}

func TestCodingAgentValidatesNativeCapabilitiesAgainstSnapshot(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{outputs: []string{
		`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Button","events":{"onPressed":[{"type":"capability.invoke","method":"notification.show","params":{"body":"done"}}]}}}`,
	}}
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())
	requirement := confirmedRequirement("完成后通知我", generation.TargetNative, "zh-CN")
	requirement.AllowedCapabilities = []string{"notification.show"}

	if _, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_capability",
		Requirement: requirement,
	}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestCodingAgentBuildsWebOutputOnlyThroughSandbox(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{outputs: []string{
		`{"files":{"src/card.tsx":"export function Card(){return <div>离线画板</div>}","src/card.css":"div{color:white}"}}`,
	}}
	builder := &fakeWebBuilder{
		output: map[string][]byte{
			"index.html":    []byte(`<script src="/runtime/bootstrap.js"></script><div id="app"></div>`),
			"assets/app.js": []byte("console.log('offline')"),
		},
	}
	codingAgent := agent.NewCodingAgent(
		provider,
		agent.NewNativeValidator(),
		agent.WithWebBuilder(builder),
	)

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_web",
		Requirement: confirmedRequirement("做一个自由绘制的离线画板", generation.TargetAuto, "zh-CN"),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Runtime != agent.RuntimeWeb || len(result.Files) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if builder.input["src/card.tsx"] == "" {
		t.Fatalf("builder input = %#v", builder.input)
	}
}

func TestCodingAgentAggregatesUsageAcrossWebBuildRepair(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{responses: []modelprovider.Response{
		{
			Content:      `{"files":{"src/card.tsx":"export function Card(){return <main>first</main>}"}}`,
			InputTokens:  13,
			OutputTokens: 17,
		},
		{
			Content:      `{"files":{"src/card.tsx":"export function Card(){return <main>second</main>}"}}`,
			InputTokens:  19,
			OutputTokens: 23,
		},
	}}
	builderFailure := errors.New("sandbox build failed")
	builder := &scriptedWebBuilder{
		errors: []error{builderFailure, nil},
		outputs: []map[string][]byte{
			nil,
			{"index.html": []byte("<main>second</main>")},
		},
	}
	codingAgent := agent.NewCodingAgent(
		provider,
		agent.NewNativeValidator(),
		agent.WithWebBuilder(builder),
	)

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_usage_web",
		Requirement: confirmedRequirement("做一个离线画板", generation.TargetWeb, "zh-CN"),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Runtime != agent.RuntimeWeb || result.Attempts != 2 ||
		result.InputTokens != 32 || result.OutputTokens != 40 || len(result.Files) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCodingAgentReturnsPartialUsageWhenWebProviderFails(t *testing.T) {
	t.Parallel()

	providerFailure := errors.New("provider-sensitive-marker")
	provider := &fakeProvider{
		responses: []modelprovider.Response{
			{Content: `{"files":{}}`, InputTokens: 29, OutputTokens: 31},
			{Content: "web-provider-error-sensitive-content", InputTokens: 37, OutputTokens: 41},
		},
		errors: []error{nil, providerFailure},
	}
	codingAgent := agent.NewCodingAgent(
		provider,
		agent.NewNativeValidator(),
		agent.WithWebBuilder(&fakeWebBuilder{}),
	)

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_usage_web_provider_error",
		Requirement: confirmedRequirement("做一个离线画板", generation.TargetWeb, "zh-CN"),
	})
	if !errors.Is(err, providerFailure) {
		t.Fatalf("Generate() error = %v, want provider failure", err)
	}
	if result.Runtime != agent.RuntimeWeb || result.Reason == "" || result.Attempts != 2 ||
		result.InputTokens != 66 || result.OutputTokens != 72 {
		t.Fatalf("partial result = %#v", result)
	}
	if result.Content != "" || result.Files != nil {
		t.Fatalf("partial result exposed invalid output = %#v", result)
	}
}

func TestCodingAgentReturnsPartialUsageAfterWebValidationExhaustion(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{responses: []modelprovider.Response{
		{Content: `{"files":{"src/card.tsx":"export const first = 1"}}`, InputTokens: 2, OutputTokens: 3},
		{Content: `{"files":{"src/card.tsx":"export const second = 2"}}`, InputTokens: 5, OutputTokens: 7},
		{Content: `{"files":{"src/card.tsx":"export const third = 3"}}`, InputTokens: 11, OutputTokens: 13},
	}}
	builder := &scriptedWebBuilder{
		errors: []error{errors.New("build one"), errors.New("build two"), errors.New("build three")},
		outputs: []map[string][]byte{
			{"index.html": []byte("sensitive-build-output-one")},
			{"index.html": []byte("sensitive-build-output-two")},
			{"index.html": []byte("sensitive-build-output-three")},
		},
	}
	codingAgent := agent.NewCodingAgent(
		provider,
		agent.NewNativeValidator(),
		agent.WithWebBuilder(builder),
	)

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_usage_web_exhausted",
		Requirement: confirmedRequirement("做一个离线画板", generation.TargetWeb, "zh-CN"),
	})
	if !errors.Is(err, agent.ErrValidationFailed) {
		t.Fatalf("Generate() error = %v, want ErrValidationFailed", err)
	}
	if result.Runtime != agent.RuntimeWeb || result.Reason == "" || result.Attempts != 3 ||
		result.InputTokens != 18 || result.OutputTokens != 23 {
		t.Fatalf("partial result = %#v", result)
	}
	if result.Content != "" || result.Files != nil {
		t.Fatalf("partial result exposed invalid output = %#v", result)
	}
}

func TestCodingAgentUnavailableWebBuilderDoesNotCallProvider(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{outputs: []string{`{"files":{"src/card.tsx":"unused"}}`}}
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_web_unavailable",
		Requirement: confirmedRequirement("做一个离线画板", generation.TargetWeb, "zh-CN"),
	})
	if !errors.Is(err, agent.ErrUnsupportedRequirement) {
		t.Fatalf("Generate() error = %v, want ErrUnsupportedRequirement", err)
	}
	if len(provider.requests) != 0 || result.Attempts != 0 || result.InputTokens != 0 || result.OutputTokens != 0 {
		t.Fatalf("provider calls=%d result=%#v", len(provider.requests), result)
	}
}

func TestCodingAgentRejectsWebSourceOutsideWhitelist(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{outputs: []string{
		`{"files":{"../../escape":"secret"}}`,
	}}
	codingAgent := agent.NewCodingAgent(
		provider,
		agent.NewNativeValidator(),
		agent.WithWebBuilder(&fakeWebBuilder{}),
	)

	_, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "gen_web",
		Requirement: confirmedRequirement("做一个离线画板", generation.TargetWeb, "zh-CN"),
	})
	if !errors.Is(err, agent.ErrValidationFailed) {
		t.Fatalf("Generate() error = %v, want ErrValidationFailed", err)
	}
}

type fakeProvider struct {
	outputs   []string
	responses []modelprovider.Response
	errors    []error
	requests  []modelprovider.Request
}

type fakeWebBuilder struct {
	input  map[string]string
	output map[string][]byte
}

type scriptedWebBuilder struct {
	errors  []error
	outputs []map[string][]byte
	calls   int
}

func (builder *fakeWebBuilder) Build(_ context.Context, files map[string]string) (map[string][]byte, error) {
	builder.input = files
	return builder.output, nil
}

func (builder *scriptedWebBuilder) Build(_ context.Context, _ map[string]string) (map[string][]byte, error) {
	index := builder.calls
	builder.calls++
	return builder.outputs[index], builder.errors[index]
}

func (provider *fakeProvider) Generate(_ context.Context, request modelprovider.Request) (modelprovider.Response, error) {
	provider.requests = append(provider.requests, request)
	index := len(provider.requests) - 1
	if len(provider.responses) > 0 {
		response := provider.responses[index]
		if index < len(provider.errors) {
			return response, provider.errors[index]
		}
		return response, nil
	}
	if index >= len(provider.outputs) {
		index = len(provider.outputs) - 1
	}
	return modelprovider.Response{Content: provider.outputs[index]}, nil
}

func confirmedRequirement(
	prompt string,
	target generation.Target,
	locale string,
) generation.RequirementSnapshot {
	return generation.RequirementSnapshot{
		InitialPrompt:       prompt,
		AdditionalMessages:  []generation.Message{},
		Target:              target,
		Locale:              locale,
		AllowedCapabilities: []string{"storage"},
		ConfirmedAt:         time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
	}
}
