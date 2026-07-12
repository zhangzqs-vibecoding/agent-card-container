package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/bootstrap"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
)

func main() {
	environment := environmentMap(os.Environ())
	runtime, err := bootstrap.NewFromEnvironment(environment)
	if err != nil {
		log.Fatal("invalid service configuration")
	}
	mode := environment["AGENTCARD_MODE"]
	if mode == "" {
		mode = "all"
	}
	if mode != "api" && mode != "worker" && mode != "all" {
		log.Fatal("AGENTCARD_MODE must be api, worker, or all")
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
		go runWorker(ctx, runtime)
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
			shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownContext)
		}()
		log.Printf("agent-card-cloud mode=%s listening=%s", mode, address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("cloud API stopped unexpectedly")
		}
		return
	}
	log.Printf("agent-card-cloud mode=%s started", mode)
	<-ctx.Done()
}

func runWorker(ctx context.Context, runtime *bootstrap.Runtime) {
	timer := time.NewTicker(500 * time.Millisecond)
	defer timer.Stop()
	for {
		_, err := runtime.RunWorkerOnce(ctx)
		if err != nil && !errors.Is(err, jobs.ErrNoJob) && ctx.Err() == nil {
			log.Print("generation worker task failed")
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
