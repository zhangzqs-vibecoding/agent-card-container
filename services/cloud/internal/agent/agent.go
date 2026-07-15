package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
)

type Request struct {
	SessionID   string
	Requirement generation.RequirementSnapshot
}

type Result struct {
	Runtime      Runtime
	Reason       string
	Content      string
	Files        map[string][]byte
	Attempts     int
	InputTokens  int
	OutputTokens int
}

const maxModelAttempts = 3

type CodingAgent struct {
	provider   modelprovider.Provider
	validator  NativeValidator
	selector   Selector
	webBuilder WebBuilder
	logger     *slog.Logger
	modelName  string
}

type WebBuilder interface {
	Build(context.Context, map[string]string) (map[string][]byte, error)
}

type Option func(*CodingAgent)

func WithWebBuilder(builder WebBuilder) Option {
	return func(codingAgent *CodingAgent) {
		codingAgent.webBuilder = builder
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(codingAgent *CodingAgent) {
		codingAgent.logger = logger
	}
}

func WithModelName(modelName string) Option {
	return func(codingAgent *CodingAgent) {
		codingAgent.modelName = modelName
	}
}

func NewCodingAgent(
	provider modelprovider.Provider,
	validator NativeValidator,
	options ...Option,
) *CodingAgent {
	codingAgent := &CodingAgent{
		provider:  provider,
		validator: validator,
		logger:    slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	for _, option := range options {
		option(codingAgent)
	}
	return codingAgent
}

func (codingAgent *CodingAgent) Generate(ctx context.Context, request Request) (Result, error) {
	prompt := requirementPrompt(request.Requirement)
	decision, err := codingAgent.selector.Select(prompt, request.Requirement.Target)
	if err != nil {
		return Result{}, err
	}
	if decision.Runtime == RuntimeWeb {
		return codingAgent.generateWeb(ctx, request, decision)
	}
	result := Result{Runtime: decision.Runtime, Reason: decision.Reason}
	var validationError string
	providerRetries := 0
	for attempt := 1; attempt <= maxModelAttempts; attempt++ {
		if contextErr := ctx.Err(); contextErr != nil {
			return result, fmt.Errorf("model request cancelled: %w", contextErr)
		}
		result.Attempts = attempt
		startedAt := time.Now()
		codingAgent.logger.InfoContext(ctx, "model_request_started",
			"model", codingAgent.modelName,
			"sessionId", request.SessionID,
			"runtime", decision.Runtime,
			"attempt", attempt,
		)
		response, providerErr := codingAgent.provider.Generate(
			ctx,
			nativeModelRequest(request, attempt, validationError),
		)
		result.InputTokens += response.InputTokens
		result.OutputTokens += response.OutputTokens
		if providerErr != nil {
			codingAgent.logger.WarnContext(ctx, "model_request_failed",
				"model", codingAgent.modelName,
				"sessionId", request.SessionID,
				"runtime", decision.Runtime,
				"attempt", attempt,
				"durationMs", time.Since(startedAt).Milliseconds(),
				"errorKind", observability.ErrorKind(providerErr),
			)
			if modelprovider.IsRetryable(providerErr) && attempt < maxModelAttempts {
				if waitErr := waitForProviderRetry(ctx, providerRetries); waitErr != nil {
					return result, fmt.Errorf("model provider retry cancelled: %w", waitErr)
				}
				providerRetries++
				continue
			}
			return result, fmt.Errorf("model provider: %w", providerErr)
		}
		providerRetries = 0
		codingAgent.logger.InfoContext(ctx, "model_request_completed",
			"model", codingAgent.modelName,
			"sessionId", request.SessionID,
			"runtime", decision.Runtime,
			"attempt", attempt,
			"durationMs", time.Since(startedAt).Milliseconds(),
		)
		if validateErr := codingAgent.validator.Validate(response.Content, request.Requirement.AllowedCapabilities...); validateErr == nil {
			result.Content = response.Content
			return result, nil
		} else {
			validationError = validateErr.Error()
		}
	}
	return result, fmt.Errorf("%w: %s", ErrValidationFailed, validationError)
}

func (codingAgent *CodingAgent) generateWeb(
	ctx context.Context,
	request Request,
	decision Decision,
) (Result, error) {
	if codingAgent.webBuilder == nil {
		return Result{}, fmt.Errorf("%w: CodeCard sandbox is unavailable", ErrUnsupportedRequirement)
	}
	result := Result{Runtime: decision.Runtime, Reason: decision.Reason}
	var validationError string
	providerRetries := 0
	for attempt := 1; attempt <= maxModelAttempts; attempt++ {
		if contextErr := ctx.Err(); contextErr != nil {
			return result, fmt.Errorf("model request cancelled: %w", contextErr)
		}
		result.Attempts = attempt
		modelStartedAt := time.Now()
		codingAgent.logger.InfoContext(ctx, "model_request_started",
			"model", codingAgent.modelName,
			"sessionId", request.SessionID,
			"runtime", decision.Runtime,
			"attempt", attempt,
		)
		response, err := codingAgent.provider.Generate(
			ctx,
			webModelRequest(request, attempt, validationError),
		)
		result.InputTokens += response.InputTokens
		result.OutputTokens += response.OutputTokens
		if err != nil {
			codingAgent.logger.WarnContext(ctx, "model_request_failed",
				"model", codingAgent.modelName,
				"sessionId", request.SessionID,
				"runtime", decision.Runtime,
				"attempt", attempt,
				"durationMs", time.Since(modelStartedAt).Milliseconds(),
				"errorKind", observability.ErrorKind(err),
			)
			if modelprovider.IsRetryable(err) && attempt < maxModelAttempts {
				if waitErr := waitForProviderRetry(ctx, providerRetries); waitErr != nil {
					return result, fmt.Errorf("model provider retry cancelled: %w", waitErr)
				}
				providerRetries++
				continue
			}
			return result, fmt.Errorf("model provider: %w", err)
		}
		providerRetries = 0
		codingAgent.logger.InfoContext(ctx, "model_request_completed",
			"model", codingAgent.modelName,
			"sessionId", request.SessionID,
			"runtime", decision.Runtime,
			"attempt", attempt,
			"durationMs", time.Since(modelStartedAt).Milliseconds(),
		)
		files, err := decodeWebSource(response.Content)
		if err != nil {
			validationError = err.Error()
			continue
		}
		buildStartedAt := time.Now()
		codingAgent.logger.InfoContext(ctx, "sandbox_build_started",
			"sessionId", request.SessionID,
			"attempt", attempt,
		)
		output, err := codingAgent.webBuilder.Build(ctx, files)
		if err != nil {
			codingAgent.logger.WarnContext(ctx, "sandbox_build_failed",
				"sessionId", request.SessionID,
				"attempt", attempt,
				"durationMs", time.Since(buildStartedAt).Milliseconds(),
				"errorKind", observability.ErrorKind(err),
			)
			validationError = err.Error()
			continue
		}
		codingAgent.logger.InfoContext(ctx, "sandbox_build_completed",
			"sessionId", request.SessionID,
			"attempt", attempt,
			"durationMs", time.Since(buildStartedAt).Milliseconds(),
		)
		if len(output) == 0 {
			validationError = "sandbox produced no files"
			continue
		}
		result.Files = output
		return result, nil
	}
	return result, fmt.Errorf("%w: %s", ErrValidationFailed, validationError)
}

func providerRetryDelay(retry int) time.Duration {
	return 500 * time.Millisecond << retry
}

func waitForProviderRetry(ctx context.Context, retry int) error {
	timer := time.NewTimer(providerRetryDelay(retry))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func requirementPrompt(requirement generation.RequirementSnapshot) string {
	parts := []string{
		"Initial requirement:",
		requirement.InitialPrompt,
		"Additional requirements:",
	}
	if len(requirement.AdditionalMessages) == 0 {
		parts = append(parts, "(none)")
	} else {
		for index, message := range requirement.AdditionalMessages {
			parts = append(parts, fmt.Sprintf("%d. %s", index+1, message.Content))
		}
	}
	parts = append(parts,
		"Target: "+string(requirement.Target),
		"Locale: "+requirement.Locale,
		"Allowed capabilities:",
	)
	if len(requirement.AllowedCapabilities) == 0 {
		parts = append(parts, "(none)")
	} else {
		for _, capability := range requirement.AllowedCapabilities {
			parts = append(parts, "- "+capability)
		}
	}
	return strings.Join(parts, "\n")
}

func decodeWebSource(content string) (map[string]string, error) {
	var envelope struct {
		Files map[string]string `json:"files"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode CodeCard source: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("CodeCard source must contain one JSON value")
	}
	if len(envelope.Files) == 0 || len(envelope.Files) > 3 {
		return nil, fmt.Errorf("CodeCard source file count is invalid")
	}
	allowed := map[string]bool{
		"src/card.tsx":      true,
		"src/card.css":      true,
		"src/card.test.tsx": true,
	}
	if _, exists := envelope.Files["src/card.tsx"]; !exists {
		return nil, fmt.Errorf("CodeCard source is missing src/card.tsx")
	}
	total := 0
	for name, source := range envelope.Files {
		if !allowed[name] {
			return nil, fmt.Errorf("CodeCard source path %q is not allowed", name)
		}
		if len(source) > 256*1024 {
			return nil, fmt.Errorf("CodeCard source file %q is too large", name)
		}
		total += len(source)
	}
	if total > 512*1024 {
		return nil, fmt.Errorf("CodeCard source exceeds total size limit")
	}
	return envelope.Files, nil
}
