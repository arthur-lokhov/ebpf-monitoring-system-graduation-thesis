package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytecodealliance/wasmtime-go/v21"
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
	Init      *wasmtime.Func
	Cleanup   *wasmtime.Func
	ProcessEvents *wasmtime.Func
}

// Engine manages WASM runtime
type Engine struct {
	engine   *wasmtime.Engine
	instances map[uuid.UUID]*Instance
	mu        sync.RWMutex
}

// NewEngine creates a new WASM engine
func NewEngine() *Engine {
	logger.Info("Creating WASM engine...")
	
	config := wasmtime.NewConfig()
	config.SetEpochInterruption(true)
	
	engine := wasmtime.NewEngineWithConfig(config)
	
	logger.Info("✅ WASM engine created")
	
	return &Engine{
		engine:    engine,
		instances: make(map[uuid.UUID]*Instance),
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
	if err := e.defineHostFunctions(linker, pluginID); err != nil {
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
		Init:      instance.GetFunc(store, "epbf_init"),
		Cleanup:   instance.GetFunc(store, "epbf_cleanup"),
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
func (e *Engine) defineHostFunctions(linker *wasmtime.Linker, pluginID uuid.UUID) error {
	// epbf_init_wrapper
	if err := linker.FuncWrap("env", "epbf_log", func(ptr int32, len int32) {
		logger.Debug("WASM log call", "plugin_id", pluginID.String(), "ptr", ptr, "len", len)
	}); err != nil {
		return fmt.Errorf("failed to define epbf_log: %w", err)
	}
	
	// epbf_now_ns
	if err := linker.FuncWrap("env", "epbf_now_ns", func() int64 {
		return 0 // TODO: Implement
	}); err != nil {
		return fmt.Errorf("failed to define epbf_now_ns: %w", err)
	}
	
	// epbf_emit_counter
	if err := linker.FuncWrap("env", "epbf_emit_counter", func(namePtr int32, nameLen int32, value int64, labelsPtr int32, labelsLen int32) {
		logger.Debug("WASM emit_counter call",
			"plugin_id", pluginID.String(),
			"name_ptr", namePtr,
			"value", value)
	}); err != nil {
		return fmt.Errorf("failed to define epbf_emit_counter: %w", err)
	}
	
	// epbf_emit_gauge
	if err := linker.FuncWrap("env", "epbf_emit_gauge", func(namePtr int32, nameLen int32, value float64, labelsPtr int32, labelsLen int32) {
		logger.Debug("WASM emit_gauge call",
			"plugin_id", pluginID.String(),
			"name_ptr", namePtr,
			"value", value)
	}); err != nil {
		return fmt.Errorf("failed to define epbf_emit_gauge: %w", err)
	}
	
	// epbf_subscribe_map
	if err := linker.FuncWrap("env", "epbf_subscribe_map", func(namePtr int32, nameLen int32) int32 {
		logger.Debug("WASM subscribe_map call",
			"plugin_id", pluginID.String(),
			"name_ptr", namePtr)
		return 0 // TODO: Implement
	}); err != nil {
		return fmt.Errorf("failed to define epbf_subscribe_map: %w", err)
	}
	
	return nil
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
