package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/epbf-monitoring/epbf-monitor/internal/filter"
	"github.com/epbf-monitoring/epbf-monitor/internal/metrics"
	pg "github.com/epbf-monitoring/epbf-monitor/internal/storage/postgres"
	"github.com/epbf-monitoring/epbf-monitor/internal/plugin"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// Handlers holds API handlers dependencies
type Handlers struct {
	PluginService    *plugin.Service
	MetricsService   *metrics.Service
	MetricsCollector *metrics.Collector
	FilterEngine     *filter.Engine
	FilterRepo       *pg.FilterRepo
	DashboardRepo    *pg.DashboardRepo
	MetricRepo       *pg.MetricRepo

	// WebSocket subscribers
	wsSubscribers map[uuid.UUID]chan []byte
	wsMu          sync.RWMutex
}

// NewHandlers creates new API handlers
func NewHandlers() *Handlers {
	h := &Handlers{
		wsSubscribers: make(map[uuid.UUID]chan []byte),
	}
	go h.cleanupWS()
	return h
}

// SetPluginService sets the plugin service
func (h *Handlers) SetPluginService(s *plugin.Service) {
	h.PluginService = s
}

// SetMetricsService sets the metrics service
func (h *Handlers) SetMetricsService(s *metrics.Service) {
	h.MetricsService = s
}

// SetMetrics sets the metrics collector
func (h *Handlers) SetMetrics(m *metrics.Collector) {
	h.MetricsCollector = m
}

// SetFilterEngine sets the filter engine
func (h *Handlers) SetFilterEngine(e *filter.Engine) {
	h.FilterEngine = e
}

// SetFilterRepo sets the filter repository
func (h *Handlers) SetFilterRepo(r *pg.FilterRepo) {
	h.FilterRepo = r
}

// SetDashboardRepo sets the dashboard repository
func (h *Handlers) SetDashboardRepo(r *pg.DashboardRepo) {
	h.DashboardRepo = r
}

// SetMetricRepo sets the metric repository
func (h *Handlers) SetMetricRepo(r *pg.MetricRepo) {
	h.MetricRepo = r
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

	handler := promhttp.HandlerFor(h.MetricsCollector.Registry(), promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
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

func (h *Handlers) EnablePlugin(w http.ResponseWriter, r *http.Request) {
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

	if err := h.PluginService.EnablePlugin(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "enabled",
	})
}

func (h *Handlers) DisablePlugin(w http.ResponseWriter, r *http.Request) {
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

	if err := h.PluginService.DisablePlugin(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "disabled",
	})
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
	if h.MetricsService == nil {
		http.Error(w, "Metrics service not initialized", http.StatusServiceUnavailable)
		return
	}

	nameFilter := r.URL.Query().Get("name")
	labelFilter := r.URL.Query().Get("label")

	metrics, err := h.MetricsService.GetMetrics(r.Context(), nameFilter, labelFilter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    metrics,
	})
}

func (h *Handlers) GetMetric(w http.ResponseWriter, r *http.Request) {
	if h.MetricsService == nil {
		http.Error(w, "Metrics service not initialized", http.StatusServiceUnavailable)
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Metric name required", http.StatusBadRequest)
		return
	}

	metric, err := h.MetricsService.GetMetricByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if metric == nil {
		http.Error(w, "Metric not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    metric,
	})
}

// Query metrics with filter expression
func (h *Handlers) QueryMetrics(w http.ResponseWriter, r *http.Request) {
	if h.MetricsService == nil {
		http.Error(w, "Metrics service not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "query required", http.StatusBadRequest)
		return
	}

	series, err := h.MetricsService.Query(r.Context(), req.Query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    series,
	})
}

// Filter handlers

func (h *Handlers) ListFilters(w http.ResponseWriter, r *http.Request) {
	if h.FilterRepo == nil {
		http.Error(w, "Filter repository not initialized", http.StatusServiceUnavailable)
		return
	}

	pluginIDStr := r.URL.Query().Get("plugin_id")
	var filters []*pg.Filter
	var err error

	if pluginIDStr != "" {
		pluginID, err := uuid.Parse(pluginIDStr)
		if err != nil {
			http.Error(w, "Invalid plugin_id", http.StatusBadRequest)
			return
		}
		filters, err = h.FilterRepo.GetByPluginID(r.Context(), pluginID)
	} else {
		filters, err = h.FilterRepo.List(r.Context())
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    filters,
	})
}

func (h *Handlers) CreateFilter(w http.ResponseWriter, r *http.Request) {
	if h.FilterRepo == nil {
		http.Error(w, "Filter repository not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		PluginID    string `json:"plugin_id,omitempty"`
		Name        string `json:"name"`
		Expression  string `json:"expression"`
		Description string `json:"description,omitempty"`
		IsDefault   bool   `json:"is_default"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Expression == "" {
		http.Error(w, "name and expression required", http.StatusBadRequest)
		return
	}

	// Validate expression
	if _, err := filter.ParseExpression(req.Expression); err != nil {
		http.Error(w, "Invalid expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	filter := &pg.Filter{
		ID:          uuid.New(),
		Name:        req.Name,
		Expression:  req.Expression,
		Description: req.Description,
		IsDefault:   req.IsDefault,
		CreatedAt:   time.Now(),
	}

	if req.PluginID != "" {
		pluginID, err := uuid.Parse(req.PluginID)
		if err != nil {
			http.Error(w, "Invalid plugin_id", http.StatusBadRequest)
			return
		}
		filter.PluginID = pluginID
	}

	if err := h.FilterRepo.Create(r.Context(), filter); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    filter,
	})
}

func (h *Handlers) GetFilter(w http.ResponseWriter, r *http.Request) {
	if h.FilterRepo == nil {
		http.Error(w, "Filter repository not initialized", http.StatusServiceUnavailable)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid filter ID", http.StatusBadRequest)
		return
	}

	f, err := h.FilterRepo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if f == nil {
		http.Error(w, "Filter not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    f,
	})
}

func (h *Handlers) DeleteFilter(w http.ResponseWriter, r *http.Request) {
	if h.FilterRepo == nil {
		http.Error(w, "Filter repository not initialized", http.StatusServiceUnavailable)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid filter ID", http.StatusBadRequest)
		return
	}

	if err := h.FilterRepo.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ExecuteFilter executes a filter expression
func (h *Handlers) ExecuteFilter(w http.ResponseWriter, r *http.Request) {
	if h.FilterEngine == nil {
		http.Error(w, "Filter engine not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Expression string `json:"expression"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Expression == "" {
		http.Error(w, "expression required", http.StatusBadRequest)
		return
	}

	result, err := h.FilterEngine.Evaluate(r.Context(), req.Expression)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    result,
	})
}

