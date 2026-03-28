package api

import (
	"net/http"

	"github.com/epbf-monitoring/epbf-monitor/internal/metrics"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handlers holds API handlers dependencies
type Handlers struct {
	PluginService   interface{} // TODO: Add when ready
	MetricsCollector *metrics.Collector
}

// NewHandlers creates new API handlers
func NewHandlers() *Handlers {
	return &Handlers{}
}

// SetPluginService sets the plugin service
func (h *Handlers) SetPluginService(s interface{}) {
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
	// TODO: Implement
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success":true,"data":[]}`))
}

func (h *Handlers) AddPlugin(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"pending"}`))
}

func (h *Handlers) GetPlugin(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid plugin ID", http.StatusBadRequest)
		return
	}
	
	_ = id
	
	// TODO: Implement
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success":true,"data":{}}`))
}

func (h *Handlers) DeletePlugin(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid plugin ID", http.StatusBadRequest)
		return
	}
	
	// TODO: Implement
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) RebuildPlugin(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid plugin ID", http.StatusBadRequest)
		return
	}
	
	// TODO: Implement
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"rebuilding"}`))
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
