package filter

import (
	"context"
	"sync"
	"time"

	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
)

// Engine is the main filter engine that evaluates filter expressions
type Engine struct {
	store      *MetricStore
	mu         sync.RWMutex
	cache      map[string]*FilterResult
	cacheExpiry map[string]time.Time
	cacheTTL    time.Duration
}

// NewEngine creates a new filter engine
func NewEngine() *Engine {
	logger.Info("Creating filter engine...")

	engine := &Engine{
		store:       NewMetricStore(),
		cache:       make(map[string]*FilterResult),
		cacheExpiry: make(map[string]time.Time),
		cacheTTL:    30 * time.Second,
	}

	logger.Info("✅ Filter engine created")
	return engine
}

// AddMetric adds a metric value to the store
func (e *Engine) AddMetric(metric *MetricValue) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.store.AddMetric(metric)
}

// AddMetrics adds multiple metric values
func (e *Engine) AddMetrics(metrics []*MetricValue) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, m := range metrics {
		e.store.AddMetric(m)
	}
}

// Evaluate evaluates a filter expression
func (e *Engine) Evaluate(ctx context.Context, expression string) (*FilterResult, error) {
	e.mu.RLock()
	
	// Check cache
	if result, ok := e.getFromCache(expression); ok {
		e.mu.RUnlock()
		return result, nil
	}
	e.mu.RUnlock()

	// Parse expression
	ast, err := ParseExpression(expression)
	if err != nil {
		return nil, err
	}

	// Create evaluation context
	evalCtx := &EvaluationContext{
		StartTime:    time.Now().Add(-5 * time.Minute),
		EndTime:      time.Now(),
		Step:         15 * time.Second,
		Lookback:     5 * time.Minute,
		MetricStore:  e.store,
		VariableBindings: make(map[string]*FilterResult),
	}

	// Evaluate AST
	e.mu.RLock()
	result, err := ast.Evaluate(evalCtx, nil)
	e.mu.RUnlock()

	if err != nil {
		return nil, err
	}

	// Cache result
	e.addToCache(expression, result)

	return result, nil
}

// EvaluateWithMetrics evaluates an expression with provided metrics
func (e *Engine) EvaluateWithMetrics(ctx context.Context, expression string, metrics []*MetricValue) (*FilterResult, error) {
	// Parse expression
	ast, err := ParseExpression(expression)
	if err != nil {
		return nil, err
	}

	// Create evaluation context
	evalCtx := &EvaluationContext{
		StartTime:    time.Now().Add(-5 * time.Minute),
		EndTime:      time.Now(),
		Step:         15 * time.Second,
		Lookback:     5 * time.Minute,
		MetricStore:  nil, // Use provided metrics instead
		VariableBindings: make(map[string]*FilterResult),
	}

	// Evaluate AST
	result, err := ast.Evaluate(evalCtx, metrics)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetMetrics retrieves all stored metrics
func (e *Engine) GetMetrics() []*MetricValue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.store.GetAllMetrics()
}

// GetMetricsByName retrieves metrics by name
func (e *Engine) GetMetricsByName(name string) []*MetricSeries {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.store.GetMetrics(name)
}

// ClearCache clears the result cache
func (e *Engine) ClearCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache = make(map[string]*FilterResult)
	e.cacheExpiry = make(map[string]time.Time)
}

// SetCacheTTL sets the cache TTL
func (e *Engine) SetCacheTTL(ttl time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cacheTTL = ttl
}

// getFromCache retrieves a cached result if valid
func (e *Engine) getFromCache(key string) (*FilterResult, bool) {
	result, ok := e.cache[key]
	if !ok {
		return nil, false
	}

	if expiry, ok := e.cacheExpiry[key]; ok && time.Now().After(expiry) {
		// Cache expired
		delete(e.cache, key)
		delete(e.cacheExpiry, key)
		return nil, false
	}

	return result, true
}

// addToCache adds a result to the cache
func (e *Engine) addToCache(key string, result *FilterResult) {
	e.cache[key] = result
	e.cacheExpiry[key] = time.Now().Add(e.cacheTTL)
}

// Cleanup removes expired cache entries
func (e *Engine) Cleanup() {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	for key, expiry := range e.cacheExpiry {
		if now.After(expiry) {
			delete(e.cache, key)
			delete(e.cacheExpiry, key)
		}
	}
}

// StartCleanupRoutine starts a background routine to clean up expired cache entries
func (e *Engine) StartCleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.Cleanup()
			}
		}
	}()
}

// GetMetricNames returns all unique metric names
func (e *Engine) GetMetricNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	names := make(map[string]bool)
	for name := range e.store.metrics {
		names[name] = true
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	return result
}

// GetMetricLabels returns all label keys for a metric
func (e *Engine) GetMetricLabels(metricName string) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	labelSet := make(map[string]bool)
	if labelMap, ok := e.store.metrics[metricName]; ok {
		for _, series := range labelMap {
			for label := range series.Labels {
				labelSet[label] = true
			}
		}
	}

	labels := make([]string, 0, len(labelSet))
	for label := range labelSet {
		labels = append(labels, label)
	}
	return labels
}

// GetLabelValues returns all values for a specific label
func (e *Engine) GetLabelValues(metricName, labelName string) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	valueSet := make(map[string]bool)
	if labelMap, ok := e.store.metrics[metricName]; ok {
		for _, series := range labelMap {
			if value, ok := series.Labels[labelName]; ok {
				valueSet[value] = true
			}
		}
	}

	values := make([]string, 0, len(valueSet))
	for value := range valueSet {
		values = append(values, value)
	}
	return values
}
