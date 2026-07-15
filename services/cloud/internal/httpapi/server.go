package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type ReadinessChecker interface {
	Ready(context.Context) error
}

// NewHandler returns the cloud HTTP API.
func NewHandler(serviceName string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(writer).Encode(healthResponse{
			Status:  "ok",
			Service: serviceName,
		})
	})
	return mux
}

func NewReadinessHandler(serviceName string, checker ReadinessChecker) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /readyz", func(writer http.ResponseWriter, request *http.Request) {
		status := "unavailable"
		statusCode := http.StatusServiceUnavailable
		if checker != nil && checker.Ready(request.Context()) == nil {
			status = "ok"
			statusCode = http.StatusOK
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(statusCode)
		_ = json.NewEncoder(writer).Encode(healthResponse{
			Status:  status,
			Service: serviceName,
		})
	})
	return mux
}

type CloudHandlerConfig struct {
	ServiceName   string
	Generations   *generation.Service
	Publisher     *publish.Publisher
	Authenticator Authenticator
	NewRequestID  func() string
	Logger        *slog.Logger
	Now           func() time.Time
	Readiness     ReadinessChecker
}

func NewCloudHandler(config CloudHandlerConfig) http.Handler {
	root := http.NewServeMux()
	root.Handle("GET /healthz", NewHandler(config.ServiceName))
	root.Handle("GET /readyz", NewReadinessHandler(config.ServiceName, config.Readiness))
	generations := NewGenerationHandler(GenerationHandlerConfig{
		ServiceName:   config.ServiceName,
		Service:       config.Generations,
		Authenticator: config.Authenticator,
		NewRequestID:  config.NewRequestID,
	})
	cards := NewCardHandler(CardHandlerConfig{
		Publisher:     config.Publisher,
		Authenticator: config.Authenticator,
		NewRequestID:  config.NewRequestID,
	})
	root.Handle("/v1/generations", generations)
	root.Handle("/v1/generations/", generations)
	root.Handle("/v1/cards", cards)
	root.Handle("/v1/cards/", cards)
	return AccessLogMiddleware(AccessLogConfig{
		Logger:       config.Logger,
		NewRequestID: config.NewRequestID,
		Now:          config.Now,
	})(root)
}
