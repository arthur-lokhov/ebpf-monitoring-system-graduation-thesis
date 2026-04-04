package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/epbf-monitoring/epbf-monitor/internal/filter"
	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
)

// Service provides metrics management functionality
type Service struct {
	filterEngine *filter.Engine
	collector    *Collector
	mu           sync.RWMutex
	subscribers  []chan []*MetricSample
}

// MetricSample represents a single metric sample
type MetricSample struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
	Timestamp time.Time         `json:"timestamp"`
	PluginID  string            `json:"plugin_id,omitempty"`
}

// NewService creates a new metrics service
func NewService(collector *Collector, filterEngine *filter.Engine) *Service {
	logger.Info("Creating metrics service...")

	service := &Service{
		filterEngine: filterEngine,
		collector:    collector,
		subscribers:  make([]chan []*MetricSample, 0),
	}

	logger.Info("✅ Metrics service created")
	return service
}

// GetMetrics retrieves all metrics with optional filtering
func (s *Service) GetMetrics(ctx context.Context, nameFilter, labelFilter string) ([]*MetricSample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allMetrics := s.filterEngine.GetMetrics()
	
	// Keep only the latest value for each unique name+labels combination
	latest := make(map[string]*MetricSample)
	
	for _, m := range allMetrics {
		// Apply name filter
		if nameFilter != "" && m.Name != nameFilter {
			continue
		}

		// Apply label filter
		if labelFilter != "" {
			match := false
			for k, v := range m.Labels {
				if k == labelFilter || v == labelFilter {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		
		// Create unique key from name + sorted labels
		key := m.Name
		for k, v := range m.Labels {
			key += fmt.Sprintf("|%s=%s", k, v)
		}
		
		// Keep only the latest sample
		if existing, ok := latest[key]; !ok || m.Timestamp.After(existing.Timestamp) {
			latest[key] = &MetricSample{
				Name:      m.Name,
				Value:     m.Value,
				Labels:    m.Labels,
				Timestamp: m.Timestamp,
			}
		}
	}
	
	// Convert map to slice
	result := make([]*MetricSample, 0, len(latest))
	for _, sample := range latest {
		result = append(result, sample)
	}

	return result, nil
}

// GetMetricByName retrieves a specific metric by name
func (s *Service) GetMetricByName(ctx context.Context, name string) (*MetricInfo, error) {
	series := s.filterEngine.GetMetricsByName(name)
	if len(series) == 0 {
		return nil, nil
	}

	// Aggregate information from all series with this name
	labels := make(map[string][]string)
	var latestValue float64
	var latestTime time.Time
	totalPoints := 0

	for _, s := range series {
		for k, v := range s.Labels {
			if !contains(labels[k], v) {
				labels[k] = append(labels[k], v)
			}
		}
		for _, p := range s.Points {
			totalPoints++
			if p.Timestamp.After(latestTime) {
				latestValue = p.Value
				latestTime = p.Timestamp
			}
		}
	}

	labelNames := make([]string, 0, len(labels))
	for k := range labels {
		labelNames = append(labelNames, k)
	}

	return &MetricInfo{
		Name:        name,
		LabelNames:  labelNames,
		LabelValues: labels,
		LatestValue: latestValue,
		LatestTime:  latestTime,
		TotalPoints: totalPoints,
	}, nil
}

// MetricInfo contains detailed information about a metric
type MetricInfo struct {
	Name        string              `json:"name"`
	LabelNames  []string            `json:"label_names"`
	LabelValues map[string][]string `json:"label_values"`
	LatestValue float64             `json:"latest_value"`
	LatestTime  time.Time           `json:"latest_time"`
	TotalPoints int                 `json:"total_points"`
}

// EvaluateFilter evaluates a filter expression
func (s *Service) EvaluateFilter(ctx context.Context, expression string) (*filter.FilterResult, error) {
	return s.filterEngine.Evaluate(ctx, expression)
}

// AddMetric adds a metric sample
func (s *Service) AddMetric(sample *MetricSample) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.filterEngine.AddMetric(&filter.MetricValue{
		Name:      sample.Name,
		Value:     sample.Value,
		Labels:    sample.Labels,
		Timestamp: sample.Timestamp,
	})

	// Notify subscribers
	s.notifySubscribers(sample)
}

// AddMetrics adds multiple metric samples
func (s *Service) AddMetrics(samples []*MetricSample) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := make([]*filter.MetricValue, len(samples))
	for i, s := range samples {
		metrics[i] = &filter.MetricValue{
			Name:      s.Name,
			Value:     s.Value,
			Labels:    s.Labels,
			Timestamp: s.Timestamp,
		}
	}

	s.filterEngine.AddMetrics(metrics)

	// Notify subscribers
	s.notifySubscribers(samples...)
}

// Subscribe creates a new subscription for metric updates
func (s *Service) Subscribe() chan []*MetricSample {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan []*MetricSample, 100)
	s.subscribers = append(s.subscribers, ch)
	return ch
}

// Unsubscribe removes a subscription
func (s *Service) Unsubscribe(ch chan []*MetricSample) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, sub := range s.subscribers {
		if sub == ch {
			s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// notifySubscribers notifies all subscribers of new metrics
func (s *Service) notifySubscribers(samples ...*MetricSample) {
	if len(s.subscribers) == 0 {
		return
	}

	for _, sub := range s.subscribers {
		select {
		case sub <- samples:
		default:
			// Channel full, skip
		}
	}
}

// GetMetricNames returns all unique metric names
func (s *Service) GetMetricNames() []string {
	return s.filterEngine.GetMetricNames()
}

// GetLabelValues returns all values for a label
func (s *Service) GetLabelValues(metricName, labelName string) []string {
	return s.filterEngine.GetLabelValues(metricName, labelName)
}

// Query executes a PromQL-like query
func (s *Service) Query(ctx context.Context, query string) ([]*MetricSeries, error) {
	result, err := s.filterEngine.Evaluate(ctx, query)
	if err != nil {
		return nil, err
	}

	series := make([]*MetricSeries, len(result.Series))
	for i, s := range result.Series {
		points := make([]MetricPoint, len(s.Points))
		for j, p := range s.Points {
			points[j] = MetricPoint{
				Value:     p.Value,
				Timestamp: p.Timestamp,
			}
		}
		series[i] = &MetricSeries{
			Name:   s.Name,
			Labels: s.Labels,
			Points: points,
		}
	}

	return series, nil
}

// MetricSeries represents a time series
type MetricSeries struct {
	Name   string       `json:"name"`
	Labels map[string]string `json:"labels"`
	Points []MetricPoint `json:"points"`
}

// MetricPoint represents a single point in a time series
type MetricPoint struct {
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

// contains checks if a slice contains a value
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
