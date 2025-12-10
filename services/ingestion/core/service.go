package core

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/praetordev/praetor/pkg/models"
)

type IngestionService struct {
	DB *sqlx.DB
}

func NewIngestionService(db *sqlx.DB) *IngestionService {
	return &IngestionService{DB: db}
}

// IngestEvents persists a batch of events.
func (s *IngestionService) IngestEvents(ctx context.Context, runID uuid.UUID, events []models.JobEvent) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Verify Run exists and is active?
	// For high throughput, we might skip reading if we trust the source.
	// But let's check it simply or catch FK error.

	insertQuery := `
		INSERT INTO job_events (
			execution_run_id, event_type, seq, created_at, event_data, stdout_snippet
		) VALUES (
			:execution_run_id, :event_type, :seq, :created_at, :event_data, :stdout_snippet
		)`

	for _, event := range events {
		// Ensure runID matches path
		event.ExecutionRunID = runID
		// ID is BIGSERIAL, let DB handle it if 0
		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now()
		}

		if _, err := tx.NamedExecContext(ctx, insertQuery, event); err != nil {
			log.Printf("Failed to insert event: %v", err)
			return fmt.Errorf("failed to insert event: %w", err)
		}
	}

	// Update last_event_seq? Not strictly required for MVP unless UI relies on it.
	// Let's reset the modified time to show activity.
	_, err = tx.ExecContext(ctx, `UPDATE execution_runs SET modified_at = NOW() WHERE id = $1`, runID)
	if err != nil {
		log.Printf("Failed to touch execution run: %v", err)
		// Non-fatal?
	}

	return tx.Commit()
}
