package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytecodealliance/wasmtime-go/v21"
	"github.com/epbf-monitoring/epbf-monitor/internal/filter"
	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/google/uuid"
)

// Instance represents a running WASM plugin instance
type Instance struct {
	ID        uuid.UUID
	Name      string
	Store     *wasmtime.Store
	Module    *wasmtime.Module
	Instance  *wasmtime.Instance
	Functions *Functions
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
}

// Functions holds exported WASM functions
type Functions struct {
	Init          *wasmtime.Func
	Cleanup       *wasmtime.Func
	ProcessEvents *wasmtime.Func
}

// Engine manages WASM runtime
type Engine struct {
	engine      *wasmtime.Engine
	instances   map[uuid.UUID]*Instance
	mu          sync.RWMutex
	metricStore *filter.MetricStore
	metricMu    sync.RWMutex
}

// NewEngine creates a new WASM engine
func NewEngine() *Engine {
	logger.Info("Creating WASM engine...")

	config := wasmtime.NewConfig()
	config.SetEpochInterruption(true)

	engine := wasmtime.NewEngineWithConfig(config)

	logger.Info("✅ WASM engine created")

	return &Engine{
		engine:      engine,
		instances:   make(map[uuid.UUID]*Instance),
		metricStore: filter.NewMetricStore(),
	}
}

// LoadPlugin loads a WASM module and creates an instance
func (e *Engine) LoadPlugin(ctx context.Context, pluginID uuid.UUID, name string, wasmBytes []byte) (*Instance, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	logger.Info("Loading WASM plugin", "plugin_id", pluginID.String(), "name", name, "size", len(wasmBytes))

	// Create store
	store := wasmtime.NewStore(e.engine)
	store.SetEpochDeadline(1)

	// Create module
	module, err := wasmtime.NewModule(e.engine, wasmBytes)
	if err != nil {
		logger.Error("Failed to create WASM module",
			"plugin_id", pluginID.String(),
			"error", err.Error())
		return nil, fmt.Errorf("failed to create module: %w", err)
	}

	// Define host functions
	linker := wasmtime.NewLinker(e.engine)

	// Link host functions
	if err := e.defineHostFunctions(linker, pluginID, name); err != nil {
		logger.Error("Failed to define host functions",
			"plugin_id", pluginID.String(),
			"error", err.Error())
		return nil, err
	}

	// Instantiate
	instance, err := linker.Instantiate(store, module)
	if err != nil {
		logger.Error("Failed to instantiate WASM module",
			"plugin_id", pluginID.String(),
			"error", err.Error())
		return nil, fmt.Errorf("failed to instantiate: %w", err)
	}

	// Get exported functions
	functions := &Functions{
		Init:          instance.GetFunc(store, "epbf_init"),
		Cleanup:       instance.GetFunc(store, "epbf_cleanup"),
		ProcessEvents: instance.GetFunc(store, "process_events"),
	}

	// Create instance
	ctx, cancel := context.WithCancel(ctx)

	inst := &Instance{
		ID:        pluginID,
		Name:      name,
		Store:     store,
		Module:    module,
		Instance:  instance,
		Functions: functions,
		ctx:       ctx,
		cancel:    cancel,
	}

	e.instances[pluginID] = inst

	logger.Info("✅ WASM plugin loaded",
		"plugin_id", pluginID.String(),
		"has_init", functions.Init != nil,
		"has_cleanup", functions.Cleanup != nil,
		"has_process_events", functions.ProcessEvents != nil)

	return inst, nil
}

