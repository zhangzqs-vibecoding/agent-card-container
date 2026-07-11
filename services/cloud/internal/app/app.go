package app

import (
	"net/http"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/httpapi"
)

// App is the cloud service composition root.
type App struct {
	name string
}

// New constructs the cloud application.
func New(name string) *App {
	return &App{name: name}
}

// Name returns the stable service name.
func (a *App) Name() string {
	return a.name
}

// Handler exposes the composed cloud HTTP API.
func (a *App) Handler() http.Handler {
	return httpapi.NewHandler(a.name)
}

// Run starts the cloud HTTP server.
func (a *App) Run(address string) error {
	server := &http.Server{
		Addr:              address,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}
