package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Router holds the HTTP router
type Router struct {
	mux *chi.Mux
}

// NewRouter creates a new HTTP router
func NewRouter() *Router {
	mux := chi.NewMux()

	// Middleware
	mux.Use(middleware.RequestID)
	mux.Use(middleware.RealIP)
	mux.Use(middleware.Logger)
	mux.Use(middleware.Recoverer)
	mux.Use(middleware.Timeout(60 * time.Second))

	// CORS
	mux.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Plugin-Name"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	return &Router{mux: mux}
}

// Mount attaches a handler to a route pattern
func (r *Router) Mount(pattern string, handler http.Handler) {
	r.mux.Mount(pattern, handler)
}

// Use adds middleware
func (r *Router) Use(middlewares ...func(http.Handler) http.Handler) {
	r.mux.Use(middlewares...)
}

// Get adds GET route
func (r *Router) Get(pattern string, handlerFn http.HandlerFunc) {
	r.mux.Get(pattern, handlerFn)
}

// Post adds POST route
func (r *Router) Post(pattern string, handlerFn http.HandlerFunc) {
	r.mux.Post(pattern, handlerFn)
}

// Put adds PUT route
func (r *Router) Put(pattern string, handlerFn http.HandlerFunc) {
	r.mux.Put(pattern, handlerFn)
}

// Delete adds DELETE route
func (r *Router) Delete(pattern string, handlerFn http.HandlerFunc) {
	r.mux.Delete(pattern, handlerFn)
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// SetupRoutes configures all API routes
func SetupRoutes(r *Router, handlers *Handlers) {
	// Health check
	r.Get("/health", handlers.Health)

	// Metrics endpoint (Prometheus format)
	r.Get("/metrics", handlers.Metrics)

	// WebSocket endpoint
	r.Get("/ws", handlers.WebSocket)

	// API v1
	r.Mount("/api/v1", apiV1Router(handlers))
}

// apiV1Router creates v1 API subrouter
func apiV1Router(h *Handlers) http.Handler {
	r := chi.NewRouter()

	// Plugins
	r.Route("/plugins", func(r chi.Router) {
		r.Get("/", h.ListPlugins)
		r.Post("/", h.AddPlugin)
		r.Get("/{id}", h.GetPlugin)
		r.Delete("/{id}", h.DeletePlugin)
		r.Post("/{id}/rebuild", h.RebuildPlugin)
		r.Post("/{id}/enable", h.EnablePlugin)
		r.Post("/{id}/disable", h.DisablePlugin)
	})

	// Metrics
	r.Route("/metrics", func(r chi.Router) {
		r.Get("/", h.ListMetrics)
		r.Get("/{name}", h.GetMetric)
		r.Post("/query", h.QueryMetrics)
		r.Get("/names", h.MetricNames)
		r.Get("/{metric}/labels/{label}", h.LabelValues)
	})

	// Filters
	r.Route("/filters", func(r chi.Router) {
		r.Get("/", h.ListFilters)
		r.Get("/{id}", h.GetFilter)
		r.Post("/", h.CreateFilter)
		r.Delete("/{id}", h.DeleteFilter)
		r.Post("/execute", h.ExecuteFilter)
	})

	// Dashboards
	r.Route("/dashboard", func(r chi.Router) {
		r.Get("/", h.GetDashboard)
		r.Get("/list", h.ListDashboards)
		r.Post("/", h.CreateDashboard)
		r.Put("/", h.UpdateDashboard)
		r.Delete("/{id}", h.DeleteDashboard)
	})

	return r
}
