package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/praetordev/praetor/pkg/events"
)

type DBWriter struct {
	DB *sqlx.DB
}

func NewDBWriter(db *sqlx.DB) *DBWriter {
	return &DBWriter{DB: db}
}

// WriteEvent projects a JobEvent into the database.
func (w *DBWriter) WriteEvent(ctx context.Context, evt events.JobEvent) error {
	tx, err := w.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Insert into job_event table
	// Note: We used int64 for ID in models, but typically events might be inserted with DEFAULT id.
	// We need to map JobEvent fields to DB columns.
	eventDataJSON, _ := json.Marshal(evt.EventData)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO job_events (
			unified_job_id, execution_run_id, seq, event_type, 
			host_id, task_name, play_name, event_data, stdout_snippet, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		evt.UnifiedJobID, evt.ExecutionRunID, evt.Seq, evt.EventType,
		nil, evt.TaskName, evt.PlayName, eventDataJSON, evt.StdoutSnippet, evt.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert job_event failed: %w", err)
	}

	// 2. Update execution_run state
	if err := w.updateRunState(ctx, tx, evt); err != nil {
		return fmt.Errorf("update run state failed: %w", err)
	}

	return tx.Commit()
}

func (w *DBWriter) updateRunState(ctx context.Context, tx *sqlx.Tx, evt events.JobEvent) error {
	var newState string
	var newStatus string // for unified_job
	finished := false

	switch evt.EventType {
	case "JOB_STARTED":
		newState = "running"
		newStatus = "running"
	case "JOB_COMPLETED":
		// Check successful/failed based on event data or convention?
		// For MVP assuming success if completed, but ideally we check rc.
		newState = "successful"
		newStatus = "successful"
		finished = true
	case "JOB_FAILED":
		newState = "failed"
		newStatus = "failed"
		finished = true
	default:
		// Normal task events don't change state
		return nil
	}

	// Update ExecutionRun
	queryRun := `UPDATE execution_runs SET state = $1, last_event_seq = $2`
	argsRun := []interface{}{newState, evt.Seq}

	if finished {
		queryRun += `, finished_at = $3 WHERE id = $4`
		argsRun = append(argsRun, evt.Timestamp, evt.ExecutionRunID)
	} else if newState == "running" {
		queryRun += `, started_at = $3 WHERE id = $4`
		argsRun = append(argsRun, evt.Timestamp, evt.ExecutionRunID)
	} else {
		queryRun += ` WHERE id = $3`
		argsRun = append(argsRun, evt.ExecutionRunID)
	}

	if _, err := tx.ExecContext(ctx, queryRun, argsRun...); err != nil {
		log.Printf("Failed to update execution_run %s: %v", evt.ExecutionRunID, err)
		return err
	}

	// Update UnifiedJob
	// Only update status if the execution_run matches current_run_id (optimistic check)
	// But simpler: just update unified_job status based on this run's progress.
	queryJob := `UPDATE unified_jobs SET status = $1`
	argsJob := []interface{}{newStatus}

	if finished {
		queryJob += `, finished_at = $2 WHERE id = $3`
		argsJob = append(argsJob, evt.Timestamp, evt.UnifiedJobID)
	} else if newStatus == "running" {
		queryJob += `, started_at = $2 WHERE id = $3`
		argsJob = append(argsJob, evt.Timestamp, evt.UnifiedJobID)
	} else {
		queryJob += ` WHERE id = $2`
		argsJob = append(argsJob, evt.UnifiedJobID)
	}

	if _, err := tx.ExecContext(ctx, queryJob, argsJob...); err != nil {
		log.Printf("Failed to update unified_job %d: %v", evt.UnifiedJobID, err)
		return err
	}

	return nil
}
