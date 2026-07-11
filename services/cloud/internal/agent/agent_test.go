package agent_test

import (
	"context"
	"errors"
	"testing"

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

func TestCodingAgentRepairsInvalidNativeOutputAtMostThreeTimes(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{outputs: []string{
		`{"schemaVersion":1,"initialState":{},"root":{"id":"x","type":"Unknown"}}`,
		`{"schemaVersion":1,"initialState":{},"root":{"id":"x","type":"Text","props":{"text":"完成"}}}`,
	}}
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())

	result, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID: "gen_01",
		Prompt:    "做一个离线文本卡片",
		Target:    generation.TargetAuto,
		Locale:    "zh-CN",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Runtime != agent.RuntimeNative || result.Attempts != 2 {
		t.Fatalf("result = %#v", result)
	}
	if len(provider.requests) != 2 || provider.requests[1].ValidationError == "" {
		t.Fatalf("repair requests = %#v", provider.requests)
	}
}

func TestCodingAgentStopsAfterThreeInvalidOutputs(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{outputs: []string{"{}", "{}", "{}"}}
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())

	_, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID: "gen_01",
		Prompt:    "做一个卡片",
		Target:    generation.TargetNative,
	})

	if !errors.Is(err, agent.ErrValidationFailed) {
		t.Fatalf("Generate() error = %v, want ErrValidationFailed", err)
	}
	if len(provider.requests) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(provider.requests))
	}
}

type fakeProvider struct {
	outputs  []string
	requests []modelprovider.Request
}

func (provider *fakeProvider) Generate(_ context.Context, request modelprovider.Request) (modelprovider.Response, error) {
	provider.requests = append(provider.requests, request)
	index := len(provider.requests) - 1
	return modelprovider.Response{Content: provider.outputs[index]}, nil
}
