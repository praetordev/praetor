package core

import (
	"encoding/json"
	"fmt"
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
	log.Printf("AnsibleRunner: Executing ansible-runner...")
	cmd := exec.Command("ansible-runner", "run", runDir, "-p", "playbook.yml", "-v")
	// cmd.Stdout = os.Stdout // Debug
	// cmd.Stderr = os.Stderr // Debug

	if err := cmd.Run(); err != nil {
		// Runner failed (could be playbook failure or system failure)
		log.Printf("AnsibleRunner: execution failed: %v", err)
		// We don't return error here immediately because we want to ensure events are flushed
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
	if inv == "" {
		inv = "localhost ansible_connection=local"
	}
	if err := os.WriteFile(filepath.Join(runDir, "inventory", "hosts"), []byte(inv), 0644); err != nil {
		return err
	}

	// Playbook
	play := req.JobManifest.ProjectContent
	if play == "" {
		// Default ping playbook
		play = "- hosts: all\n  tasks:\n    - name: Ping\n      ping:"
	}
	if err := os.WriteFile(filepath.Join(runDir, "project", "playbook.yml"), []byte(play), 0644); err != nil {
		return err
	}

	// ExtraVars
	// TODO: Marshal map to JSON/YAML and write to env/extravars

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
