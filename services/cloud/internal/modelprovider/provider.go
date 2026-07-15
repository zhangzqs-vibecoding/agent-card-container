package modelprovider

import (
	"context"
	"errors"
)

var ErrRetryable = errors.New("model provider failure is retryable")

func IsRetryable(err error) bool {
	return errors.Is(err, ErrRetryable)
}

type retryableError struct {
	message string
}

func (err retryableError) Error() string {
	return err.message
}

func (err retryableError) Unwrap() error {
	return ErrRetryable
}

func newRetryableError(message string) error {
	return retryableError{message: message}
}

type Request struct {
	SystemPrompt string
	UserPrompt   string
	JSONOutput   bool
	MaxTokens    int
}

type Response struct {
	Content      string
	InputTokens  int
	OutputTokens int
}

type Provider interface {
	Generate(context.Context, Request) (Response, error)
}
