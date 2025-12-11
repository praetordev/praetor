package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"github.com/praetordev/praetor/pkg/models"
	"github.com/praetordev/praetor/services/api/render"
)

// HostsResource handles host operations within inventories
type HostsResource struct {
	DB *sqlx.DB
}

// NewHostsResource creates a new hosts resource handler
func NewHostsResource(db *sqlx.DB) *HostsResource {
	return &HostsResource{DB: db}
}

// Routes creates a REST router for hosts
func (rs *HostsResource) Routes() chi.Router {
	r := chi.NewRouter()
	// Nested under /inventories/{inventoryId}/hosts
	r.Get("/", rs.ListHosts)
	r.Post("/", rs.CreateHost)
	return r
}

// HostRoutes for individual host operations
func (rs *HostsResource) HostRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{hostId}", rs.GetHost)
	r.Put("/{hostId}", rs.UpdateHost)
	r.Delete("/{hostId}", rs.DeleteHost)
	return r
}

// ListHosts GET /api/v1/inventories/{inventoryId}/hosts
func (rs *HostsResource) ListHosts(w http.ResponseWriter, r *http.Request) {
	inventoryIdStr := chi.URLParam(r, "inventoryId")
	inventoryId, err := strconv.ParseInt(inventoryIdStr, 10, 64)
	if err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}

	var hosts []models.Host
	query := `SELECT * FROM hosts WHERE inventory_id = $1 ORDER BY name`
	err = rs.DB.SelectContext(r.Context(), &hosts, query, inventoryId)
	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}

	if hosts == nil {
		hosts = []models.Host{}
	}

	render.JSON(w, r, hosts)
}

// CreateHost POST /api/v1/inventories/{inventoryId}/hosts
func (rs *HostsResource) CreateHost(w http.ResponseWriter, r *http.Request) {
	inventoryIdStr := chi.URLParam(r, "inventoryId")
	inventoryId, err := strconv.ParseInt(inventoryIdStr, 10, 64)
	if err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}

	var input models.Host
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}

	if input.Name == "" {
		render.ErrInvalidRequest(nil).Render(w, r)
		return
	}

	input.InventoryID = inventoryId

	// Default variables to empty object if nil
	if input.Variables == nil {
		input.Variables = json.RawMessage("{}")
	}

	query := `
		INSERT INTO hosts (inventory_id, name, description, variables, enabled) 
		VALUES ($1, $2, $3, $4, $5) 
		RETURNING *`

	var created models.Host
	err = rs.DB.QueryRowxContext(r.Context(), query,
		input.InventoryID, input.Name, input.Description,
		input.Variables, true,
	).StructScan(&created)

	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}

	render.Created(w, r, created)
}

// GetHost GET /api/v1/hosts/{hostId}
func (rs *HostsResource) GetHost(w http.ResponseWriter, r *http.Request) {
	hostIdStr := chi.URLParam(r, "hostId")
	hostId, err := strconv.ParseInt(hostIdStr, 10, 64)
	if err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}

	var host models.Host
	query := `SELECT * FROM hosts WHERE id = $1`
	err = rs.DB.GetContext(r.Context(), &host, query, hostId)
	if err != nil {
		render.ErrNotFound(nil).Render(w, r)
		return
	}

	render.JSON(w, r, host)
}

// UpdateHost PUT /api/v1/hosts/{hostId}
func (rs *HostsResource) UpdateHost(w http.ResponseWriter, r *http.Request) {
	hostIdStr := chi.URLParam(r, "hostId")
	hostId, err := strconv.ParseInt(hostIdStr, 10, 64)
	if err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}

	var input models.Host
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}

	query := `
		UPDATE hosts 
		SET name = $2, description = $3, variables = $4, enabled = $5, modified_at = now()
		WHERE id = $1 
		RETURNING *`

	var updated models.Host
	err = rs.DB.QueryRowxContext(r.Context(), query,
		hostId, input.Name, input.Description, input.Variables, input.Enabled,
	).StructScan(&updated)

	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}

	render.JSON(w, r, updated)
}

// DeleteHost DELETE /api/v1/hosts/{hostId}
func (rs *HostsResource) DeleteHost(w http.ResponseWriter, r *http.Request) {
	hostIdStr := chi.URLParam(r, "hostId")
	hostId, err := strconv.ParseInt(hostIdStr, 10, 64)
	if err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}

	query := `DELETE FROM hosts WHERE id = $1`
	_, err = rs.DB.ExecContext(r.Context(), query, hostId)
	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
