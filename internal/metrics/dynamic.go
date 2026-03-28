package metrics

import (
	"fmt"
	"sync"

	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// PluginMetrics holds all metrics for a plugin
type PluginMetrics struct {
	Name     string
	Manifest map[string]any
	Collectors map[string]prometheus.Collector
}

// DynamicMetrics manages dynamic metrics from plugins
type DynamicMetrics struct {
	registry      *prometheus.Registry
	pluginMetrics map[string]*PluginMetrics
	mu            sync.RWMutex
}

// NewDynamicMetrics creates a new dynamic metrics manager
func NewDynamicMetrics(registry *prometheus.Registry) *DynamicMetrics {
	logger.Info("Creating dynamic metrics manager...")
	
	return &DynamicMetrics{
		registry:      registry,
		pluginMetrics: make(map[string]*PluginMetrics),
	}
}

// RegisterPluginMetrics registers metrics from a plugin manifest
func (d *DynamicMetrics) RegisterPluginMetrics(pluginName, version string, manifest map[string]any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	logger.Info("Registering plugin metrics",
		"plugin", pluginName,
		"version", version,
		"manifest_keys", getMapKeys(manifest))
	
	// Get metrics from manifest
	metricsRaw, ok := manifest["metrics"]
	if !ok {
		logger.Warn("No 'metrics' key found in manifest", 
			"plugin", pluginName,
			"available_keys", getMapKeys(manifest))
		return nil
	}
	
	logger.Debug("Found metrics in manifest",
		"plugin", pluginName,
		"metrics_type", fmt.Sprintf("%T", metricsRaw),
		"metrics_value", fmt.Sprintf("%+v", metricsRaw))
	
	// Convert to slice of maps
	var metricsList []map[string]any
	switch v := metricsRaw.(type) {
	case []any:
		logger.Debug("Processing metrics as []any",
			"plugin", pluginName,
			"count", len(v))
		for i, m := range v {
			logger.Debug("Processing metric",
				"plugin", pluginName,
				"index", i,
				"type", fmt.Sprintf("%T", m),
				"value", fmt.Sprintf("%+v", m))
			if mMap, ok := m.(map[string]any); ok {
				metricsList = append(metricsList, mMap)
			} else {
				logger.Warn("Metric item is not a map",
					"plugin", pluginName,
					"index", i,
					"type", fmt.Sprintf("%T", m))
			}
		}
	case []map[string]any:
		logger.Debug("Processing metrics as []map[string]any",
			"plugin", pluginName,
			"count", len(v))
		metricsList = v
	default:
		logger.Error("Invalid metrics format in manifest", 
			"plugin", pluginName, 
			"type", fmt.Sprintf("%T", metricsRaw),
			"value", fmt.Sprintf("%+v", metricsRaw))
		return fmt.Errorf("invalid metrics format: %T", metricsRaw)
	}
	
	if len(metricsList) == 0 {
		logger.Warn("No valid metrics found in manifest", 
			"plugin", pluginName,
			"metrics_list", fmt.Sprintf("%+v", metricsList))
		return nil
	}
	
	logger.Info("Processing metrics",
		"plugin", pluginName,
		"count", len(metricsList))
	
	pm := &PluginMetrics{
		Name:       pluginName,
		Manifest:   manifest,
		Collectors: make(map[string]prometheus.Collector),
	}
	
	for i, metricMap := range metricsList {
		logger.Debug("Processing metric map",
			"plugin", pluginName,
			"index", i,
			"map", fmt.Sprintf("%+v", metricMap))
		
		name, ok := metricMap["name"].(string)
		if !ok {
			logger.Warn("Metric missing 'name' field", 
				"plugin", pluginName, 
				"index", i,
				"name_field", fmt.Sprintf("%T", metricMap["name"]))
			continue
		}
		
		metricType, ok := metricMap["type"].(string)
		if !ok {
			logger.Warn("Metric missing 'type' field", 
				"plugin", pluginName, 
				"name", name,
				"type_field", fmt.Sprintf("%T", metricMap["type"]))
			continue
		}
		
		help, _ := metricMap["help"].(string)
		
		// Get labels - handle both []any and []string
		var labels []string
		if labelsRaw, ok := metricMap["labels"].([]any); ok {
			logger.Debug("Processing labels as []any",
				"plugin", pluginName,
				"name", name,
				"labels_raw", labelsRaw)
			for _, l := range labelsRaw {
				if labelStr, ok := l.(string); ok {
					labels = append(labels, labelStr)
				}
			}
		} else if labelsRaw, ok := metricMap["labels"].([]string); ok {
			logger.Debug("Processing labels as []string",
				"plugin", pluginName,
				"name", name,
				"labels", labelsRaw)
			labels = labelsRaw
		} else {
			logger.Debug("Labels not found or wrong type",
				"plugin", pluginName,
				"name", name,
				"labels_type", fmt.Sprintf("%T", metricMap["labels"]))
		}
		
		// Create full metric name with plugin prefix
		fullName := fmt.Sprintf("epbf_plugin_%s_%s", pluginName, name)
		
		// Create collector based on type
		var collector prometheus.Collector
		
		switch metricType {
		case "counter":
			collector = prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: fullName,
					Help: help,
				},
				labels,
			)
			logger.Info("Registered counter",
				"plugin", pluginName,
				"name", fullName,
				"labels", labels)
			
		case "gauge":
			collector = prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: fullName,
					Help: help,
				},
				labels,
			)
			logger.Info("Registered gauge",
				"plugin", pluginName,
				"name", fullName,
				"labels", labels)
			
		case "histogram":
			collector = prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name: fullName,
					Help: help,
				},
				labels,
			)
			logger.Info("Registered histogram",
				"plugin", pluginName,
				"name", fullName,
				"labels", labels)
			
		default:
			logger.Warn("Unknown metric type",
				"plugin", pluginName,
				"name", name,
				"type", metricType)
			continue
		}
		
		// Register collector
		if err := d.registry.Register(collector); err != nil {
			logger.Error("Failed to register metric",
				"plugin", pluginName,
				"name", fullName,
				"error", err.Error())
			continue
		}
		
		// Initialize metric with zero value (so it appears in /metrics)
		switch c := collector.(type) {
		case *prometheus.CounterVec:
			// Create empty labels map for initialization
			if len(labels) == 0 {
				c.WithLabelValues().Add(0)
			}
		case *prometheus.GaugeVec:
			if len(labels) == 0 {
				c.WithLabelValues().Set(0)
			}
		}
		
		pm.Collectors[name] = collector
	}
	
	d.pluginMetrics[pluginName] = pm
	
	// Debug: list all registered collectors
	mfs, err := d.registry.Gather()
	if err != nil {
		logger.Error("Failed to gather metrics", "error", err.Error())
	} else {
		for _, mf := range mfs {
			if mf.GetName() != "" && len(mf.GetMetric()) > 0 {
				logger.Debug("Registry contains metric",
					"name", mf.GetName(),
					"count", len(mf.GetMetric()))
			}
		}
	}
	
	logger.Info("✅ Plugin metrics registered",
		"plugin", pluginName,
		"count", len(pm.Collectors),
		"registered_metrics", getCollectorNames(pm.Collectors))
	
	return nil
}

