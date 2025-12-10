package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/praetordev/praetor/pkg/events"
	natsTransport "github.com/praetordev/praetor/pkg/transport/nats"
	"github.com/praetordev/praetor/services/executor/core"
)

func main() {
	log.Println("Starting Executor Agent...")

	// 1. Setup Infrastructure
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://127.0.0.1:4222"
	}
	bus, err := natsTransport.NewNatsBus(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer bus.Close()

	// runner := &core.MockRunner{}
	runner := core.NewAnsibleRunner()

	// Check for One-Shot Mode
	if os.Getenv("PRAETOR_MODE") == "oneshot" {
		log.Println("Starting in ONE-SHOT mode")
		manifestPath := os.Getenv("PRAETOR_MANIFEST_PATH")
		if manifestPath == "" {
			manifestPath = "/etc/praetor/manifest.json"
		}

		// Read manifest
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			log.Fatalf("Failed to read manifest at %s: %v", manifestPath, err)
		}

		var req events.ExecutionRequest
		if err := json.Unmarshal(data, &req); err != nil {
			log.Fatalf("Failed to unmarshal manifest: %v", err)
		}

		log.Printf("Loaded execution request %s for job %d", req.ExecutionRunID, req.UnifiedJobID)

		// Create event channel and publisher loop
		eventChan := make(chan events.JobEvent, 100)
		doneChan := make(chan bool)

		go func() {
			for evt := range eventChan {
				if err := bus.PublishJobEvent(&evt); err != nil {
					log.Printf("Failed to publish event: %v", err)
				}
			}
			doneChan <- true
		}()

		// Run job
		if err := runner.Run(&req, eventChan); err != nil {
			log.Printf("Job execution failed: %v", err)
			// TODO: Publish failure event?
			os.Exit(1)
		}

		close(eventChan)
		<-doneChan // Wait for events to flush
		log.Println("One-shot execution finished successfully.")
		return
	}

	// 2. Create Agent (Daemon Mode)
	agent := core.NewAgent(bus, bus, runner)

	// 3. Start
	if err := agent.Start(); err != nil {
		log.Fatalf("Agent failed: %v", err)
	}
}
