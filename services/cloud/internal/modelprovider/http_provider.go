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
	if baseURL.Scheme != "https" && !config.AllowInsecure {
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
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/v1/chat/completions"
	return &HTTPProvider{
		endpoint: baseURL,
		apiKey:   config.APIKey,
		model:    config.Model,
		client:   &http.Client{Timeout: config.Timeout},
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
	body := struct {
		Model       string        `json:"model"`
		Messages    []chatMessage `json:"messages"`
		Temperature float64       `json:"temperature"`
	}{
		Model: provider.model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: "你是 AgentCard 编码智能体。只输出满足宿主合同的最终 JSON，不使用 Markdown 代码块，不请求 Shell、任意文件系统或未授权能力。",
			},
			{
				Role: "user",
				Content: fmt.Sprintf(
					"session=%s\nruntime=%s\nlocale=%s\nattempt=%d\nrequirement=%s\npreviousValidationError=%s",
					input.SessionID,
					input.Runtime,
					input.Locale,
					input.Attempt,
					input.Prompt,
					input.ValidationError,
				),
			},
		},
		Temperature: 0.2,
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
		return Response{}, fmt.Errorf("model request failed")
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return Response{}, fmt.Errorf("model provider returned status %d", httpResponse.StatusCode)
	}
	const maxResponseBytes = 2 * 1024 * 1024
	responseBytes, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes+1))
	if err != nil {
		return Response{}, fmt.Errorf("read model response: %w", err)
	}
	if len(responseBytes) > maxResponseBytes {
		return Response{}, fmt.Errorf("model response exceeds size limit")
	}
	var decoded struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	decoder := json.NewDecoder(bytes.NewReader(responseBytes))
	if err := decoder.Decode(&decoded); err != nil {
		return Response{}, fmt.Errorf("decode model response: %w", err)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return Response{}, fmt.Errorf("model response has no content")
	}
	return Response{
		Content:      decoded.Choices[0].Message.Content,
		InputTokens:  decoded.Usage.PromptTokens,
		OutputTokens: decoded.Usage.CompletionTokens,
	}, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
