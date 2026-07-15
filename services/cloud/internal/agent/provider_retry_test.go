package agent_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

func TestCodingAgentRetriesRetryableProviderErrors(t *testing.T) {
	tests := []struct {
		name          string
		target        generation.Target
		prompt        string
		validResponse string
		assertOutput  func(*testing.T, agent.Result)
	}{
		{
			name:          "native",
			target:        generation.TargetNative,
			prompt:        "做一个离线文本卡片",
			validResponse: "{\"schemaVersion\":1,\"initialState\":{\"text\":\"完成\"},\"root\":{\"id\":\"root\",\"type\":\"Text\",\"props\":{\"text\":{\"path\":\"state.text\"}}}}",
			assertOutput: func(t *testing.T, result agent.Result) {
				t.Helper()
				if result.Content == "" || result.Files != nil {
					t.Fatalf("native output = %#v", result)
				}
			},
		},
		{
			name:          "web",
			target:        generation.TargetWeb,
			prompt:        "做一个自由绘制的离线画板",
			validResponse: "{\"files\":{\"src/card.tsx\":\"export function Card(){return <canvas/>}\"}}",
			assertOutput: func(t *testing.T, result agent.Result) {
				t.Helper()
				if result.Content != "" || string(result.Files["index.html"]) != "<canvas></canvas>" {
					t.Fatalf("web output = %#v", result)
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			provider := &retryScriptProvider{steps: []retryProviderStep{
				{
					response: modelprovider.Response{
						Content:      "retry-sensitive-content",
						InputTokens:  2,
						OutputTokens: 3,
					},
					err: retryableProviderError("first"),
				},
				{
					response: modelprovider.Response{
						Content:      test.validResponse,
						InputTokens:  5,
						OutputTokens: 7,
					},
				},
			}}
			codingAgent := retryTestAgent(provider, test.target)

			startedAt := time.Now()
			result, err := codingAgent.Generate(context.Background(), agent.Request{
				SessionID:   "retry_success_" + test.name,
				Requirement: confirmedRequirement(test.prompt, test.target, "zh-CN"),
			})
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			if len(provider.requests) != 2 || result.Attempts != 2 ||
				result.InputTokens != 7 || result.OutputTokens != 10 {
				t.Fatalf("provider calls=%d result=%#v", len(provider.requests), result)
			}
			if elapsed := time.Since(startedAt); elapsed < 450*time.Millisecond {
				t.Fatalf("retry completed in %s, want 500ms backoff", elapsed)
			}
			test.assertOutput(t, result)
		})
	}
}

func TestCodingAgentBoundsRetryableProviderErrorsToThreeCalls(t *testing.T) {
	for _, target := range []generation.Target{generation.TargetNative, generation.TargetWeb} {
		target := target
		t.Run(string(target), func(t *testing.T) {
			provider := &retryScriptProvider{steps: []retryProviderStep{
				{
					response: modelprovider.Response{Content: "first-sensitive-content", InputTokens: 1, OutputTokens: 2},
					err:      retryableProviderError("first"),
				},
				{
					response: modelprovider.Response{Content: "second-sensitive-content", InputTokens: 3, OutputTokens: 5},
					err:      retryableProviderError("second"),
				},
				{
					response: modelprovider.Response{Content: "third-sensitive-content", InputTokens: 7, OutputTokens: 11},
					err:      retryableProviderError("third"),
				},
			}}
			codingAgent := retryTestAgent(provider, target)

			startedAt := time.Now()
			result, err := codingAgent.Generate(context.Background(), agent.Request{
				SessionID:   "retry_exhausted_" + string(target),
				Requirement: confirmedRequirement(retryTestPrompt(target), target, "zh-CN"),
			})
			if !errors.Is(err, modelprovider.ErrRetryable) {
				t.Fatalf("Generate() error = %v, want ErrRetryable", err)
			}
			if len(provider.requests) != 3 || result.Attempts != 3 ||
				result.InputTokens != 11 || result.OutputTokens != 18 {
				t.Fatalf("provider calls=%d result=%#v", len(provider.requests), result)
			}
			if result.Content != "" || result.Files != nil {
				t.Fatalf("failed result exposed provider output = %#v", result)
			}
			if elapsed := time.Since(startedAt); elapsed < 1400*time.Millisecond {
				t.Fatalf("retries completed in %s, want 500ms and 1s backoffs", elapsed)
			}
		})
	}
}

func TestCodingAgentReturnsNonRetryableProviderErrorsImmediately(t *testing.T) {
	for _, target := range []generation.Target{generation.TargetNative, generation.TargetWeb} {
		target := target
		t.Run(string(target), func(t *testing.T) {
			permanentError := errors.New("permanent-provider-error")
			provider := &retryScriptProvider{steps: []retryProviderStep{{
				response: modelprovider.Response{
					Content:      "permanent-sensitive-content",
					InputTokens:  13,
					OutputTokens: 17,
				},
				err: permanentError,
			}}}
			codingAgent := retryTestAgent(provider, target)

			result, err := codingAgent.Generate(context.Background(), agent.Request{
				SessionID:   "permanent_error_" + string(target),
				Requirement: confirmedRequirement(retryTestPrompt(target), target, "zh-CN"),
			})
			if !errors.Is(err, permanentError) {
				t.Fatalf("Generate() error = %v, want permanent error", err)
			}
			if len(provider.requests) != 1 || result.Attempts != 1 ||
				result.InputTokens != 13 || result.OutputTokens != 17 {
				t.Fatalf("provider calls=%d result=%#v", len(provider.requests), result)
			}
			if result.Content != "" || result.Files != nil {
				t.Fatalf("failed result exposed provider output = %#v", result)
			}
		})
	}
}

func TestCodingAgentCancellationStopsRetryWaitWithoutAnotherCall(t *testing.T) {
	for _, target := range []generation.Target{generation.TargetNative, generation.TargetWeb} {
		target := target
		t.Run(string(target), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			provider := &retryScriptProvider{
				steps: []retryProviderStep{{
					response: modelprovider.Response{
						Content:      "cancel-sensitive-content",
						InputTokens:  19,
						OutputTokens: 23,
					},
					err: retryableProviderError("cancelled"),
				}},
				onCall: func(call int) {
					if call == 1 {
						cancel()
					}
				},
			}
			codingAgent := retryTestAgent(provider, target)

			startedAt := time.Now()
			result, err := codingAgent.Generate(ctx, agent.Request{
				SessionID:   "retry_cancelled_" + string(target),
				Requirement: confirmedRequirement(retryTestPrompt(target), target, "zh-CN"),
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Generate() error = %v, want context.Canceled", err)
			}
			if len(provider.requests) != 1 || result.Attempts != 1 ||
				result.InputTokens != 19 || result.OutputTokens != 23 {
				t.Fatalf("provider calls=%d result=%#v", len(provider.requests), result)
			}
			if result.Content != "" || result.Files != nil {
				t.Fatalf("cancelled result exposed provider output = %#v", result)
			}
			if elapsed := time.Since(startedAt); elapsed >= 50*time.Millisecond {
				t.Fatalf("cancelled retry took %s, want less than 50ms", elapsed)
			}
		})
	}
}

type retryProviderStep struct {
	response modelprovider.Response
	err      error
}

type retryScriptProvider struct {
	steps    []retryProviderStep
	requests []modelprovider.Request
	onCall   func(int)
}

func (provider *retryScriptProvider) Generate(
	_ context.Context,
	request modelprovider.Request,
) (modelprovider.Response, error) {
	provider.requests = append(provider.requests, request)
	call := len(provider.requests)
	if provider.onCall != nil {
		provider.onCall(call)
	}
	if call > len(provider.steps) {
		return modelprovider.Response{}, errors.New("unexpected extra model call")
	}
	step := provider.steps[call-1]
	return step.response, step.err
}

func retryTestAgent(provider modelprovider.Provider, target generation.Target) *agent.CodingAgent {
	options := []agent.Option{}
	if target == generation.TargetWeb {
		options = append(options, agent.WithWebBuilder(&fakeWebBuilder{output: map[string][]byte{
			"index.html": []byte("<canvas></canvas>"),
		}}))
	}
	return agent.NewCodingAgent(provider, agent.NewNativeValidator(), options...)
}

func retryableProviderError(label string) error {
	return fmt.Errorf("%s temporary failure: %w", label, modelprovider.ErrRetryable)
}

func retryTestPrompt(target generation.Target) string {
	if target == generation.TargetWeb {
		return "做一个自由绘制的离线画板"
	}
	return "做一个离线文本卡片"
}
