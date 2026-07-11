package modelprovider

import "context"

type Request struct {
	SessionID       string
	Prompt          string
	Locale          string
	Runtime         string
	Attempt         int
	ValidationError string
}

type Response struct {
	Content      string
	InputTokens  int
	OutputTokens int
}

type Provider interface {
	Generate(context.Context, Request) (Response, error)
}
