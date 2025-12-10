package main

import (
	"log"
	"os"
	"sort"
	"strings"

	"github.com/praetordev/praetor/pkg/db"
)

func main() {
	log.Println("Starting migration...")

	database, err := db.InitDB()
	if err != nil {
		log.Fatalf("DB Init failed: %v", err)
	}
	defer database.Close()

	// Read migrations dir
	files, err := os.ReadDir("db/migrations")
	if err != nil {
		log.Fatalf("Read dir failed: %v", err)
	}

	var ups []string
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".up.sql") {
			ups = append(ups, "db/migrations/"+f.Name())
		}
	}
	sort.Strings(ups)

	for _, f := range ups {
		log.Printf("Applying %s...", f)
		content, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("Read file %s failed: %v", f, err)
		}

		if _, err := database.Exec(string(content)); err != nil {
			log.Fatalf("Exec %s failed: %v", f, err)
		}
	}
	log.Println("Migration complete.")
}
