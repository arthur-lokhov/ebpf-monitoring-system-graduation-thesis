package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/epbf-monitoring/epbf-monitor/internal/plugin"
	pg "github.com/epbf-monitoring/epbf-monitor/internal/storage/postgres"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handlers holds API handlers dependencies
type Handlers struct {
	PluginService *plugin.Service
}

// NewHandlers creates new API handlers
func NewHandlers() *Handlers {
	return &Handlers{}
}

// SetPluginService sets the plugin service
func (h *Handlers) SetPluginService(s *plugin.Service) {
	h.PluginService = s
}

// Response helpers

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, err string, message string) {
	writeJSON(w, status, ErrorResponse{
		Error:   err,
		Message: message,
	})
}

func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Data: data})
}

// Health endpoint
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
		"version":   "0.1.0",
	}
	writeJSON(w, http.StatusOK, response)
}

// Metrics endpoint (Prometheus format)
func (h *Handlers) Metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	// TODO: Implement actual metrics export
	w.Write([]byte("# epbf-monitoring metrics\n"))
	w.Write([]byte("# HELP epbf_info Epbf monitoring service info\n"))
	w.Write([]byte("# TYPE epbf_info gauge\n"))
	w.Write([]byte("epbf_info{version=\"0.1.0\"} 1\n"))
}

// Plugin handlers

func (h *Handlers) ListPlugins(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		writeSuccess(w, []interface{}{})
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
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writeSuccess(w, plugins)
}

func (h *Handlers) AddPlugin(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Plugin service not initialized")
		return
	}

	var req struct {
		GitURL string `json:"git_url"`
		Ref    string `json:"ref,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	if req.GitURL == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "git_url is required")
		return
	}

	plugin, err := h.PluginService.LoadPlugin(r.Context(), req.GitURL, req.Ref)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, plugin)
}

func (h *Handlers) GetPlugin(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Plugin service not initialized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid plugin ID")
		return
	}

	plugin, err := h.PluginService.GetPlugin(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if plugin == nil {
		writeError(w, http.StatusNotFound, "not_found", "Plugin not found")
		return
	}

	writeSuccess(w, plugin)
}

func (h *Handlers) GetPluginByName(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Plugin service not initialized")
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "name is required")
		return
	}

	plugin, err := h.PluginService.GetPluginByName(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if plugin == nil {
		writeError(w, http.StatusNotFound, "not_found", "Plugin not found")
		return
	}

	writeSuccess(w, plugin)
}

func (h *Handlers) DeletePlugin(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Plugin service not initialized")
		return
	}

	idStr := chi.URLParam(r, "id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid plugin ID")
		return
	}

	// TODO: Implement delete
	// if err := h.PluginService.DeletePlugin(r.Context(), id); err != nil {
	// 	writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	// 	return
	// }

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) RebuildPlugin(w http.ResponseWriter, r *http.Request) {
	if h.PluginService == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Plugin service not initialized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid plugin ID")
		return
	}

	if err := h.PluginService.RebuildPlugin(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writeSuccess(w, map[string]string{
		"status": "rebuilding",
	})
}

// Metric handlers

func (h *Handlers) ListMetrics(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	metrics := []map[string]interface{}{}
	writeSuccess(w, metrics)
}

func (h *Handlers) GetMetric(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "metric name is required")
		return
	}

	// TODO: Get metric
	writeSuccess(w, map[string]interface{}{
		"name": name,
		"type": "counter",
	})
}

// Filter handlers

func (h *Handlers) ListFilters(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	filters := []map[string]interface{}{}
	writeSuccess(w, filters)
}

func (h *Handlers) CreateFilter(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	var req struct {
		Name       string `json:"name"`
		Expression string `json:"expression"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	filter := map[string]interface{}{
		"name":       req.Name,
		"expression": req.Expression,
	}

	writeJSON(w, http.StatusCreated, filter)
}

func (h *Handlers) DeleteFilter(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid filter ID")
		return
	}

	// TODO: Delete filter

	w.WriteHeader(http.StatusNoContent)
}

// Dashboard handlers

func (h *Handlers) GetDashboard(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	dashboard := map[string]interface{}{
		"version": 1,
		"panels":  []interface{}{},
	}
	writeSuccess(w, dashboard)
}

func (h *Handlers) UpdateDashboard(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	var config map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	writeSuccess(w, map[string]string{
		"status": "updated",
	})
}
