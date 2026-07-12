package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

type Request struct {
	SessionID string
	Prompt    string
	Target    generation.Target
	Locale    string
}

type Result struct {
	Runtime  Runtime
	Reason   string
	Content  string
	Files    map[string][]byte
	Attempts int
}

type CodingAgent struct {
	provider   modelprovider.Provider
	validator  NativeValidator
	selector   Selector
	webBuilder WebBuilder
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

func NewCodingAgent(
	provider modelprovider.Provider,
	validator NativeValidator,
	options ...Option,
) *CodingAgent {
	codingAgent := &CodingAgent{provider: provider, validator: validator}
	for _, option := range options {
		option(codingAgent)
	}
	return codingAgent
}

func (codingAgent *CodingAgent) Generate(ctx context.Context, request Request) (Result, error) {
	decision, err := codingAgent.selector.Select(request.Prompt, request.Target)
	if err != nil {
		return Result{}, err
	}
	if decision.Runtime == RuntimeWeb {
		return codingAgent.generateWeb(ctx, request, decision)
	}
	var validationError string
	for attempt := 1; attempt <= 3; attempt++ {
		response, providerErr := codingAgent.provider.Generate(ctx, modelprovider.Request{
			SessionID:       request.SessionID,
			Prompt:          request.Prompt,
			Locale:          request.Locale,
			Runtime:         string(decision.Runtime),
			Attempt:         attempt,
			ValidationError: validationError,
		})
		if providerErr != nil {
			return Result{}, fmt.Errorf("model provider: %w", providerErr)
		}
		if validateErr := codingAgent.validator.Validate(response.Content); validateErr == nil {
			return Result{
				Runtime:  decision.Runtime,
				Reason:   decision.Reason,
				Content:  response.Content,
				Attempts: attempt,
			}, nil
		} else {
			validationError = validateErr.Error()
		}
	}
	return Result{}, fmt.Errorf("%w: %s", ErrValidationFailed, validationError)
}

func (codingAgent *CodingAgent) generateWeb(
	ctx context.Context,
	request Request,
	decision Decision,
) (Result, error) {
	if codingAgent.webBuilder == nil {
		return Result{}, fmt.Errorf("%w: CodeCard sandbox is unavailable", ErrUnsupportedRequirement)
	}
	var validationError string
	for attempt := 1; attempt <= 3; attempt++ {
		response, err := codingAgent.provider.Generate(ctx, modelprovider.Request{
			SessionID:       request.SessionID,
			Prompt:          request.Prompt,
			Locale:          request.Locale,
			Runtime:         string(decision.Runtime),
			Attempt:         attempt,
			ValidationError: validationError,
		})
		if err != nil {
			return Result{}, fmt.Errorf("model provider: %w", err)
		}
		files, err := decodeWebSource(response.Content)
		if err != nil {
			validationError = err.Error()
			continue
		}
		output, err := codingAgent.webBuilder.Build(ctx, files)
		if err != nil {
			validationError = err.Error()
			continue
		}
		if len(output) == 0 {
			validationError = "sandbox produced no files"
			continue
		}
		return Result{
			Runtime:  decision.Runtime,
			Reason:   decision.Reason,
			Files:    output,
			Attempts: attempt,
		}, nil
	}
	return Result{}, fmt.Errorf("%w: %s", ErrValidationFailed, validationError)
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
