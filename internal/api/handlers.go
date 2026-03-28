package api

import (
	"encoding/json"
	"net/http"

	"github.com/epbf-monitoring/epbf-monitor/internal/metrics"
	pg "github.com/epbf-monitoring/epbf-monitor/internal/storage/postgres"
	"github.com/epbf-monitoring/epbf-monitor/internal/plugin"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handlers holds API handlers dependencies
type Handlers struct {
	PluginService    *plugin.Service
	MetricsCollector *metrics.Collector
}

// NewHandlers creates new API handlers
func NewHandlers() *Handlers {
	return &Handlers{}
}

// SetPluginService sets the plugin service
func (h *Handlers) SetPluginService(s *plugin.Service) {
	h.PluginService = s
}

// SetMetrics sets the metrics collector
func (h *Handlers) SetMetrics(m *metrics.Collector) {
	h.MetricsCollector = m
}

// Health endpoint
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// Metrics endpoint (Prometheus format)
func (h *Handlers) Metrics(w http.ResponseWriter, r *http.Request) {
	if h.MetricsCollector == nil {
		http.Error(w, "Metrics collector not initialized", http.StatusServiceUnavailable)
		return
	}
	
	handler := promhttp.HandlerFor(h.MetricsCollector.Registry(), promhttp.HandlerOpts{})
	handler.ServeHTTP(w, r)
}

// Plugin handlers

func (h *Handlers) ListPlugins(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		http.Error(w, "Plugin service not initialized", http.StatusServiceUnavailable)
		return
	}

	// Get optional status filter
	statusParam := r.URL.Query().Get("status")
	var status *pg.PluginStatus
	if statusParam != "" {
		s := pg.PluginStatus(statusParam)
		status = &s
	}

	plugins, err := h.PluginService.ListPlugins(r.Context(), status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    plugins,
	})
}

func (h *Handlers) AddPlugin(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		http.Error(w, "Plugin service not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		GitURL string `json:"git_url"`
		Ref    string `json:"ref,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.GitURL == "" {
		http.Error(w, "git_url required", http.StatusBadRequest)
		return
	}

	// Record metric
	h.MetricsCollector.PluginBuildStarted(req.GitURL)

	// Load plugin
	plugin, err := h.PluginService.LoadPlugin(r.Context(), req.GitURL, req.Ref)
	if err != nil {
		h.MetricsCollector.PluginBuildFailure(req.GitURL, 0)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(plugin)
}

func (h *Handlers) GetPlugin(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		http.Error(w, "Plugin service not initialized", http.StatusServiceUnavailable)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid plugin ID", http.StatusBadRequest)
		return
	}

	plugin, err := h.PluginService.GetPlugin(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if plugin == nil {
		http.Error(w, "Plugin not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    plugin,
	})
}

func (h *Handlers) DeletePlugin(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		http.Error(w, "Plugin service not initialized", http.StatusServiceUnavailable)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid plugin ID", http.StatusBadRequest)
		return
	}

	if err := h.PluginService.DeletePlugin(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) RebuildPlugin(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		http.Error(w, "Plugin service not initialized", http.StatusServiceUnavailable)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid plugin ID", http.StatusBadRequest)
		return
	}

	if err := h.PluginService.RebuildPlugin(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "rebuilding",
	})
}

// Metric handlers

func (h *Handlers) ListMetrics(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success":true,"data":[]}`))
}

func (h *Handlers) GetMetric(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Metric name required", http.StatusBadRequest)
		return
	}
	
	// TODO: Implement
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success":true,"data":{"name":"` + name + `"}}`))
}

// Filter handlers

func (h *Handlers) ListFilters(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success":true,"data":[]}`))
}

func (h *Handlers) CreateFilter(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"success":true}`))
}

func (h *Handlers) DeleteFilter(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid filter ID", http.StatusBadRequest)
		return
	}
	
	// TODO: Implement
	w.WriteHeader(http.StatusNoContent)
}

// Dashboard handlers

func (h *Handlers) GetDashboard(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success":true,"data":{"version":1,"panels":[]}}`))
}

func (h *Handlers) UpdateDashboard(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success":true}`))
}
