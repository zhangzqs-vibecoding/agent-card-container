package modelprovider

import "context"

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
