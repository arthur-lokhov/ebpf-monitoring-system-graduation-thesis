package metrics

import (
	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// Collector wraps Prometheus registry and collectors
type Collector struct {
	registry *prometheus.Registry
	
	// Plugin metrics
	pluginBuildsTotal   *prometheus.CounterVec
	pluginBuildDuration *prometheus.HistogramVec
	pluginStatus        *prometheus.GaugeVec
	
	// eBPF metrics
	ebpfProgramsLoaded  *prometheus.GaugeVec
	ebpfEventsReceived  *prometheus.CounterVec
	
	// WASM metrics
	wasmInstancesActive *prometheus.GaugeVec
	wasmEventsProcessed *prometheus.CounterVec
}

// NewCollector creates a new Prometheus metrics collector
func NewCollector() *Collector {
	logger.Info("Creating Prometheus metrics collector...")
	
	registry := prometheus.NewRegistry()
	
	c := &Collector{
		registry: registry,
	}
	
	// Register default collectors (Go runtime, process)
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	
	// Define custom metrics
	c.defineMetrics()
	
	// Register custom metrics
	registry.MustRegister(
		c.pluginBuildsTotal,
		c.pluginBuildDuration,
		c.pluginStatus,
		c.ebpfProgramsLoaded,
		c.ebpfEventsReceived,
		c.wasmInstancesActive,
		c.wasmEventsProcessed,
	)
	
	logger.Info("✅ Prometheus metrics collector created")
	
	return c
}

// defineMetrics defines all custom metrics
func (c *Collector) defineMetrics() {
	// Plugin builds
	c.pluginBuildsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "epbf",
			Subsystem: "plugin",
			Name:      "builds_total",
			Help:      "Total number of plugin builds",
		},
		[]string{"plugin", "status"}, // status: success, failure
	)
	
	c.pluginBuildDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "epbf",
			Subsystem: "plugin",
			Name:      "build_duration_seconds",
			Help:      "Plugin build duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"plugin"},
	)
	
	c.pluginStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "epbf",
			Subsystem: "plugin",
			Name:      "status",
			Help:      "Current status of plugins (1=ready, 0=other)",
		},
		[]string{"plugin", "version"},
	)
	
	// eBPF metrics
	c.ebpfProgramsLoaded = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "epbf",
			Subsystem: "ebpf",
			Name:      "programs_loaded",
			Help:      "Number of eBPF programs currently loaded",
		},
		[]string{"plugin"},
	)
	
	c.ebpfEventsReceived = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "epbf",
			Subsystem: "ebpf",
			Name:      "events_received_total",
			Help:      "Total number of eBPF events received",
		},
		[]string{"plugin", "event_type"},
	)
	
	// WASM metrics
	c.wasmInstancesActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "epbf",
			Subsystem: "wasm",
			Name:      "instances_active",
			Help:      "Number of active WASM instances",
		},
		[]string{"plugin"},
	)
	
	c.wasmEventsProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "epbf",
			Subsystem: "wasm",
			Name:      "events_processed_total",
			Help:      "Total number of WASM events processed",
		},
		[]string{"plugin"},
	)
}

// PluginBuildStarted records a plugin build start
func (c *Collector) PluginBuildStarted(plugin string) {
	logger.Debug("Metric: plugin build started", "plugin", plugin)
}

// PluginBuildSuccess records a successful plugin build
func (c *Collector) PluginBuildSuccess(plugin, version string, duration float64) {
	c.pluginBuildsTotal.WithLabelValues(plugin, "success").Inc()
	c.pluginBuildDuration.WithLabelValues(plugin).Observe(duration)
	c.pluginStatus.WithLabelValues(plugin, version).Set(1)
	
	logger.Info("Metric: plugin build success",
		"plugin", plugin,
		"version", version,
		"duration", duration)
}

// PluginBuildFailure records a failed plugin build
func (c *Collector) PluginBuildFailure(plugin string, duration float64) {
	c.pluginBuildsTotal.WithLabelValues(plugin, "failure").Inc()
	c.pluginBuildDuration.WithLabelValues(plugin).Observe(duration)
	c.pluginStatus.WithLabelValues(plugin, "failed").Set(0)
	
	logger.Warn("Metric: plugin build failure",
		"plugin", plugin,
		"duration", duration)
}

// EBPFProgramLoaded records an eBPF program loaded
func (c *Collector) EBPFProgramLoaded(plugin string) {
	c.ebpfProgramsLoaded.WithLabelValues(plugin).Inc()
	logger.Debug("Metric: eBPF program loaded", "plugin", plugin)
}

// EBPFProgramUnloaded records an eBPF program unloaded
func (c *Collector) EBPFProgramUnloaded(plugin string) {
	c.ebpfProgramsLoaded.WithLabelValues(plugin).Dec()
	logger.Debug("Metric: eBPF program unloaded", "plugin", plugin)
}

// EBPFEventReceived records an eBPF event received
func (c *Collector) EBPFEventReceived(plugin, eventType string) {
	c.ebpfEventsReceived.WithLabelValues(plugin, eventType).Inc()
	logger.Debug("Metric: eBPF event received",
		"plugin", plugin,
		"event_type", eventType)
}

// WASMInstanceStarted records a WASM instance started
func (c *Collector) WASMInstanceStarted(plugin string) {
	c.wasmInstancesActive.WithLabelValues(plugin).Inc()
	logger.Debug("Metric: WASM instance started", "plugin", plugin)
}

// WASMInstanceStopped records a WASM instance stopped
func (c *Collector) WASMInstanceStopped(plugin string) {
	c.wasmInstancesActive.WithLabelValues(plugin).Dec()
	logger.Debug("Metric: WASM instance stopped", "plugin", plugin)
}

// WASMEventProcessed records a WASM event processed
func (c *Collector) WASMEventProcessed(plugin string) {
	c.wasmEventsProcessed.WithLabelValues(plugin).Inc()
	logger.Debug("Metric: WASM event processed", "plugin", plugin)
}

// Registry returns the Prometheus registry
func (c *Collector) Registry() *prometheus.Registry {
	return c.registry
}

// RemovePluginMetrics removes all metrics for a plugin
func (c *Collector) RemovePluginMetrics(plugin string) {
	labels := prometheus.Labels{"plugin": plugin}
	
	c.pluginBuildsTotal.DeletePartialMatch(labels)
	c.pluginBuildDuration.DeletePartialMatch(labels)
	c.pluginStatus.DeletePartialMatch(labels)
	c.ebpfProgramsLoaded.DeletePartialMatch(labels)
	c.ebpfEventsReceived.DeletePartialMatch(labels)
	c.wasmInstancesActive.DeletePartialMatch(labels)
	c.wasmEventsProcessed.DeletePartialMatch(labels)
	
	logger.Info("Removed plugin metrics", "plugin", plugin)
}
