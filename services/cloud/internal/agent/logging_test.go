package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
)

func TestCodingAgentLogsModelLifecycleWithoutContent(t *testing.T) {
	var output bytes.Buffer
	codingAgent := agent.NewCodingAgent(
		loggingProvider{content: `{"schemaVersion":1,"initialState":{"text":"ok"},"root":{"id":"root","type":"Text","props":{"text":{"path":"state.text"}}}}`},
		agent.NewNativeValidator(),
		agent.WithLogger(observability.NewJSONLogger(&output)),
		agent.WithModelName("deepseek-v4-flash"),
	)

	_, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "session-01",
		Requirement: confirmedRequirement("prompt-secret-value", generation.TargetNative, "zh-CN"),
	})
	if err != nil {
		t.Fatal(err)
	}

	events := loggedEvents(t, output.String())
	assertEventSequence(t, events, "model_request_started", "model_request_completed")
	for _, event := range events {
		if event["model"] != "deepseek-v4-flash" {
			t.Fatalf("model = %#v", event["model"])
		}
	}
	if strings.Contains(output.String(), "prompt-secret-value") || strings.Contains(output.String(), `initialState`) {
		t.Fatalf("model content leaked: %s", output.String())
	}
}

func TestCodingAgentLogsSandboxLifecycleForWebBuild(t *testing.T) {
	var output bytes.Buffer
	codingAgent := agent.NewCodingAgent(
		loggingProvider{content: `{"files":{"src/card.tsx":"export const secret='model-output'"}}`},
		agent.NewNativeValidator(),
		agent.WithWebBuilder(loggingWebBuilder{}),
		agent.WithLogger(observability.NewJSONLogger(&output)),
	)

	_, err := codingAgent.Generate(context.Background(), agent.Request{
		SessionID:   "session-web",
		Requirement: confirmedRequirement("画一个自由画板", generation.TargetWeb, "zh-CN"),
	})
	if err != nil {
		t.Fatal(err)
	}

	events := loggedEvents(t, output.String())
	assertEventSequence(t, events,
		"model_request_started", "model_request_completed",
		"sandbox_build_started", "sandbox_build_completed",
	)
	if strings.Contains(output.String(), "model-output") {
		t.Fatalf("model output leaked: %s", output.String())
	}
}

type loggingProvider struct{ content string }

func (provider loggingProvider) Generate(context.Context, modelprovider.Request) (modelprovider.Response, error) {
	return modelprovider.Response{Content: provider.content}, nil
}

type loggingWebBuilder struct{}

func (loggingWebBuilder) Build(context.Context, map[string]string) (map[string][]byte, error) {
	return map[string][]byte{"index.html": []byte("<main></main>")}, nil
}

func loggedEvents(t *testing.T, source string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(source), "\n")
	events := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

func assertEventSequence(t *testing.T, events []map[string]any, expected ...string) {
	t.Helper()
	if len(events) != len(expected) {
		t.Fatalf("events = %#v", events)
	}
	for index, name := range expected {
		if events[index]["event"] != name {
			t.Fatalf("event[%d] = %#v, want %q", index, events[index], name)
		}
	}
}
