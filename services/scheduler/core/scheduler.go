package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/praetordev/praetor/pkg/events"
	"github.com/praetordev/praetor/pkg/models"
)

type Scheduler struct {
	DB        *sqlx.DB
	Ticker    *time.Ticker
	Done      chan bool
	Publisher EventPublisher
}

func NewScheduler(db *sqlx.DB, interval time.Duration, publisher EventPublisher) *Scheduler {
	return &Scheduler{
		DB:        db,
		Ticker:    time.NewTicker(interval),
		Done:      make(chan bool),
		Publisher: publisher,
	}
}

func (s *Scheduler) Start() {
	log.Println("Scheduler started")
	for {
		select {
		case <-s.Done:
			return
		case <-s.Ticker.C:
			if err := s.processPendingJobs(); err != nil {
				log.Printf("Error processing jobs: %v", err)
			}
		}
	}
}

func (s *Scheduler) Stop() {
	s.Ticker.Stop()
	s.Done <- true
	log.Println("Scheduler stopped")
}

func (s *Scheduler) processPendingJobs() error {
	ctx := context.Background()

	// Transaction for atomic claim-and-schedule
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Fetch pending jobs with SKIP LOCKED
	query := `
		SELECT id, name, unified_job_template_id, status 
		FROM unified_jobs 
		WHERE status = 'pending' AND current_run_id IS NULL
		FOR UPDATE SKIP LOCKED 
		LIMIT 10`

	var jobs []models.UnifiedJob
	if err := tx.SelectContext(ctx, &jobs, query); err != nil {
		return fmt.Errorf("failed to select pending jobs: %w", err)
	}

	if len(jobs) == 0 {
		return nil
	}

	for _, job := range jobs {
		// 3. Create Execution Run
		var runID uuid.UUID
		err := tx.QueryRowContext(ctx, `
			INSERT INTO execution_runs (unified_job_id, attempt_number, state) 
			VALUES ($1, 1, 'pending') 
			RETURNING id`, job.ID).Scan(&runID)

		if err != nil {
			log.Printf("Failed to create run for job %d: %v", job.ID, err)
			continue
		}

		// 4. Update Job
		_, err = tx.ExecContext(ctx, `
			UPDATE unified_jobs 
			SET status = 'queued', current_run_id = $1 
			WHERE id = $2`, runID, job.ID)

		if err != nil {
			log.Printf("Failed to update job %d: %v", job.ID, err)
			continue
		}

		// 5. Resolve Project/Playbook from Template - REQUIRES a template with a project
		var playbookContent string

		if job.UnifiedJobTemplateID == nil {
			log.Printf("Job %d has no template - skipping (template required)", job.ID)
			_, _ = tx.ExecContext(ctx, "UPDATE unified_jobs SET status = 'failed' WHERE id = $1", job.ID)
			continue
		}

		// Look up Template
		var template models.JobTemplate
		err = tx.GetContext(ctx, &template, "SELECT * FROM job_templates WHERE id = $1", *job.UnifiedJobTemplateID)
		if err != nil {
			log.Printf("Failed to find template %d for job %d: %v", *job.UnifiedJobTemplateID, job.ID, err)
			_, _ = tx.ExecContext(ctx, "UPDATE unified_jobs SET status = 'failed' WHERE id = $1", job.ID)
			continue
		}

		if template.ProjectID == nil {
			log.Printf("Template %s has no project - job %d failed", template.Name, job.ID)
			_, _ = tx.ExecContext(ctx, "UPDATE unified_jobs SET status = 'failed' WHERE id = $1", job.ID)
			continue
		}

		// Sync from Git project
		var project models.Project
		err = tx.GetContext(ctx, &project, "SELECT * FROM projects WHERE id = $1", *template.ProjectID)
		if err != nil || project.SCMURL == "" {
			log.Printf("Failed to find project %d for template %s: %v", *template.ProjectID, template.Name, err)
			_, _ = tx.ExecContext(ctx, "UPDATE unified_jobs SET status = 'failed' WHERE id = $1", job.ID)
			continue
		}

		log.Printf("Syncing project %s from %s for job %d", project.Name, project.SCMURL, job.ID)

		// Git Clone (In-Memory)
		fs := memfs.New()
		_, err = git.Clone(memory.NewStorage(), fs, &git.CloneOptions{
			URL: project.SCMURL,
		})
		if err != nil {
			log.Printf("Failed to clone project: %v", err)
			_, _ = tx.ExecContext(ctx, "UPDATE unified_jobs SET status = 'failed' WHERE id = $1", job.ID)
			continue
		}

		// Read Playbook File
		file, err := findFile(fs, template.Playbook)
		if err != nil {
			log.Printf("Failed to open playbook %s: %v", template.Playbook, err)
			_, _ = tx.ExecContext(ctx, "UPDATE unified_jobs SET status = 'failed' WHERE id = $1", job.ID)
			continue
		}

		buf := make([]byte, 1024*1024) // 1MB limit
		n, _ := file.Read(buf)
		playbookContent = string(buf[:n])
		file.Close()

		log.Printf("Loaded playbook %s (%d bytes) from project %s", template.Playbook, n, project.Name)

		// 6. Create Manifest with demo cluster inventory
		clusterInventory := `[webservers]
web1 ansible_user=root ansible_ssh_private_key_file=/home/praetor/.ssh/id_rsa ansible_ssh_common_args='-o StrictHostKeyChecking=no'
web2 ansible_user=root ansible_ssh_private_key_file=/home/praetor/.ssh/id_rsa ansible_ssh_common_args='-o StrictHostKeyChecking=no'

[databases]
db1 ansible_user=root ansible_ssh_private_key_file=/home/praetor/.ssh/id_rsa ansible_ssh_common_args='-o StrictHostKeyChecking=no'

[all:children]
webservers
databases`

		manifest := events.JobManifest{
			Inventory:       clusterInventory,
			ProjectContent:  playbookContent,
			ExtraVars:       map[string]interface{}{"stub_var": "true"},
			EnvironmentRefs: []string{"default_env"},
		}

		req := &events.ExecutionRequest{
			ExecutionRunID: runID,
			UnifiedJobID:   job.ID,
			JobManifest:    manifest,
			CreatedAt:      time.Now(),
		}

		if err := s.Publisher.PublishExecutionRequest(req); err != nil {
			log.Printf("Failed to publish execution request for run %s: %v", runID, err)
		}

		log.Printf("Scheduled Job %d -> Run %s", job.ID, runID)
	}

	return tx.Commit()
}

func findFile(fs billy.Filesystem, path string) (billy.File, error) {
	// Try direct
	f, err := fs.Open(path)
	if err == nil {
		return f, nil
	}

	// Try with leading slash
	if len(path) > 0 && path[0] != '/' {
		f, err = fs.Open("/" + path)
		if err == nil {
			return f, nil
		}
	}

	// Walk to find match
	var foundPath string
	_ = util.Walk(fs, "/", func(p string, info os.FileInfo, err error) error {
		if foundPath != "" {
			return nil
		}

		// Debug log
		// log.Printf(" - Visiting: %s", p)

		// Match strict but ignoring leading slash for comparison
		cleanP := p
		if len(cleanP) > 0 && cleanP[0] == '/' {
			cleanP = cleanP[1:]
		}
		cleanTarget := path
		if len(cleanTarget) > 0 && cleanTarget[0] == '/' {
			cleanTarget = cleanTarget[1:]
		}

		if cleanP == cleanTarget {
			foundPath = p
		}
		return nil
	})

	if foundPath != "" {
		return fs.Open(foundPath)
	}

	return nil, fmt.Errorf("file not found: %s", path)
}
