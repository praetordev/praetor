package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/praetordev/praetor/pkg/events"
)

func extractSeq(filename string) int {
	parts := strings.Split(filename, "-")
	if len(parts) > 0 {
		if val, err := strconv.Atoi(parts[0]); err == nil {
			return val
		}
	}
	return 0
}

type AnsibleRunner struct {
	BaseDir string
}

func NewAnsibleRunner() *AnsibleRunner {
	// Create base dir for runs
	base := "/tmp/praetor_runs"
	if err := os.MkdirAll(base, 0755); err != nil {
		log.Printf("Warning: Failed to create base dir %s: %v", base, err)
	}
	return &AnsibleRunner{BaseDir: base}
}

func (r *AnsibleRunner) Run(req *events.ExecutionRequest, eventChan chan<- events.JobEvent) error {
	runID := req.ExecutionRunID.String()
	runDir := filepath.Join(r.BaseDir, runID)
	log.Printf("AnsibleRunner: Preparing run %s in %s", runID, runDir)

	// 1. Prepare Directory Structure
	// ansible-runner expects:
	// private_data_dir/
	//   inventory/
	//     hosts
	//   project/
	//     playbook.yml
	//   env/
	//     extravars
	if err := r.prepareDirectory(runDir, req); err != nil {
		return fmt.Errorf("failed to prepare directory: %w", err)
	}

	// 2. Start Event Watcher
	// We watch artifacts/<ident>/job_events/ for *.json files
	// Since ansible-runner creates directories dynamically, we'll poll inside the goroutine
	doneChan := make(chan bool)
	go r.watchEvents(runDir, req, eventChan, doneChan)

	// 3. Exec ansible-runner with verbosity for detailed output
	// Use playbook path from manifest, default to playbook.yml
	playbookPath := req.JobManifest.Playbook
	if playbookPath == "" {
		playbookPath = "playbook.yml"
	}
	log.Printf("AnsibleRunner: Executing ansible-runner with playbook %s...", playbookPath)
	cmd := exec.Command("ansible-runner", "run", runDir, "-p", playbookPath, "-v")
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Emit explicit STARTED event
	eventChan <- events.JobEvent{
		ExecutionRunID: req.ExecutionRunID,
		UnifiedJobID:   req.UnifiedJobID,
		EventType:      "JOB_STARTED",
		Timestamp:      time.Now(),
	}

	if err := cmd.Run(); err != nil {
		// Runner failed (could be playbook failure or system failure)
		log.Printf("AnsibleRunner: execution failed: %v", err)
		log.Printf("AnsibleRunner Stdout: %s", stdoutBuf.String())
		log.Printf("AnsibleRunner Stderr: %s", stderrBuf.String())

		// Emit explicit failure event since runner output might not have been captured
		// Emit explicit failure event
		// We do NOT include Stderr here to avoid duplicating logs, as ansible-runner output is already streamed.
		// Only if the runner failed to start (no events) would this be critical, but we assume stream works.
		msg := fmt.Sprintf("Ansible execution process failed: %v", err)
		eventChan <- events.JobEvent{
			ExecutionRunID: req.ExecutionRunID,
			UnifiedJobID:   req.UnifiedJobID,
			EventType:      "JOB_FAILED",
			Timestamp:      time.Now(),
			StdoutSnippet:  &msg,
		}
	} else {
		// Emit explicit COMPLETED event on success
		eventChan <- events.JobEvent{
			ExecutionRunID: req.ExecutionRunID,
			UnifiedJobID:   req.UnifiedJobID,
			EventType:      "JOB_COMPLETED",
			Timestamp:      time.Now(),
		}
	}

	// 4. Cleanup / Finish
	close(doneChan)
	log.Printf("AnsibleRunner: Finished run %s", runID)

	// Emit completion event if not already emitted by watcher (usually watcher handles tasks, but Agent handles completion?
	// Actually, Agent code says: "Emit JOB_FAILED event if runner did not".
	// The watchers usually emit a recap or specific event.
	// For simplicity, we let the Agent handle the final JOB_COMPLETED/FAILED based on our return,
	// OR we ensure we emit at least JOB_STARTED/COMPLETED via the loop.
	// But let's assume successful return means success.

	// Wait a moment for watcher to drain?
	time.Sleep(1 * time.Second)

	return nil
}

