package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
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

type CloudHandlerConfig struct {
	ServiceName   string
	Generations   *generation.Service
	Publisher     *publish.Publisher
	Authenticator Authenticator
	NewRequestID  func() string
}

func NewCloudHandler(config CloudHandlerConfig) http.Handler {
	root := http.NewServeMux()
	root.Handle("GET /healthz", NewHandler(config.ServiceName))
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
	return root
}
