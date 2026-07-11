package agent

import (
	"context"
	"fmt"

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
	Attempts int
}

type CodingAgent struct {
	provider  modelprovider.Provider
	validator NativeValidator
	selector  Selector
}

func NewCodingAgent(provider modelprovider.Provider, validator NativeValidator) *CodingAgent {
	return &CodingAgent{provider: provider, validator: validator}
}

func (codingAgent *CodingAgent) Generate(ctx context.Context, request Request) (Result, error) {
	decision, err := codingAgent.selector.Select(request.Prompt, request.Target)
	if err != nil {
		return Result{}, err
	}
	if decision.Runtime == RuntimeWeb {
		return Result{}, fmt.Errorf("%w: CodeCard requires sandbox build", ErrUnsupportedRequirement)
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
