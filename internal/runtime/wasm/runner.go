package runtime

import (
	"context"
	"time"

	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/google/uuid"
)

// Runner manages WASM plugin execution
type Runner struct {
	engine     *Engine
	pluginsDir string
}

// NewRunner creates a new WASM runner
func NewRunner(engine *Engine, pluginsDir string) *Runner {
	logger.Info("Creating WASM runner", "plugins_dir", pluginsDir)
	
	return &Runner{
		engine:     engine,
		pluginsDir: pluginsDir,
	}
}

// StartPlugin starts a WASM plugin
func (r *Runner) StartPlugin(ctx context.Context, pluginID uuid.UUID, name string, wasmBytes []byte) error {
	logger.Info("Starting WASM plugin", "plugin_id", pluginID.String(), "name", name)
	
	// Load plugin
	instance, err := r.engine.LoadPlugin(ctx, pluginID, name, wasmBytes)
	if err != nil {
		logger.Error("Failed to load WASM plugin",
			"plugin_id", pluginID.String(),
			"error", err.Error())
		return err
	}
	
	// Call init if available
	if instance.Functions.Init != nil {
		logger.Debug("Calling epbf_init", "plugin_id", pluginID.String())
		
		result, err := instance.Functions.Init.Call(instance.Store)
		if err != nil {
			logger.Error("epbf_init failed",
				"plugin_id", pluginID.String(),
				"error", err.Error())
			return err
		}
		
		logger.Debug("epbf_init completed", "plugin_id", pluginID.String(), "result", result)
	}
	
	// Start event processing goroutine
	go r.processEvents(ctx, instance)
	
	logger.Info("✅ WASM plugin started", "plugin_id", pluginID.String())
	return nil
}

// processEvents runs the plugin's event processing loop
func (r *Runner) processEvents(ctx context.Context, instance *Instance) {
	logger.Debug("Starting event processing loop", "plugin_id", instance.ID.String())
	
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			logger.Debug("Event processing loop stopped", "plugin_id", instance.ID.String())
			return
			
		case <-ticker.C:
			if instance.Functions.ProcessEvents != nil {
				_, err := instance.Functions.ProcessEvents.Call(instance.Store)
				if err != nil {
					logger.Error("process_events failed",
						"plugin_id", instance.ID.String(),
						"error", err.Error())
				}
			}
		}
	}
}

// StopPlugin stops a WASM plugin
func (r *Runner) StopPlugin(pluginID uuid.UUID) error {
	logger.Info("Stopping WASM plugin", "plugin_id", pluginID.String())
	
	if err := r.engine.UnloadPlugin(pluginID); err != nil {
		logger.Error("Failed to stop WASM plugin",
			"plugin_id", pluginID.String(),
			"error", err.Error())
		return err
	}
	
	logger.Info("✅ WASM plugin stopped", "plugin_id", pluginID.String())
	return nil
}

// Close closes the runner
func (r *Runner) Close() error {
	logger.Info("Closing WASM runner...")
	return r.engine.Close()
}
