package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/praetordev/praetor/pkg/models"
)

type JobsResource struct {
	DB *sqlx.DB
}

func NewJobsResource(db *sqlx.DB) *JobsResource {
	return &JobsResource{DB: db}
}

// Routes creates a REST router for the Jobs resource
func (rs *JobsResource) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", rs.ListUnifiedJobs)
	r.Post("/", rs.LaunchJob)
	r.Get("/runs/{runID}", rs.GetExecutionRun)
	r.Get("/runs/{runID}/events", rs.ListJobEvents)
	return r
}

// ListUnifiedJobs returns a list of unified jobs
func (rs *JobsResource) ListUnifiedJobs(w http.ResponseWriter, r *http.Request) {
	query := `SELECT * FROM unified_jobs ORDER BY created_at DESC LIMIT 50`
	jobs := []models.UnifiedJob{}
	if err := rs.DB.SelectContext(r.Context(), &jobs, query); err != nil {
		render.Render(w, r, ErrInternal(err))
		return
	}
	render.JSON(w, r, jobs)
}

// LaunchJob creates a new unified job with status 'pending'
func (rs *JobsResource) LaunchJob(w http.ResponseWriter, r *http.Request) {
	// Simple launch payload
	type LaunchRequest struct {
		UnifiedJobTemplateID int64  `json:"unified_job_template_id"`
		Name                 string `json:"name"`
	}
	var req LaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}

	// For MVP: Insert simple unified_job
	// In reality we'd look up template, etc.
	var jobID int64
	err := rs.DB.QueryRowContext(r.Context(), `
		INSERT INTO unified_jobs (name, unified_job_template_id, status, created_at)
		VALUES ($1, $2, 'pending', $3)
		RETURNING id`,
		req.Name, req.UnifiedJobTemplateID, time.Now(),
	).Scan(&jobID)

	if err != nil {
		render.Render(w, r, ErrInternal(err))
		return
	}

	// Return created job
	render.Status(r, http.StatusCreated)
	render.JSON(w, r, map[string]interface{}{"id": jobID, "status": "pending"})
}

// GetExecutionRun returns details of a specific execution run
func (rs *JobsResource) GetExecutionRun(w http.ResponseWriter, r *http.Request) {
	runIDStr := chi.URLParam(r, "runID")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}

	var run models.ExecutionRun
	err = rs.DB.GetContext(r.Context(), &run, `SELECT * FROM execution_runs WHERE id = $1`, runID)
	if err == sql.ErrNoRows {
		render.Render(w, r, ErrNotFound)
		return
	} else if err != nil {
		render.Render(w, r, ErrInternal(err))
		return
	}

	render.JSON(w, r, run)
}

// ListJobEvents returns all events for a specific execution run
func (rs *JobsResource) ListJobEvents(w http.ResponseWriter, r *http.Request) {
	runIDStr := chi.URLParam(r, "runID")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}

	query := `SELECT * FROM job_events WHERE execution_run_id = $1 ORDER BY seq ASC`
	events := []models.JobEvent{}
	if err := rs.DB.SelectContext(r.Context(), &events, query, runID); err != nil {
		render.Render(w, r, ErrInternal(err))
		return
	}
	render.JSON(w, r, events)
}

// -- Err Helpers (Basic) --

func ErrInternal(err error) render.Renderer {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: 500,
		StatusText:     "Internal Server Error",
		ErrorText:      err.Error(),
	}
}

func ErrInvalidRequest(err error) render.Renderer {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: 400,
		StatusText:     "Invalid Request",
		ErrorText:      err.Error(),
	}
}

var ErrNotFound = &ErrResponse{HTTPStatusCode: 404, StatusText: "Resource not found"}

type ErrResponse struct {
	Err            error  `json:"-"`
	HTTPStatusCode int    `json:"-"`
	StatusText     string `json:"status"`
	AppCode        int64  `json:"code,omitempty"`
	ErrorText      string `json:"error,omitempty"`
}

func (e *ErrResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, e.HTTPStatusCode)
	return nil
}
