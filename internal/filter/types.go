package filter

import (
	"time"
)

// MetricValue represents a single metric value with labels
type MetricValue struct {
	Name      string
	Value     float64
	Labels    map[string]string
	Timestamp time.Time
}

// MetricSeries represents a time series of metric values
type MetricSeries struct {
	Name   string
	Labels map[string]string
	Points []MetricPoint
}

// MetricPoint represents a single point in a time series
type MetricPoint struct {
	Value     float64
	Timestamp time.Time
}

// FilterResult represents the result of applying a filter
type FilterResult struct {
	Series []*MetricSeries
}

// ASTNode represents a node in the filter expression AST
type ASTNode interface {
	Evaluate(ctx *EvaluationContext, metrics []*MetricValue) (*FilterResult, error)
	String() string
}

// EvaluationContext holds the state during filter evaluation
type EvaluationContext struct {
	StartTime    time.Time
	EndTime      time.Time
	Step         time.Duration
	Lookback     time.Duration
	MetricStore  *MetricStore
	VariableBindings map[string]*FilterResult
}

// MetricStore stores metric values for querying
type MetricStore struct {
	metrics map[string]map[uint64]*MetricSeries // name -> labelHash -> series
}

// NewMetricStore creates a new metric store
func NewMetricStore() *MetricStore {
	return &MetricStore{
		metrics: make(map[string]map[uint64]*MetricSeries),
	}
}

// AddMetric adds a metric value to the store
func (m *MetricStore) AddMetric(metric *MetricValue) {
	if _, ok := m.metrics[metric.Name]; !ok {
		m.metrics[metric.Name] = make(map[uint64]*MetricSeries)
	}

	labelHash := hashLabels(metric.Labels)
	if _, ok := m.metrics[metric.Name][labelHash]; !ok {
		m.metrics[metric.Name][labelHash] = &MetricSeries{
			Name:   metric.Name,
			Labels: metric.Labels,
			Points: make([]MetricPoint, 0, 1), // Pre-allocate for 1 point
		}
	}

	series := m.metrics[metric.Name][labelHash]
	
	// Keep only the latest point (replace instead of append)
	if len(series.Points) > 0 {
		series.Points[0] = MetricPoint{
			Value:     metric.Value,
			Timestamp: metric.Timestamp,
		}
	} else {
		series.Points = append(series.Points, MetricPoint{
			Value:     metric.Value,
			Timestamp: metric.Timestamp,
		})
	}
}

// GetMetrics retrieves metrics by name
func (m *MetricStore) GetMetrics(name string) []*MetricSeries {
	if nameMap, ok := m.metrics[name]; ok {
		series := make([]*MetricSeries, 0, len(nameMap))
		for _, s := range nameMap {
			series = append(series, s)
		}
		return series
	}
	return nil
}

// GetAllMetrics returns all metrics
func (m *MetricStore) GetAllMetrics() []*MetricValue {
	var result []*MetricValue
	for name, labelMap := range m.metrics {
		for _, series := range labelMap {
			// Return only the latest point for each series
			if len(series.Points) > 0 {
				latest := series.Points[len(series.Points)-1]
				result = append(result, &MetricValue{
					Name:      name,
					Value:     latest.Value,
					Labels:    series.Labels,
					Timestamp: latest.Timestamp,
				})
			}
		}
	}
	return result
}

// hashLabels creates a hash of label map for grouping
func hashLabels(labels map[string]string) uint64 {
	// Simple hash implementation
	var hash uint64 = 0
	for k, v := range labels {
		for _, c := range k {
			hash = hash*31 + uint64(c)
		}
		hash = hash*31 + ':'
		for _, c := range v {
			hash = hash*31 + uint64(c)
		}
	}
	return hash
}
