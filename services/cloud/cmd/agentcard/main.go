package main

import (
	"log"
	"os"

	"github.com/zzq/agent-card-container/services/cloud/internal/app"
)

func main() {
	application := app.New("agent-card-cloud")
	address := os.Getenv("AGENTCARD_ADDR")
	if address == "" {
		address = ":8080"
	}
	log.Printf("%s listening on %s", application.Name(), address)
	if err := application.Run(address); err != nil {
		log.Fatal(err)
	}
}
