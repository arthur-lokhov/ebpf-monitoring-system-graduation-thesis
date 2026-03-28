package plugin

import (
	"context"
	"fmt"
	"time"

	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/epbf-monitoring/epbf-monitor/internal/metrics"
	"github.com/epbf-monitoring/epbf-monitor/internal/runtime/ebpf"
	wasmruntime "github.com/epbf-monitoring/epbf-monitor/internal/runtime/wasm"
	"github.com/epbf-monitoring/epbf-monitor/internal/storage/s3"
	"github.com/google/uuid"
)

// Runtime manages plugin runtime (eBPF + WASM)
type Runtime struct {
	ebpfLoader     *ebpf.Loader
	wasmRunner     *wasmruntime.Runner
	s3Client       *s3.Client
	metrics        *metrics.Collector
	pluginRuntimes map[uuid.UUID]*PluginRuntime
}

// PluginRuntime holds runtime state for a plugin
type PluginRuntime struct {
	ID          uuid.UUID
	Name        string
	Version     string
	EBPFProgram *ebpf.Program
	StartedAt   time.Time
	Enabled     bool
}

// NewRuntime creates a new plugin runtime manager
func NewRuntime(s3Client *s3.Client, metricsCollector *metrics.Collector) (*Runtime, error) {
	logger.Info("Creating plugin runtime manager...")

	// Create eBPF loader
	ebpfLoader, err := ebpf.NewLoader()
	if err != nil {
		return nil, fmt.Errorf("failed to create eBPF loader: %w", err)
	}

	// Create WASM engine and runner
	wasmEngine := wasmruntime.NewEngine()
	wasmRunner := wasmruntime.NewRunner(wasmEngine, "/tmp/epbf-builds")

	logger.Info("✅ Plugin runtime manager created")

	return &Runtime{
		ebpfLoader:     ebpfLoader,
		wasmRunner:     wasmRunner,
		s3Client:       s3Client,
		metrics:        metricsCollector,
		pluginRuntimes: make(map[uuid.UUID]*PluginRuntime),
	}, nil
}

// StartPlugin starts a plugin's eBPF and WASM components
func (r *Runtime) StartPlugin(ctx context.Context, pluginID uuid.UUID, name, version, ebpfS3Key, wasmS3Key string) error {
	logger.Info("Starting plugin runtime",
		"plugin_id", pluginID.String(),
		"name", name,
		"version", version)

	// Download eBPF object - TODO: implement when s3.Client.Download is fixed
	// For now, skip eBPF loading

	// Start WASM instance - TODO: load from S3
	// For now, skip WASM loading

	// Store runtime state
	r.pluginRuntimes[pluginID] = &PluginRuntime{
		ID:        pluginID,
		Name:      name,
		Version:   version,
		StartedAt: time.Now(),
		Enabled:   true,
	}

	logger.Info("✅ Plugin runtime started (placeholder)",
		"plugin_id", pluginID.String(),
		"name", name)

	return nil
}

// StopPlugin stops a plugin's runtime
func (r *Runtime) StopPlugin(pluginID uuid.UUID) error {
	runtime, ok := r.pluginRuntimes[pluginID]
	if !ok {
		return fmt.Errorf("plugin runtime not found: %s", pluginID.String())
	}

	logger.Info("Stopping plugin runtime",
		"plugin_id", pluginID.String(),
		"name", runtime.Name)

	// Stop WASM
	// if err := r.wasmRunner.StopPlugin(pluginID); err != nil { ... }

	// Unload eBPF
	// if runtime.EBPFProgram != nil { ... }

	// Update state
	runtime.Enabled = false

	logger.Info("✅ Plugin runtime stopped",
		"plugin_id", pluginID.String())

	return nil
}

// EnablePlugin enables a stopped plugin
func (r *Runtime) EnablePlugin(pluginID uuid.UUID) error {
	runtime, ok := r.pluginRuntimes[pluginID]
	if !ok {
		return fmt.Errorf("plugin runtime not found: %s", pluginID.String())
	}

	if runtime.Enabled {
		return fmt.Errorf("plugin is already enabled")
	}

	logger.Info("Enabling plugin",
		"plugin_id", pluginID.String(),
		"name", runtime.Name)

	runtime.Enabled = true
	r.metrics.WASMInstanceStarted(runtime.Name)
	if runtime.EBPFProgram != nil {
		r.metrics.EBPFProgramLoaded(runtime.Name)
	}

	logger.Info("✅ Plugin enabled",
		"plugin_id", pluginID.String())

	return nil
}

// DisablePlugin disables a running plugin
func (r *Runtime) DisablePlugin(pluginID uuid.UUID) error {
	runtime, ok := r.pluginRuntimes[pluginID]
	if !ok {
		return fmt.Errorf("plugin runtime not found: %s", pluginID.String())
	}

	if !runtime.Enabled {
		return fmt.Errorf("plugin is already disabled")
	}

	logger.Info("Disabling plugin",
		"plugin_id", pluginID.String(),
		"name", runtime.Name)

	runtime.Enabled = false
	r.metrics.WASMInstanceStopped(runtime.Name)
	if runtime.EBPFProgram != nil {
		r.metrics.EBPFProgramUnloaded(runtime.Name)
	}

	logger.Info("✅ Plugin disabled",
		"plugin_id", pluginID.String())

	return nil
}

// RemovePlugin removes a plugin's runtime
func (r *Runtime) RemovePlugin(pluginID uuid.UUID) error {
	runtime, ok := r.pluginRuntimes[pluginID]
	if !ok {
		return nil // Already removed
	}

	logger.Info("Removing plugin runtime",
		"plugin_id", pluginID.String(),
		"name", runtime.Name)

	// Stop if running
	if runtime.Enabled {
		if err := r.StopPlugin(pluginID); err != nil {
			logger.Error("Failed to stop plugin before removal",
				"plugin_id", pluginID.String(),
				"error", err.Error())
		}
	}

	// Remove from map
	delete(r.pluginRuntimes, pluginID)

	// Remove metrics
	r.metrics.RemovePluginMetrics(runtime.Name)

	logger.Info("✅ Plugin runtime removed",
		"plugin_id", pluginID.String())

	return nil
}

// GetRuntime returns runtime state for a plugin
func (r *Runtime) GetRuntime(pluginID uuid.UUID) (*PluginRuntime, error) {
	runtime, ok := r.pluginRuntimes[pluginID]
	if !ok {
		return nil, fmt.Errorf("plugin runtime not found: %s", pluginID.String())
	}
	return runtime, nil
}

// ListRuntimes returns all plugin runtimes
func (r *Runtime) ListRuntimes() []*PluginRuntime {
	runtimes := make([]*PluginRuntime, 0, len(r.pluginRuntimes))
	for _, rt := range r.pluginRuntimes {
		runtimes = append(runtimes, rt)
	}
	return runtimes
}

// Close closes the runtime manager
func (r *Runtime) Close() error {
	logger.Info("Closing plugin runtime manager...")

	// Stop all plugins
	for id := range r.pluginRuntimes {
		if err := r.StopPlugin(id); err != nil {
			logger.Error("Failed to stop plugin during close",
				"plugin_id", id.String(),
				"error", err.Error())
		}
	}

	// Close eBPF loader
	if err := r.ebpfLoader.Close(); err != nil {
		logger.Error("Failed to close eBPF loader", "error", err.Error())
	}

	// Close WASM runner
	if err := r.wasmRunner.Close(); err != nil {
		logger.Error("Failed to close WASM runner", "error", err.Error())
	}

	logger.Info("✅ Plugin runtime manager closed")
	return nil
}