// defineHostFunctions defines host functions available to WASM modules
func (e *Engine) defineHostFunctions(linker *wasmtime.Linker, pluginID uuid.UUID, pluginName string) error {
	// epbf_log - logging function
	if err := linker.FuncWrap("env", "epbf_log", func(level int32, ptr int32, length int32) {
		// In a real implementation, we would read the string from WASM memory
		logger.Debug("WASM log",
			"plugin_id", pluginID.String(),
			"level", level,
			"ptr", ptr,
			"length", length)
	}); err != nil {
		return fmt.Errorf("failed to define epbf_log: %w", err)
	}

	// epbf_now_ns - get current time in nanoseconds
	if err := linker.FuncWrap("env", "epbf_now_ns", func() int64 {
		return time.Now().UnixNano()
	}); err != nil {
		return fmt.Errorf("failed to define epbf_now_ns: %w", err)
	}

	// epbf_emit_counter - emit counter metric
	if err := linker.FuncWrap("env", "epbf_emit_counter", func(namePtr int32, nameLen int32, value int64, labelsPtr int32, labelsLen int32) {
		e.handleCounterMetric(pluginID, pluginName, namePtr, nameLen, value, labelsPtr, labelsLen)
	}); err != nil {
		return fmt.Errorf("failed to define epbf_emit_counter: %w", err)
	}

	// epbf_emit_gauge - emit gauge metric
	if err := linker.FuncWrap("env", "epbf_emit_gauge", func(namePtr int32, nameLen int32, value float64, labelsPtr int32, labelsLen int32) {
		e.handleGaugeMetric(pluginID, pluginName, namePtr, nameLen, value, labelsPtr, labelsLen)
	}); err != nil {
		return fmt.Errorf("failed to define epbf_emit_gauge: %w", err)
	}

	// epbf_emit_histogram - emit histogram metric (simplified)
	if err := linker.FuncWrap("env", "epbf_emit_histogram", func(namePtr int32, nameLen int32, value float64, labelsPtr int32, labelsLen int32) {
		e.handleHistogramMetric(pluginID, pluginName, namePtr, nameLen, value, labelsPtr, labelsLen)
	}); err != nil {
		return fmt.Errorf("failed to define epbf_emit_histogram: %w", err)
	}

	// epbf_subscribe_map - subscribe to eBPF map events
	if err := linker.FuncWrap("env", "epbf_subscribe_map", func(namePtr int32, nameLen int32) int32 {
		logger.Debug("WASM subscribe_map",
			"plugin_id", pluginID.String(),
			"name_ptr", namePtr,
			"name_len", nameLen)
		// In a real implementation, this would register the map subscription
		return 0 // Success
	}); err != nil {
		return fmt.Errorf("failed to define epbf_subscribe_map: %w", err)
	}

	// epbf_read_map - read from eBPF map
	if err := linker.FuncWrap("env", "epbf_read_map", func(mapNamePtr int32, mapNameLen int32, keyPtr int32, keyLen int32, valuePtr int32, valueLen int32) int32 {
		logger.Debug("WASM read_map",
			"plugin_id", pluginID.String(),
			"map_name_ptr", mapNamePtr,
			"key_ptr", keyPtr)
		return 0 // Success (stub)
	}); err != nil {
		return fmt.Errorf("failed to define epbf_read_map: %w", err)
	}

	// epbf_update_map - update eBPF map
	if err := linker.FuncWrap("env", "epbf_update_map", func(mapNamePtr int32, mapNameLen int32, keyPtr int32, keyLen int32, valuePtr int32, valueLen int32) int32 {
		logger.Debug("WASM update_map",
			"plugin_id", pluginID.String(),
			"map_name_ptr", mapNamePtr)
		return 0 // Success (stub)
	}); err != nil {
		return fmt.Errorf("failed to define epbf_update_map: %w", err)
	}

	return nil
}

