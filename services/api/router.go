package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
	"github.com/jmoiron/sqlx"
	"github.com/praetordev/praetor/services/api/handlers"
	modelAuth "github.com/praetordev/praetor/services/api/middleware"
	praetorRender "github.com/praetordev/praetor/services/api/render"
)

// NewRouter instantiates the chi Router and wires middleware.
func NewRouter(db *sqlx.DB) *chi.Mux {
	r := chi.NewRouter()

	// Base Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)
	r.Use(render.SetContentType(render.ContentTypeJSON))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Stub Auth
	r.Use(modelAuth.StubAuthMiddleware)

	// Handlers
	content := handlers.NewContentHandler(db)

	// API v1 Routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			praetorRender.JSON(w, r, map[string]string{"status": "pong"})
		})

		r.Get("/organizations", content.ListOrganizations)
		r.Post("/organizations", content.CreateOrganization)

		r.Get("/users", content.ListUsers)
		r.Post("/users", content.CreateUser)

		r.Get("/projects", content.ListProjects)
		r.Post("/projects", content.CreateProject)
		r.Post("/projects/{id}/sync", content.SyncProject)

		jobs := handlers.NewJobsResource(db)
		r.Mount("/jobs", jobs.Routes())

		templates := handlers.NewTemplatesResource(db)
		r.Mount("/job-templates", templates.Routes())

		inventories := handlers.NewInventoriesResource(db)
		r.Mount("/inventories", inventories.Routes())

		// Nested hosts/groups under inventories
		hosts := handlers.NewHostsResource(db)
		groups := handlers.NewGroupsResource(db)

		r.Route("/inventories/{inventoryId}", func(r chi.Router) {
			r.Mount("/hosts", hosts.Routes())
			r.Mount("/groups", groups.Routes())
		})

		// Direct access to hosts and groups by ID
		r.Mount("/hosts", hosts.HostRoutes())
		r.Mount("/groups", groups.GroupRoutes())
	})

	return r
}
