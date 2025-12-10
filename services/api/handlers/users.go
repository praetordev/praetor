package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/praetordev/praetor/pkg/models"
	"github.com/praetordev/praetor/services/api/render"
)

// ListUsers GET /api/v1/users
func (h *ContentHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	pg := render.ParsePagination(r)

	var users []models.User
	query := `SELECT id, username, first_name, last_name, email, is_superuser, is_active, created_at, modified_at FROM users ORDER BY id LIMIT $1 OFFSET $2`
	err := h.DB.Select(&users, query, pg.Limit, pg.Offset)
	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}

	var total int64
	_ = h.DB.Get(&total, "SELECT count(*) FROM users")

	if users == nil {
		users = []models.User{}
	}

	render.JSON(w, r, &render.PaginatedResponse{
		Items:  users,
		Total:  total,
		Limit:  pg.Limit,
		Offset: pg.Offset,
	})
}

// CreateUser POST /api/v1/users
func (h *ContentHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	// Simplified user creation (no password hashing implemented yet for skeleton)
	var input models.User
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}

	// Just assume password_hash is provided or empty for now
	query := `
		INSERT INTO users (username, password_hash, email, first_name, last_name, is_superuser) 
		VALUES (:username, 'placeholder_hash', :email, :first_name, :last_name, :is_superuser) 
		RETURNING id, username, email, first_name, last_name, is_superuser, is_active, created_at, modified_at`

	rows, err := h.DB.NamedQuery(query, input)
	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}
	defer rows.Close()

	if rows.Next() {
		var created models.User
		if err := rows.StructScan(&created); err != nil {
			render.ErrInternal(err).Render(w, r)
			return
		}
		render.Created(w, r, created)
	} else {
		render.ErrInternal(nil).Render(w, r)
	}
}
