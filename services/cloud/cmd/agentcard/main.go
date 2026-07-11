package main

import (
	"log"

	"github.com/zzq/agent-card-container/services/cloud/internal/app"
)

func main() {
	application := app.New("agent-card-cloud")
	log.Printf("%s initialized", application.Name())
}
