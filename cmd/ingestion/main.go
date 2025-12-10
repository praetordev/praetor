package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/praetordev/praetor/pkg/db"
	"github.com/praetordev/praetor/services/ingestion/core"
	"github.com/praetordev/praetor/services/ingestion/handler"
)

func main() {
	port := os.Getenv("INGESTION_PORT")
	if port == "" {
		port = "8081" // Distinct port from API (8080)
	}

	log.Println("Starting Ingestion Service...")

	// 1. DB
	database, err := db.InitDB()
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}

	// 2. Service & Handler
	svc := core.NewIngestionService(database)
	h := handler.NewIngestionHandler(svc)

	// 3. Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/api/v1/runs/{run_id}/events", h.Ingest)

	// 4. Start
	log.Printf("Ingestion listening on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
