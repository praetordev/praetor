package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
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

		// 5. Resolve Project from Template - REQUIRES a template with a project
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

		// Sync from Git project (if provided)
		var projectURL string
		if template.ProjectID != nil {
			var project models.Project
			err = tx.GetContext(ctx, &project, "SELECT * FROM projects WHERE id = $1", *template.ProjectID)
			if err != nil {
				log.Printf("Failed to find project %d for template %s: %v", *template.ProjectID, template.Name, err)
				_, _ = tx.ExecContext(ctx, "UPDATE unified_jobs SET status = 'failed' WHERE id = $1", job.ID)
				continue
			}
			projectURL = project.SCMURL
			log.Printf("Using project %s (%s) for job %d", project.Name, project.SCMURL, job.ID)
		} else {
			log.Printf("Template %s has no project - using default/inline logic for job %d", template.Name, job.ID)
		}

		// 6. Generate inventory from structured hosts and groups
		var inventoryContent string
		if template.InventoryID != nil {
			var inventory models.Inventory
			err = tx.GetContext(ctx, &inventory, "SELECT * FROM inventories WHERE id = $1", *template.InventoryID)
			if err != nil {
				log.Printf("Failed to find inventory %d for template %s: %v", *template.InventoryID, template.Name, err)
				_, _ = tx.ExecContext(ctx, "UPDATE unified_jobs SET status = 'failed' WHERE id = $1", job.ID)
				continue
			}

			// Fetch all hosts in this inventory
			var hosts []models.Host
			err = tx.SelectContext(ctx, &hosts, "SELECT * FROM hosts WHERE inventory_id = $1 AND enabled = true", *template.InventoryID)
			if err != nil {
				log.Printf("Failed to fetch hosts for inventory %d: %v", *template.InventoryID, err)
				_, _ = tx.ExecContext(ctx, "UPDATE unified_jobs SET status = 'failed' WHERE id = $1", job.ID)
				continue
			}

			// Fetch all groups in this inventory
			var groups []models.Group
			err = tx.SelectContext(ctx, &groups, "SELECT * FROM groups WHERE inventory_id = $1", *template.InventoryID)
			if err != nil {
				log.Printf("Failed to fetch groups for inventory %d: %v", *template.InventoryID, err)
			}

			// Build INI inventory
			inventoryContent = generateInventoryINI(tx, ctx, hosts, groups)
			log.Printf("Generated inventory for %s (Job %d) Content:\n%s", inventory.Name, job.ID, inventoryContent)
			log.Printf("Generated inventory for %s with %d hosts and %d groups for job %d", inventory.Name, len(hosts), len(groups), job.ID)

			if len(hosts) == 0 {
				log.Printf("Inventory %s has no enabled hosts - proceeding anyway to allow Ansible to handle it (e.g. localhost or group vars)", inventory.Name)
			}
		} else {
			log.Printf("Template %s has no inventory - using default localhost for job %d", template.Name, job.ID)
			// inventoryContent remains empty, Executor will default to localhost
		}

		var pbContent string
		if template.PlaybookContent != nil {
			pbContent = *template.PlaybookContent
		}

		manifest := events.JobManifest{
			Inventory:       inventoryContent,
			ProjectURL:      projectURL,
			Playbook:        template.Playbook,
			PlaybookContent: pbContent,
			ExtraVars:       map[string]interface{}{},
			EnvironmentRefs: []string{},
		}

		req := &events.ExecutionRequest{
			ExecutionRunID: runID,
			UnifiedJobID:   job.ID,
			JobManifest:    manifest,
			CreatedAt:      time.Now(),
		}

		log.Printf("Publishing ExecutionRequest for Job %d. Playbook: %s, PlaybookContent Length: %d", job.ID, manifest.Playbook, len(manifest.PlaybookContent))

		if err := s.Publisher.PublishExecutionRequest(req); err != nil {
			log.Printf("Failed to publish execution request for run %s: %v", runID, err)
		}

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

// generateInventoryINI converts structured hosts and groups to Ansible INI format
func generateInventoryINI(tx *sqlx.Tx, ctx context.Context, hosts []models.Host, groups []models.Group) string {
	var sb strings.Builder

	// Build map of host ID to groups
	hostGroups := make(map[int64][]string)
	ungroupedHosts := make(map[int64]bool)

	for _, h := range hosts {
		ungroupedHosts[h.ID] = true
	}

	// Process each group
	for _, g := range groups {
		// Get hosts in this group
		var groupHostIDs []int64
		tx.SelectContext(ctx, &groupHostIDs, "SELECT host_id FROM host_group_mapping WHERE group_id = $1", g.ID)

		if len(groupHostIDs) > 0 {
			sb.WriteString(fmt.Sprintf("[%s]\n", g.Name))

			for _, hostID := range groupHostIDs {
				// Find the host
				for _, h := range hosts {
					if h.ID == hostID {
						sb.WriteString(formatHostLine(h))
						delete(ungroupedHosts, h.ID)
						hostGroups[h.ID] = append(hostGroups[h.ID], g.Name)
						break
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	// Add ungrouped hosts under [all] if any
	if len(ungroupedHosts) > 0 {
		sb.WriteString("[ungrouped]\n")
		for _, h := range hosts {
			if ungroupedHosts[h.ID] {
				sb.WriteString(formatHostLine(h))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatHostLine formats a host with its variables for INI
func formatHostLine(h models.Host) string {
	var sb strings.Builder
	sb.WriteString(h.Name)

	// Parse variables JSON
	// Parse variables JSON
	var vars map[string]interface{}
	if h.Variables != nil {
		_ = json.Unmarshal(h.Variables, &vars)
	}
	if vars == nil {
		vars = make(map[string]interface{})
	}

	// Inject ControlMaster=no to prevent Docker crashes
	if val, ok := vars["ansible_ssh_common_args"]; ok {
		vars["ansible_ssh_common_args"] = fmt.Sprintf("%v -o ControlMaster=no", val)
	} else {
		vars["ansible_ssh_common_args"] = "-o StrictHostKeyChecking=no -o ControlMaster=no"
	}

	for k, v := range vars {
		// Quote string values if they contain spaces
		strVal := fmt.Sprintf("%v", v)
		if strings.Contains(strVal, " ") {
			sb.WriteString(fmt.Sprintf(" %s=\"%s\"", k, strVal))
		} else {
			sb.WriteString(fmt.Sprintf(" %s=%s", k, strVal))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// packProjectToTar creates a base64-encoded tar.gz of the entire in-memory filesystem
func packProjectToTar(fs billy.Filesystem) (string, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	err := util.Walk(fs, "/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip root
		if path == "/" {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Use relative path (strip leading /)
		if len(path) > 0 && path[0] == '/' {
			header.Name = path[1:]
		} else {
			header.Name = path
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Write file content if not a directory
		if !info.IsDir() {
			file, err := fs.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(tw, file)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	if err := tw.Close(); err != nil {
		return "", err
	}
	if err := gzw.Close(); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