func (r *AnsibleRunner) prepareDirectory(runDir string, req *events.ExecutionRequest) error {
	if err := os.MkdirAll(filepath.Join(runDir, "inventory"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(runDir, "project"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(runDir, "env"), 0755); err != nil {
		return err
	}

	// Inventory
	inv := req.JobManifest.Inventory
	log.Printf("AnsibleRunner: Received Inventory Length: %d", len(inv))
	if len(inv) > 100 {
		log.Printf("AnsibleRunner: Inventory Content Head: %s", inv[:100])
	} else {
		log.Printf("AnsibleRunner: Inventory Content: %s", inv)
	}

	if inv == "" {
		inv = "localhost ansible_connection=local"
	}
	if err := os.WriteFile(filepath.Join(runDir, "inventory", "hosts.ini"), []byte(inv), 0644); err != nil {
		return err
	}

	// Project - clone from git if URL provided
	projectURL := req.JobManifest.ProjectURL
	if projectURL != "" {
		// Clone the project repository
		projectDir := filepath.Join(runDir, "project")
		log.Printf("Cloning project from %s to %s", projectURL, projectDir)

		cmd := exec.Command("git", "clone", "--depth", "1", projectURL, projectDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to clone project: %w\n%s", err, string(output))
		}
		log.Printf("Cloned project successfully")
	} else {
		// Use inline playbook content if provided, otherwise default to ping
		play := req.JobManifest.PlaybookContent
		if play == "" {
			play = "- hosts: all\n  tasks:\n    - name: Ping\n      ping:"
		}

		if err := os.WriteFile(filepath.Join(runDir, "project", "playbook.yml"), []byte(play), 0644); err != nil {
			return err
		}
	}

	// ExtraVars
	// TODO: Marshal map to JSON/YAML and write to env/extravars

	return nil
}

// extractTarGz extracts a tar.gz byte slice to the target directory
func extractTarGz(data []byte, targetDir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		targetPath := filepath.Join(targetDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}

			outFile, err := os.Create(targetPath)
			if err != nil {
				return err
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()

			// Set permissions
			if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *AnsibleRunner) watchEvents(runDir string, req *events.ExecutionRequest, eventChan chan<- events.JobEvent, doneChan <-chan bool) {
	// Ansible Runner puts events in <runDir>/artifacts/<ident>/job_events/
	// But wait, if we run with `ansible-runner run <runDir>`, the artifact dir is usually <runDir>/artifacts/<uuid>/
	// getting that UUID is tricky unless we enforce it with --ident.
	// Let's rely on finding the newest directory in artifacts.

	var eventsDir string
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	seenEvents := make(map[string]bool)

	for {
		select {
		case <-doneChan:
			// Ensure we try to find the dir one last time if we haven't found it yet
			if eventsDir == "" {
				matches, _ := filepath.Glob(filepath.Join(runDir, "artifacts", "*", "job_events"))
				if len(matches) > 0 {
					eventsDir = matches[0]
				}
			}
			r.processNewEvents(eventsDir, req, eventChan, seenEvents)
			return
		case <-ticker.C:
			// Find dir if not found
			if eventsDir == "" {
				matches, _ := filepath.Glob(filepath.Join(runDir, "artifacts", "*", "job_events"))
				if len(matches) > 0 {
					eventsDir = matches[0] // Pick first one
				}
			}
			if eventsDir != "" {
				r.processNewEvents(eventsDir, req, eventChan, seenEvents)
			}
		}
	}
}

func (r *AnsibleRunner) processNewEvents(eventsDir string, req *events.ExecutionRequest, eventChan chan<- events.JobEvent, seenEvents map[string]bool) {
	if eventsDir == "" {
		return
	}
	files, err := os.ReadDir(eventsDir)
	if err != nil {
		return
	}

	// Sort files by the numeric prefix (e.g. "1-uuid.json" vs "10-uuid.json")
	sort.Slice(files, func(i, j int) bool {
		numI := extractSeq(files[i].Name())
		numJ := extractSeq(files[j].Name())
		return numI < numJ
	})

	var newFiles []string
	for _, f := range files {
		if !seenEvents[f.Name()] && filepath.Ext(f.Name()) == ".json" {
			newFiles = append(newFiles, f.Name())
		}
	}

	for _, fname := range newFiles {
		content, err := os.ReadFile(filepath.Join(eventsDir, fname))
		if err != nil {
			continue
		}

		var rawEvt map[string]interface{}
		if err := json.Unmarshal(content, &rawEvt); err != nil {
			continue
		}

		// Extract fields
		evtType, _ := rawEvt["event"].(string)
		counter, _ := rawEvt["counter"].(float64) // JSON numbers are floats

		// Map Ansible event to Praetor event
		praetorType := "UNKNOWN"
		switch evtType {
		case "playbook_on_start":
			praetorType = "JOB_STARTED"
		case "runner_on_ok":
			praetorType = "TASK_OK"
		case "playbook_on_stats":
			praetorType = "JOB_COMPLETED"
		case "runner_on_failed":
			praetorType = "TASK_FAILED"
		}

		// Helper to extract stdout safely
		var stdoutSnippet *string
		if s, ok := rawEvt["stdout"].(string); ok && s != "" {
			stdoutSnippet = new(string)
			*stdoutSnippet = s
		}

		// If we haven't mapped it to a lifecycle event, but it has output (e.g. headers), include it as generic
		if praetorType == "UNKNOWN" && stdoutSnippet != nil {
			praetorType = "JOB_LOG"
		}

		if praetorType != "UNKNOWN" {
			var taskName string
			if eventData, ok := rawEvt["event_data"].(map[string]interface{}); ok {
				if t, ok := eventData["task"].(string); ok {
					taskName = t
				}
			}

			eventChan <- events.JobEvent{
				ExecutionRunID: req.ExecutionRunID,
				UnifiedJobID:   req.UnifiedJobID,
				EventType:      praetorType,
				Timestamp:      time.Now(),
				Seq:            int64(counter),
				TaskName:       safeStringPtr(taskName),
				EventData:      json.RawMessage(content),
				StdoutSnippet:  stdoutSnippet,
			}
		}

		seenEvents[fname] = true
	}
}

func safeStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
