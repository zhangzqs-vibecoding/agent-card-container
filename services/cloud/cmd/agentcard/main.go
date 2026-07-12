package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/bootstrap"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
)

func main() {
	logger := observability.NewJSONLogger(os.Stdout)
	environment := environmentMap(os.Environ())
	runtime, err := bootstrap.NewFromEnvironmentWithLogger(environment, logger)
	if err != nil {
		logger.Error("service_configuration_failed", "errorKind", "invalid_configuration")
		os.Exit(1)
	}
	defer func() { _ = runtime.Close() }()
	mode := environment["AGENTCARD_MODE"]
	if mode == "" {
		mode = "all"
	}
	if mode != "api" && mode != "worker" && mode != "all" {
		logger.Error("service_configuration_failed", "errorKind", "invalid_mode")
		os.Exit(1)
	}
	address := environment["AGENTCARD_ADDR"]
	if address == "" {
		address = ":8080"
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	if mode == "worker" || mode == "all" {
		go runWorker(ctx, runtime, logger)
	}
	if mode == "api" || mode == "all" {
		server := &http.Server{
			Addr:              address,
			Handler:           runtime.Handler(),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		go func() {
			<-ctx.Done()
			logger.Info("service_stopping", "mode", mode)
			shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownContext)
		}()
		logger.Info("service_started", "mode", mode, "address", address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("service_failed", "mode", mode, "errorKind", observability.ErrorKind(err))
			os.Exit(1)
		}
		logger.Info("service_stopped", "mode", mode)
		return
	}
	logger.Info("service_started", "mode", mode)
	<-ctx.Done()
	logger.Info("service_stopping", "mode", mode)
	logger.Info("service_stopped", "mode", mode)
}

func runWorker(ctx context.Context, runtime *bootstrap.Runtime, logger *slog.Logger) {
	timer := time.NewTicker(500 * time.Millisecond)
	defer timer.Stop()
	for {
		_, err := runtime.RunWorkerOnce(ctx)
		if err != nil && !errors.Is(err, jobs.ErrNoJob) && ctx.Err() == nil {
			logger.WarnContext(ctx, "worker_loop_failed", "errorKind", observability.ErrorKind(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func environmentMap(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		for index := 0; index < len(value); index++ {
			if value[index] == '=' {
				result[value[:index]] = value[index+1:]
				break
			}
		}
	}
	return result
}
