package reconciler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/praetordev/praetor/pkg/models"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Reconciler struct {
	DB        *sqlx.DB
	K8sClient kubernetes.Interface
	Ticker    *time.Ticker
	Done      chan bool
}

func NewReconciler(db *sqlx.DB, k8sClient kubernetes.Interface, interval time.Duration) *Reconciler {
	return &Reconciler{
		DB:        db,
		K8sClient: k8sClient,
		Ticker:    time.NewTicker(interval),
		Done:      make(chan bool),
	}
}

func (r *Reconciler) Start() {
	log.Println("Reconciler started")
	for {
		select {
		case <-r.Done:
			return
		case <-r.Ticker.C:
			if err := r.reconcileRuns(); err != nil {
				log.Printf("Error reconciling runs: %v", err)
			}
		}
	}
}

func (r *Reconciler) Stop() {
	r.Ticker.Stop()
	r.Done <- true
	log.Println("Reconciler stopped")
}

func (r *Reconciler) reconcileRuns() error {
	ctx := context.Background()

	// Fetch 'pending' or 'running' execution runs
	// TODO: Limit this query? Pagination? For skeleton, fetch latest 20 active.
	runs := []models.ExecutionRun{}
	query := `
		SELECT * FROM execution_runs 
		WHERE state IN ('pending', 'running') 
		ORDER BY created_at ASC 
		LIMIT 20`

	if err := r.DB.SelectContext(ctx, &runs, query); err != nil {
		return fmt.Errorf("failed to fetch runs: %w", err)
	}

	for _, run := range runs {
		if err := r.processRun(ctx, run); err != nil {
			log.Printf("Failed to process run %s: %v", run.ID, err)
		}
	}
	return nil
}

func (r *Reconciler) processRun(ctx context.Context, run models.ExecutionRun) error {
	jobName := fmt.Sprintf("praetor-run-%s", run.ID)
	namespace := "default" // TODO: Make configurable per Project?

	// 1. Check if K8s Job exists
	k8sJob, err := r.K8sClient.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})

	jobExists := err == nil

	// Handle Pending
	if run.State == "pending" {
		if !jobExists {
			// Create Job
			log.Printf("Creating K8s Job for run %s", run.ID)
			newJob := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: jobName,
					Labels: map[string]string{
						"praetor.io/run-id": run.ID.String(),
						"app":               "praetor-executor",
					},
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:    "ansible-runner",
									Image:   "alpine", // Stub image
									Command: []string{"echo", "Running Ansible Playbook... SUCCESS"},
								},
							},
						},
					},
				},
			}
			_, createErr := r.K8sClient.BatchV1().Jobs(namespace).Create(ctx, newJob, metav1.CreateOptions{})
			if createErr != nil {
				return fmt.Errorf("failed to create k8s job: %w", createErr)
			}

			// Update DB to 'running'
			// We assume creation means it's now tracked by K8s.
			// K8s Job Controller will launch pods.
			return r.updateState(run.ID, "running")
		} else {
			// Job already exists, maybe we missed updating state?
			return r.updateState(run.ID, "running")
		}
	}

	// Handle Running
	if run.State == "running" {
		if !jobExists {
			// Job gone? Failed or deleted?
			// Mark as stored? Or failed?
			log.Printf("Job %s missing for running execution %s", jobName, run.ID)
			return r.updateState(run.ID, "failed") // Or 'lost'
		}

		// Check Status
		// Simplification: Check for any Succeeded condition or Failed condition
		if k8sJob.Status.Succeeded > 0 {
			log.Printf("Job %s succeeded", jobName)
			return r.updateState(run.ID, "successful")
		} else if k8sJob.Status.Failed > 0 {
			log.Printf("Job %s failed", jobName)
			return r.updateState(run.ID, "failed")
		}
	}

	return nil
}

func (r *Reconciler) updateState(runID uuid.UUID, newState string) error {
	query := `UPDATE execution_runs SET state = $1, modified_at = NOW() WHERE id = $2`
	if newState == "successful" || newState == "failed" {
		query = `UPDATE execution_runs SET state = $1, finished_at = NOW() WHERE id = $2`
	} else if newState == "running" {
		query = `UPDATE execution_runs SET state = $1, started_at = NOW() WHERE id = $2`
	}

	_, err := r.DB.Exec(query, newState, runID)
	return err
}
