package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/praetordev/praetor/pkg/models"
	"github.com/praetordev/praetor/services/api/render"
)

type ContentHandler struct {
	DB *sqlx.DB
}

func NewContentHandler(db *sqlx.DB) *ContentHandler {
	return &ContentHandler{DB: db}
}

// ListOrganizations GET /api/v1/organizations
func (h *ContentHandler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	pg := render.ParsePagination(r)

	var orgs []models.Organization
	query := `SELECT * FROM organizations ORDER BY id LIMIT $1 OFFSET $2`
	err := h.DB.Select(&orgs, query, pg.Limit, pg.Offset)
	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}

	// Count total (simplified, improper for production but works for skeleton)
	var total int64
	_ = h.DB.Get(&total, "SELECT count(*) FROM organizations")

	// Ensure empty slice is JSON [] not null
	if orgs == nil {
		orgs = []models.Organization{}
	}

	render.JSON(w, r, &render.PaginatedResponse{
		Items:  orgs,
		Total:  total,
		Limit:  pg.Limit,
		Offset: pg.Offset,
	})
}

// CreateOrganization POST /api/v1/organizations
func (h *ContentHandler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	var input models.Organization
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}

	// Simple insert
	query := `
		INSERT INTO organizations (name, description) 
		VALUES (:name, :description) 
		RETURNING *`

	rows, err := h.DB.NamedQuery(query, input)
	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}
	defer rows.Close()

	if rows.Next() {
		var created models.Organization
		if err := rows.StructScan(&created); err != nil {
			render.ErrInternal(err).Render(w, r)
			return
		}
		render.Created(w, r, created)
	} else {
		render.ErrInternal(nil).Render(w, r)
	}
}
