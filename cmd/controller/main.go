package main

import (
	"log"
	"os"

	"github.com/praetordev/praetor/services/controller/core"
)

func main() {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://127.0.0.1:4222"
	}

	controller, err := core.NewController(natsURL)
	if err != nil {
		log.Fatalf("Failed to initialize controller: %v", err)
	}

	controller.Start()
}