// Dashboard handlers

func (h *Handlers) GetDashboard(w http.ResponseWriter, r *http.Request) {
	if h.DashboardRepo == nil {
		http.Error(w, "Dashboard repository not initialized", http.StatusServiceUnavailable)
		return
	}

	idStr := r.URL.Query().Get("id")
	var dashboard *pg.Dashboard
	var err error

	if idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, "Invalid dashboard ID", http.StatusBadRequest)
			return
		}
		dashboard, err = h.DashboardRepo.GetByID(r.Context(), id)
	} else {
		dashboard, err = h.DashboardRepo.GetDefault(r.Context())
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if dashboard == nil {
		// Return null instead of 404 so UI can create default
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data":    nil,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    dashboard,
	})
}

func (h *Handlers) ListDashboards(w http.ResponseWriter, r *http.Request) {
	if h.DashboardRepo == nil {
		http.Error(w, "Dashboard repository not initialized", http.StatusServiceUnavailable)
		return
	}

	dashboards, err := h.DashboardRepo.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    dashboards,
	})
}

func (h *Handlers) UpdateDashboard(w http.ResponseWriter, r *http.Request) {
	if h.DashboardRepo == nil {
		http.Error(w, "Dashboard repository not initialized", http.StatusServiceUnavailable)
		return
	}

	var req pg.Dashboard
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	req.UpdatedAt = time.Now()

	if err := h.DashboardRepo.Save(r.Context(), &req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    req,
	})
}

func (h *Handlers) CreateDashboard(w http.ResponseWriter, r *http.Request) {
	if h.DashboardRepo == nil {
		http.Error(w, "Dashboard repository not initialized", http.StatusServiceUnavailable)
		return
	}

	var req pg.Dashboard
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	req.ID = uuid.New()
	req.CreatedAt = time.Now()
	req.UpdatedAt = time.Now()

	if err := h.DashboardRepo.Create(r.Context(), &req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    req,
	})
}

func (h *Handlers) DeleteDashboard(w http.ResponseWriter, r *http.Request) {
	if h.DashboardRepo == nil {
		http.Error(w, "Dashboard repository not initialized", http.StatusServiceUnavailable)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid dashboard ID", http.StatusBadRequest)
		return
	}

	if err := h.DashboardRepo.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// WebSocket handler for real-time updates
func (h *Handlers) WebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	subscriberID := uuid.New()
	ch := make(chan []byte, 100)

	h.wsMu.Lock()
	h.wsSubscribers[subscriberID] = ch
	h.wsMu.Unlock()

	defer func() {
		h.wsMu.Lock()
		delete(h.wsSubscribers, subscriberID)
		h.wsMu.Unlock()
		close(ch)
	}()

	// Send initial connection message
	conn.WriteJSON(map[string]interface{}{
		"type":    "connected",
		"id":      subscriberID.String(),
		"message": "Connected to real-time updates",
	})

	// Handle incoming messages
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()

	// Send updates
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

// BroadcastWS broadcasts a message to all WebSocket subscribers
func (h *Handlers) BroadcastWS(msgType string, data interface{}) {
	h.wsMu.RLock()
	defer h.wsMu.RUnlock()

	message, _ := json.Marshal(map[string]interface{}{
		"type": msgType,
		"data": data,
	})

	for _, ch := range h.wsSubscribers {
		select {
		case ch <- message:
		default:
			// Channel full, skip
		}
	}
}

// cleanupWS periodically cleans up disconnected WebSocket subscribers
func (h *Handlers) cleanupWS() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.wsMu.Lock()
		for id, ch := range h.wsSubscribers {
			if len(ch) >= cap(ch) {
				delete(h.wsSubscribers, id)
				close(ch)
			}
		}
		h.wsMu.Unlock()
	}
}

// MetricNames returns available metric names
func (h *Handlers) MetricNames(w http.ResponseWriter, r *http.Request) {
	if h.MetricsService == nil {
		http.Error(w, "Metrics service not initialized", http.StatusServiceUnavailable)
		return
	}

	names := h.MetricsService.GetMetricNames()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    names,
	})
}

// LabelValues returns label values for a metric
func (h *Handlers) LabelValues(w http.ResponseWriter, r *http.Request) {
	if h.MetricsService == nil {
		http.Error(w, "Metrics service not initialized", http.StatusServiceUnavailable)
		return
	}

	metricName := chi.URLParam(r, "metric")
	labelName := r.URL.Query().Get("label")

	if metricName == "" || labelName == "" {
		http.Error(w, "metric and label required", http.StatusBadRequest)
		return
	}

	values := h.MetricsService.GetLabelValues(metricName, labelName)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    values,
	})
}
