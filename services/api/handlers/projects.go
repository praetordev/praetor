package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/praetordev/praetor/pkg/models"
	"github.com/praetordev/praetor/services/api/render"
)

// ListProjects GET /api/v1/projects
// ListProjects GET /api/v1/projects
func (h *ContentHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	pg := render.ParsePagination(r)

	var projects []models.Project
	query := `SELECT * FROM projects ORDER BY id LIMIT $1 OFFSET $2`
	err := h.DB.Select(&projects, query, pg.Limit, pg.Offset)
	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}

	var total int64
	_ = h.DB.Get(&total, "SELECT count(*) FROM projects")

	if projects == nil {
		projects = []models.Project{}
	}

	render.JSON(w, r, &render.PaginatedResponse{
		Items:  projects,
		Total:  total,
		Limit:  pg.Limit,
		Offset: pg.Offset,
	})
}

// CreateProject POST /api/v1/projects
func (h *ContentHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var input models.Project
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}

	// Basic validation
	if input.Name == "" || input.SCMURL == "" || input.OrganizationID == 0 {
		render.ErrInvalidRequest(nil).Render(w, r)
		return
	}

	// Default SCM Type
	if input.SCMType == "" {
		input.SCMType = "git"
	}

	query := `
		INSERT INTO projects (organization_id, name, description, scm_type, scm_url, scm_branch) 
		VALUES (:organization_id, :name, :description, :scm_type, :scm_url, :scm_branch) 
		RETURNING *`

	rows, err := h.DB.NamedQuery(query, input)
	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}
	defer rows.Close()

	if rows.Next() {
		var created models.Project
		if err := rows.StructScan(&created); err != nil {
			render.ErrInternal(err).Render(w, r)
			return
		}
		render.Created(w, r, created)
	} else {
		render.ErrInternal(nil).Render(w, r)
	}
}
