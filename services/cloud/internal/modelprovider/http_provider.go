package modelprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPConfig struct {
	BaseURL       string
	APIKey        string
	Model         string
	AllowInsecure bool
	Timeout       time.Duration
}

type HTTPProvider struct {
	endpoint *url.URL
	apiKey   string
	model    string
	client   *http.Client
}

func NewHTTPProvider(config HTTPConfig) (*HTTPProvider, error) {
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || !baseURL.IsAbs() || baseURL.Host == "" {
		return nil, fmt.Errorf("model base URL is invalid")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.ForceQuery || baseURL.Fragment != "" {
		return nil, fmt.Errorf("model base URL must not contain user info, query, or fragment")
	}
	if baseURL.Scheme != "https" && !(config.AllowInsecure && baseURL.Scheme == "http") {
		return nil, fmt.Errorf("model base URL must use HTTPS")
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("model API key is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 60 * time.Second
	}
	basePath := strings.TrimRight(baseURL.Path, "/")
	if basePath == "" {
		basePath = "/v1"
	} else if basePath != "/v1" && !strings.HasSuffix(basePath, "/v1") {
		basePath += "/v1"
	}
	baseURL.Path = basePath + "/chat/completions"
	baseURL.RawPath = ""
	return &HTTPProvider{
		endpoint: baseURL,
		apiKey:   config.APIKey,
		model:    config.Model,
		client: &http.Client{
			Timeout: config.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func NewHTTPProviderFromEnvironment(environment map[string]string) (*HTTPProvider, error) {
	return NewHTTPProvider(HTTPConfig{
		BaseURL:       environment["AGENTCARD_MODEL_BASE_URL"],
		APIKey:        environment["AGENTCARD_MODEL_API_KEY"],
		Model:         environment["AGENTCARD_MODEL"],
		AllowInsecure: environment["AGENTCARD_MODEL_ALLOW_INSECURE"] == "true",
	})
}

func (provider *HTTPProvider) Generate(ctx context.Context, input Request) (Response, error) {
	if input.MaxTokens <= 0 {
		return Response{}, fmt.Errorf("model max tokens must be positive")
	}
	responseFormat := (*chatResponseFormat)(nil)
	if input.JSONOutput {
		responseFormat = &chatResponseFormat{Type: "json_object"}
	}
	body := struct {
		Model          string              `json:"model"`
		Messages       []chatMessage       `json:"messages"`
		Temperature    float64             `json:"temperature"`
		ResponseFormat *chatResponseFormat `json:"response_format,omitempty"`
		MaxTokens      int                 `json:"max_tokens"`
		Thinking       chatThinking        `json:"thinking"`
	}{
		Model: provider.model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: input.SystemPrompt,
			},
			{
				Role:    "user",
				Content: input.UserPrompt,
			},
		},
		Temperature:    0.2,
		ResponseFormat: responseFormat,
		MaxTokens:      input.MaxTokens,
		Thinking:       chatThinking{Type: "disabled"},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return Response{}, fmt.Errorf("encode model request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.endpoint.String(), bytes.NewReader(encoded))
	if err != nil {
		return Response{}, fmt.Errorf("create model request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+provider.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	httpResponse, err := provider.client.Do(request)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return Response{}, fmt.Errorf("model request cancelled: %w", contextErr)
		}
		return Response{}, newRetryableError("model request failed")
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		if isRetryableHTTPStatus(httpResponse.StatusCode) {
			return Response{}, newRetryableError(fmt.Sprintf("model provider returned status %d", httpResponse.StatusCode))
		}
		return Response{}, fmt.Errorf("model provider returned status %d", httpResponse.StatusCode)
	}
	const maxResponseBytes = 2 * 1024 * 1024
	responseBytes, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes+1))
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return Response{}, fmt.Errorf("read model response cancelled: %w", contextErr)
		}
		return Response{}, newRetryableError("read model response failed")
	}
	if len(responseBytes) > maxResponseBytes {
		return Response{}, fmt.Errorf("model response exceeds size limit")
	}
	var decoded struct {
		Choices []struct {
			FinishReason string      `json:"finish_reason"`
			Message      chatMessage `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	decoder := json.NewDecoder(bytes.NewReader(responseBytes))
	if err := decoder.Decode(&decoded); err != nil {
		return Response{}, fmt.Errorf("decode model response failed")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Response{}, fmt.Errorf("decode model response failed")
	}
	response := Response{
		InputTokens:  decoded.Usage.PromptTokens,
		OutputTokens: decoded.Usage.CompletionTokens,
	}
	if len(decoded.Choices) == 0 {
		return response, fmt.Errorf("model response has no content")
	}
	choice := decoded.Choices[0]
	if choice.FinishReason != "stop" {
		if choice.FinishReason == "insufficient_system_resource" {
			return response, newRetryableError("model response did not complete successfully")
		}
		return response, fmt.Errorf("model response did not complete successfully")
	}
	if strings.TrimSpace(choice.Message.Content) == "" {
		return response, fmt.Errorf("model response has no content")
	}
	response.Content = choice.Message.Content
	return response, nil
}

func isRetryableHTTPStatus(statusCode int) bool {
	return statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= 500 && statusCode <= 599
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponseFormat struct {
	Type string `json:"type"`
}

type chatThinking struct {
	Type string `json:"type"`
}