// Helper functions for debugging
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func getCollectorNames(collectors map[string]prometheus.Collector) []string {
	names := make([]string, 0, len(collectors))
	for name := range collectors {
		names = append(names, name)
	}
	return names
}

// GetCounter returns a counter metric for a plugin
func (d *DynamicMetrics) GetCounter(pluginName, metricName string, labels map[string]string) (prometheus.Counter, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	pm, ok := d.pluginMetrics[pluginName]
	if !ok {
		return nil, fmt.Errorf("plugin metrics not found: %s", pluginName)
	}
	
	collector, ok := pm.Collectors[metricName]
	if !ok {
		return nil, fmt.Errorf("metric not found: %s.%s", pluginName, metricName)
	}
	
	counterVec, ok := collector.(*prometheus.CounterVec)
	if !ok {
		return nil, fmt.Errorf("metric %s.%s is not a counter", pluginName, metricName)
	}
	
	return counterVec.With(labels), nil
}

// GetGauge returns a gauge metric for a plugin
func (d *DynamicMetrics) GetGauge(pluginName, metricName string, labels map[string]string) (prometheus.Gauge, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	pm, ok := d.pluginMetrics[pluginName]
	if !ok {
		return nil, fmt.Errorf("plugin metrics not found: %s", pluginName)
	}
	
	collector, ok := pm.Collectors[metricName]
	if !ok {
		return nil, fmt.Errorf("metric not found: %s.%s", pluginName, metricName)
	}
	
	gaugeVec, ok := collector.(*prometheus.GaugeVec)
	if !ok {
		return nil, fmt.Errorf("metric %s.%s is not a gauge", pluginName, metricName)
	}
	
	return gaugeVec.With(labels), nil
}

// UnregisterPluginMetrics unregisters all metrics for a plugin
func (d *DynamicMetrics) UnregisterPluginMetrics(pluginName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	pm, ok := d.pluginMetrics[pluginName]
	if !ok {
		return nil // Already unregistered
	}
	
	count := 0
	for name, collector := range pm.Collectors {
		if d.registry.Unregister(collector) {
			count++
			logger.Debug("Unregistered metric",
				"plugin", pluginName,
				"name", name)
		}
	}
	
	delete(d.pluginMetrics, pluginName)
	
	logger.Info("✅ Plugin metrics unregistered",
		"plugin", pluginName,
		"count", count)
	
	return nil
}

// ListPluginMetrics returns all registered metrics for a plugin
func (d *DynamicMetrics) ListPluginMetrics(pluginName string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	pm, ok := d.pluginMetrics[pluginName]
	if !ok {
		return nil
	}
	
	names := make([]string, 0, len(pm.Collectors))
	for name := range pm.Collectors {
		names = append(names, name)
	}
	
	return names
}
