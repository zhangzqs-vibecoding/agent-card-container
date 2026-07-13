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
	outputs  []string
	requests []modelprovider.Request
}

type fakeWebBuilder struct {
	input  map[string]string
	output map[string][]byte
}

func (builder *fakeWebBuilder) Build(_ context.Context, files map[string]string) (map[string][]byte, error) {
	builder.input = files
	return builder.output, nil
}

func (provider *fakeProvider) Generate(_ context.Context, request modelprovider.Request) (modelprovider.Response, error) {
	provider.requests = append(provider.requests, request)
	index := len(provider.requests) - 1
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
