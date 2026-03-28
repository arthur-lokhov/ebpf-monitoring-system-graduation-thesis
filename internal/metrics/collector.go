package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/google/uuid"
)

// MetricType represents the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// Metric represents a single metric
type Metric struct {
	ID        uuid.UUID
	Name      string
	Type      MetricType
	Help      string
	Labels    []string
	Value     float64
	Timestamp time.Time
	PluginID  uuid.UUID
}

// Collector collects and manages metrics
type Collector struct {
	metrics   map[string]*Metric
	histograms map[string]*Histogram
	mu        sync.RWMutex
}

// Histogram represents a histogram metric
type Histogram struct {
	Name   string
	Help   string
	Labels []string
	Buckets map[float64]uint64
	Sum    float64
	Count  uint64
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	logger.Info("Creating metrics collector...")
	
	return &Collector{
		metrics:    make(map[string]*Metric),
		histograms: make(map[string]*Histogram),
	}
}

// IncCounter increments a counter metric
func (c *Collector) IncCounter(pluginID uuid.UUID, name string, labels map[string]string, value float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	key := makeKey(name, labels)
	
	if metric, ok := c.metrics[key]; ok {
		metric.Value += value
		metric.Timestamp = time.Now()
	} else {
		c.metrics[key] = &Metric{
			ID:        uuid.New(),
			Name:      name,
			Type:      MetricTypeCounter,
			Labels:    mapToSlice(labels),
			Value:     value,
			Timestamp: time.Now(),
			PluginID:  pluginID,
		}
	}
	
	logger.Debug("Counter incremented",
		"name", name,
		"value", value,
		"labels", labels,
		"plugin_id", pluginID.String())
}

// SetGauge sets a gauge metric
func (c *Collector) SetGauge(pluginID uuid.UUID, name string, labels map[string]string, value float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	key := makeKey(name, labels)
	
	c.metrics[key] = &Metric{
		ID:        uuid.New(),
		Name:      name,
		Type:      MetricTypeGauge,
		Labels:    mapToSlice(labels),
		Value:     value,
		Timestamp: time.Now(),
		PluginID:  pluginID,
	}
	
	logger.Debug("Gauge set",
		"name", name,
		"value", value,
		"labels", labels,
		"plugin_id", pluginID.String())
}

// ObserveHistogram observes a value in a histogram
func (c *Collector) ObserveHistogram(pluginID uuid.UUID, name string, labels map[string]string, value float64, buckets []float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	key := makeKey(name, labels)
	
	hist, ok := c.histograms[key]
	if !ok {
		hist = &Histogram{
			Name:    name,
			Help:    "",
			Labels:  mapToSlice(labels),
			Buckets: make(map[float64]uint64),
		}
		for _, b := range buckets {
			hist.Buckets[b] = 0
		}
		c.histograms[key] = hist
	}
	
	hist.Sum += value
	hist.Count++
	
	for _, b := range buckets {
		if value <= b {
			hist.Buckets[b]++
		}
	}
	
	logger.Debug("Histogram observed",
		"name", name,
		"value", value,
		"labels", labels,
		"plugin_id", pluginID.String())
}

// GetMetrics returns all metrics
func (c *Collector) GetMetrics() []*Metric {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	metrics := make([]*Metric, 0, len(c.metrics))
	for _, m := range c.metrics {
		metrics = append(metrics, m)
	}
	
	return metrics
}

// WritePrometheus writes metrics in Prometheus format
func (c *Collector) WritePrometheus(w io.Writer) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Group metrics by name
	metricsByName := make(map[string][]*Metric)
	for _, m := range c.metrics {
		metricsByName[m.Name] = append(metricsByName[m.Name], m)
	}
	
	// Sort metric names for consistent output
	names := make([]string, 0, len(metricsByName))
	for name := range metricsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	
	// Write metrics
	for _, name := range names {
		metrics := metricsByName[name]
		if len(metrics) == 0 {
			continue
		}
		
		metric := metrics[0]
		
		// Write HELP
		if metric.Help != "" {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", name, metric.Help); err != nil {
				return err
			}
		}
		
		// Write TYPE
		if _, err := fmt.Fprintf(w, "# TYPE %s %s\n", name, metric.Type); err != nil {
			return err
		}
		
		// Write values
		for _, m := range metrics {
			labelsStr := formatLabels(m.Labels)
			if _, err := fmt.Fprintf(w, "%s%s %g\n", name, labelsStr, m.Value); err != nil {
				return err
			}
		}
	}
	
	// Write histograms
	for key, hist := range c.histograms {
		// Write HELP
		if hist.Help != "" {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", hist.Name, hist.Help); err != nil {
				return err
			}
		}
		
		// Write TYPE
		if _, err := fmt.Fprintf(w, "# TYPE %s histogram\n", hist.Name); err != nil {
			return err
		}
		
		// Write buckets
		labelsStr := formatLabels(hist.Labels)
		for bucket, count := range hist.Buckets {
			bucketLabels := mergeLabels(labelsStr, fmt.Sprintf(`le="%g"`, bucket))
			if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", hist.Name, bucketLabels, count); err != nil {
				return err
			}
		}
		
		// Write +Inf bucket
		infLabels := mergeLabels(labelsStr, `le="+Inf"`)
		if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", hist.Name, infLabels, hist.Count); err != nil {
			return err
		}
		
		// Write sum
		sumLabels := mergeLabels(labelsStr, "")
		if _, err := fmt.Fprintf(w, "%s_sum%s %g\n", hist.Name, sumLabels, hist.Sum); err != nil {
			return err
		}
		
		// Write count
		if _, err := fmt.Fprintf(w, "%s_count%s %d\n", hist.Name, sumLabels, hist.Count); err != nil {
			return err
		}
		
		_ = key // unused
	}
	
	return nil
}

// makeKey creates a unique key for a metric
func makeKey(name string, labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(labels[k])
	}
	sb.WriteString("}")
	
	return sb.String()
}

// formatLabels formats labels for Prometheus output
func formatLabels(labelSlice []string) string {
	if len(labelSlice) == 0 {
		return ""
	}
	
	labels := make(map[string]string)
	for _, label := range labelSlice {
		parts := strings.SplitN(label, "=", 2)
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		}
	}
	
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	var sb strings.Builder
	sb.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(k)
		sb.WriteString("=\"")
		sb.WriteString(labels[k])
		sb.WriteString("\"")
	}
	sb.WriteString("}")
	
	return sb.String()
}

// mergeLabels merges two label strings
func mergeLabels(base, extra string) string {
	if base == "" {
		return "{" + extra + "}"
	}
	if extra == "" {
		return base
	}
	
	// Remove trailing } from base and leading { from extra
	base = strings.TrimSuffix(base, "}")
	extra = strings.TrimPrefix(extra, "{")
	
	return base + "," + extra + "}"
}

// mapToSlice converts a map to a slice of "key=value" strings
func mapToSlice(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

// RemovePluginMetrics removes all metrics for a plugin
func (c *Collector) RemovePluginMetrics(pluginID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	count := 0
	for key, metric := range c.metrics {
		if metric.PluginID == pluginID {
			delete(c.metrics, key)
			count++
		}
	}
	
	for key, hist := range c.histograms {
		// Check if any label contains plugin ID
		for _, label := range hist.Labels {
			if strings.Contains(label, pluginID.String()) {
				delete(c.histograms, key)
				count++
				break
			}
		}
	}
	
	logger.Info("Removed plugin metrics",
		"plugin_id", pluginID.String(),
		"count", count)
}