// handleCounterMetric handles counter metric emission from WASM
func (e *Engine) handleCounterMetric(pluginID uuid.UUID, pluginName string, namePtr, nameLen int32, value int64, labelsPtr, labelsLen int32) {
	// In a real implementation, we would read the metric name and labels from WASM memory
	// For now, we just log the call
	logger.Debug("WASM emit_counter",
		"plugin_id", pluginID.String(),
		"plugin_name", pluginName,
		"name_ptr", namePtr,
		"value", value)

	// Store metric
	e.metricMu.Lock()
	defer e.metricMu.Unlock()

	e.metricStore.AddMetric(&filter.MetricValue{
		Name:      fmt.Sprintf("%s_%d", pluginName, namePtr),
		Value:     float64(value),
		Labels:    map[string]string{"plugin": pluginName},
		Timestamp: time.Now(),
	})
}

// handleGaugeMetric handles gauge metric emission from WASM
func (e *Engine) handleGaugeMetric(pluginID uuid.UUID, pluginName string, namePtr, nameLen int32, value float64, labelsPtr, labelsLen int32) {
	logger.Debug("WASM emit_gauge",
		"plugin_id", pluginID.String(),
		"plugin_name", pluginName,
		"name_ptr", namePtr,
		"value", value)

	e.metricMu.Lock()
	defer e.metricMu.Unlock()

	e.metricStore.AddMetric(&filter.MetricValue{
		Name:      fmt.Sprintf("%s_%d", pluginName, namePtr),
		Value:     value,
		Labels:    map[string]string{"plugin": pluginName},
		Timestamp: time.Now(),
	})
}

// handleHistogramMetric handles histogram metric emission from WASM
func (e *Engine) handleHistogramMetric(pluginID uuid.UUID, pluginName string, namePtr, nameLen int32, value float64, labelsPtr, labelsLen int32) {
	logger.Debug("WASM emit_histogram",
		"plugin_id", pluginID.String(),
		"plugin_name", pluginName,
		"name_ptr", namePtr,
		"value", value)

	e.metricMu.Lock()
	defer e.metricMu.Unlock()

	e.metricStore.AddMetric(&filter.MetricValue{
		Name:      fmt.Sprintf("%s_%d_bucket", pluginName, namePtr),
		Value:     value,
		Labels:    map[string]string{"plugin": pluginName, "le": "+Inf"},
		Timestamp: time.Now(),
	})
}

// GetInstance returns a plugin instance by ID
func (e *Engine) GetInstance(pluginID uuid.UUID) (*Instance, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	inst, ok := e.instances[pluginID]
	if !ok {
		return nil, fmt.Errorf("plugin instance not found: %s", pluginID.String())
	}

	return inst, nil
}

// UnloadPlugin unloads a WASM plugin
func (e *Engine) UnloadPlugin(pluginID uuid.UUID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	inst, ok := e.instances[pluginID]
	if !ok {
		return fmt.Errorf("plugin instance not found: %s", pluginID.String())
	}

	logger.Info("Unloading WASM plugin", "plugin_id", pluginID.String())

	// Cancel context
	inst.cancel()

	// Call cleanup if available
	if inst.Functions.Cleanup != nil {
		if _, err := inst.Functions.Cleanup.Call(inst.Store); err != nil {
			logger.Warn("Plugin cleanup failed", "plugin_id", pluginID.String(), "error", err.Error())
		}
	}

	// Remove from instances
	delete(e.instances, pluginID)

	logger.Info("✅ WASM plugin unloaded", "plugin_id", pluginID.String())

	return nil
}

// GetMetrics returns metrics collected from WASM plugins
func (e *Engine) GetMetrics() []*filter.MetricValue {
	e.metricMu.RLock()
	defer e.metricMu.RUnlock()
	return e.metricStore.GetAllMetrics()
}

// Close closes the WASM engine
func (e *Engine) Close() error {
	logger.Info("Closing WASM engine...")

	e.mu.Lock()
	defer e.mu.Unlock()

	// Unload all instances
	for id := range e.instances {
		if err := e.UnloadPlugin(id); err != nil {
			logger.Error("Failed to unload plugin during close",
				"plugin_id", id.String(),
				"error", err.Error())
		}
	}

	logger.Info("✅ WASM engine closed")
	return nil
}
