package plugin

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/epbf-monitoring/epbf-monitor/internal/filter"
	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/epbf-monitoring/epbf-monitor/internal/metrics"
	"github.com/epbf-monitoring/epbf-monitor/internal/runtime/ebpf"
	"github.com/epbf-monitoring/epbf-monitor/internal/storage/s3"
	"github.com/google/uuid"
)

// Runtime manages plugin runtime (eBPF only)
type Runtime struct {
	ebpfLoader     *ebpf.Loader
	s3Client       *s3.Client
	metrics        *metrics.Collector
	filterEngine   *filter.Engine
	pluginRuntimes map[uuid.UUID]*PluginRuntime
	mu             sync.RWMutex
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
func NewRuntime(pluginStorage *s3.PluginStorage, metricsCollector *metrics.Collector, filterEngine *filter.Engine) (*Runtime, error) {
	logger.Info("Creating plugin runtime manager...")

	// Create eBPF loader
	ebpfLoader, err := ebpf.NewLoader()
	if err != nil {
		return nil, fmt.Errorf("failed to create eBPF loader: %w", err)
	}

	// Get S3 client from PluginStorage
	var s3Client *s3.Client
	if pluginStorage != nil {
		s3Client = pluginStorage.GetClient()
	}

	logger.Info("✅ Plugin runtime manager created")

	return &Runtime{
		ebpfLoader:     ebpfLoader,
		s3Client:       s3Client,
		metrics:        metricsCollector,
		filterEngine:   filterEngine,
		pluginRuntimes: make(map[uuid.UUID]*PluginRuntime),
	}, nil
}

// StartPlugin starts a plugin's eBPF component
func (r *Runtime) StartPlugin(ctx context.Context, pluginID uuid.UUID, name, version, ebpfS3Key string, manifest map[string]any) error {
	logger.Info("Starting plugin runtime",
		"plugin_id", pluginID.String(),
		"name", name,
		"version", version,
		"ebpf_key", ebpfS3Key)

	// Register dynamic metrics from manifest
	if manifest != nil {
		if err := r.metrics.DynamicMetrics().RegisterPluginMetrics(name, version, manifest); err != nil {
			logger.Error("Failed to register plugin metrics",
				"plugin_id", pluginID.String(),
				"error", err.Error())
		}
	}

	var ebpfProgram *ebpf.Program

	// Download and load eBPF program
	if r.s3Client != nil && ebpfS3Key != "" {
		ebpfBytes, err := r.downloadFromS3(ctx, ebpfS3Key)
		if err != nil {
			logger.Error("Failed to download eBPF object",
				"plugin_id", pluginID.String(),
				"key", ebpfS3Key,
				"error", err.Error())
		} else {
			// Extract ebpf programs info from manifest for correct tracepoint mapping
			var ebpfPrograms []struct{ Name, Attach string }
			if manifest != nil {
				logger.Info("DEBUG: manifest is not nil", "keys", getMapKeys(manifest))
				if ebpf, ok := manifest["ebpf"].(map[string]any); ok {
					logger.Info("DEBUG: ebpf section found", "ebpf_keys", getMapKeys(ebpf))
					programsRaw, exists := ebpf["programs"]
					if !exists {
						logger.Warn("DEBUG: programs key missing")
					} else {
						// Try both []any and []map[string]any
						var programs []map[string]any
						if p1, ok := programsRaw.([]any); ok {
							for _, item := range p1 {
								if m, ok := item.(map[string]any); ok {
									programs = append(programs, m)
								}
							}
						} else if p2, ok := programsRaw.([]map[string]any); ok {
							programs = p2
						} else {
							logger.Warn("DEBUG: programs unknown type", "type", fmt.Sprintf("%T", programsRaw))
						}

						for _, p := range programs {
							name, _ := p["name"].(string)
							attach, _ := p["attach"].(string)
							logger.Info("DEBUG: program", "name", name, "attach", attach)
							ebpfPrograms = append(ebpfPrograms, struct{ Name, Attach string }{name, attach})
						}
					}
				}
			} else {
				logger.Warn("DEBUG: manifest is nil")
			}
			logger.Info("DEBUG: extracted ebpfPrograms", "count", len(ebpfPrograms))

			// Extract event mapping from manifest
			eventMetrics := make(map[uint8]string)
			eventNames := make(map[uint8]string)
			if manifest != nil {
				eventsRaw, hasEvents := manifest["events"]
				if hasEvents {
					var events []map[string]any
					if e1, ok := eventsRaw.([]any); ok {
						for _, item := range e1 {
							if m, ok := item.(map[string]any); ok {
								events = append(events, m)
							}
						}
					} else if e2, ok := eventsRaw.([]map[string]any); ok {
						events = e2
					}

					for _, e := range events {
						var eventTypeNum uint8
						if ft, ok := e["type"].(float64); ok {
							eventTypeNum = uint8(ft)
						} else if it, ok := e["type"].(int); ok {
							eventTypeNum = uint8(it)
						} else {
							continue
						}
						metricName, _ := e["metric"].(string)
						eventName, _ := e["name"].(string)
						if metricName != "" {
							eventMetrics[eventTypeNum] = metricName
						}
						if eventName != "" {
							eventNames[eventTypeNum] = eventName
						}
					}
				}
			}

			ebpfProgram, err = r.ebpfLoader.LoadProgramWithManifest(ctx, pluginID, name, ebpfBytes, ebpfPrograms, func(event ebpf.EBPFEvent) {
				// Handle eBPF event - lookup metric name from manifest event mapping
				eventType := "unknown"
				metricName := ""

				if len(event.Data) > 0 {
					eventTypeNum := event.Data[0]
					// Lookup metric name from manifest
					if mn, ok := eventMetrics[eventTypeNum]; ok {
						metricName = mn
					}
					// Lookup event name from manifest
					if en, ok := eventNames[eventTypeNum]; ok {
						eventType = en
					} else {
						eventType = fmt.Sprintf("type_%d", eventTypeNum)
					}
				}

				// logger.Info("eBPF event received",
				// 	"plugin_id", pluginID.String(),
				// 	"type", eventType,
				// 	"pid", event.PID,
				// 	"comm", string(event.Comm[:]))

				// Emit metric to filter engine
				if metricName != "" {
					r.filterEngine.AddMetric(&filter.MetricValue{
						Name:      metricName,
						Value:     1,
						Labels:    map[string]string{"plugin": name},
						Timestamp: time.Now(),
					})

					r.metrics.EBPFEventReceived(name, eventType)
				}
			})
			if err != nil {
				logger.Error("Failed to load eBPF program",
					"plugin_id", pluginID.String(),
					"error", err.Error())
			} else {
				logger.Info("✅ eBPF program loaded",
					"plugin_id", pluginID.String(),
					"name", name)
				r.metrics.EBPFProgramLoaded(name)
			}
		}
	}

	// Store runtime state
	r.mu.Lock()
	r.pluginRuntimes[pluginID] = &PluginRuntime{
		ID:          pluginID,
		Name:        name,
		Version:     version,
		EBPFProgram: ebpfProgram,
		StartedAt:   time.Now(),
		Enabled:     true,
	}
	r.mu.Unlock()

	logger.Info("✅ Plugin runtime started",
		"plugin_id", pluginID.String(),
		"has_ebpf", ebpfProgram != nil)

	return nil
}

// Helper function to get map keys
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// downloadFromS3 downloads a file from S3
func (r *Runtime) downloadFromS3(ctx context.Context, key string) ([]byte, error) {
	reader, err := r.s3Client.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object: %w", err)
	}

	logger.Debug("Downloaded from S3", "key", key, "size", len(data))
	return data, nil
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

	// Update state
	runtime.Enabled = false

	logger.Info("✅ Plugin runtime stopped",
		"plugin_id", pluginID.String())

	return nil
}

// EnablePlugin enables a stopped plugin
func (r *Runtime) EnablePlugin(pluginID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	runtime, ok := r.pluginRuntimes[pluginID]
	if !ok {
		return fmt.Errorf("plugin runtime not found: %s", pluginID.String())
	}

	if runtime.Enabled {
		return nil // Already enabled, no-op
	}

	logger.Info("Enabling plugin",
		"plugin_id", pluginID.String(),
		"name", runtime.Name)

	runtime.Enabled = true
	r.metrics.EBPFProgramLoaded(runtime.Name)

	logger.Info("✅ Plugin enabled",
		"plugin_id", pluginID.String())

	return nil
}

// DisablePlugin disables a running plugin
func (r *Runtime) DisablePlugin(pluginID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	runtime, ok := r.pluginRuntimes[pluginID]
	if !ok {
		return fmt.Errorf("plugin runtime not found: %s", pluginID.String())
	}

	if !runtime.Enabled {
		return nil // Already disabled, no-op
	}

	logger.Info("Disabling plugin",
		"plugin_id", pluginID.String(),
		"name", runtime.Name)

	runtime.Enabled = false

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

	logger.Info("✅ Plugin runtime manager closed")
	return nil
}
