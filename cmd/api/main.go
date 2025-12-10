package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/praetordev/praetor/pkg/db"
	"github.com/praetordev/praetor/services/api"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	database, err := db.InitDB()
	if err != nil {
		log.Printf("Warning: Failed to connect to DB: %v", err)
		log.Println("Starting in NO-DB mode (endpoints will fail)")
	}

	router := api.NewRouter(database)

	fmt.Printf("Praetor API Service starting on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
