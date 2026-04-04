package plugin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/docker/docker/client"
	"github.com/epbf-monitoring/epbf-monitor/internal/filter"
	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/epbf-monitoring/epbf-monitor/internal/metrics"
	"github.com/epbf-monitoring/epbf-monitor/internal/plugin/builder"
	pg "github.com/epbf-monitoring/epbf-monitor/internal/storage/postgres"
	"github.com/epbf-monitoring/epbf-monitor/internal/storage/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Service handles plugin lifecycle management
type Service struct {
	loader       *Loader
	builder      *builder.Builder
	pluginRepo   *pg.PluginRepo
	storage      *s3.PluginStorage
	runtime      *Runtime
	metrics      *metrics.Collector
	filterEngine *filter.Engine
	dockerClient *client.Client
	buildDir     string
}

// Config holds service configuration
type Config struct {
	BuildDir      string
	BuilderImage  string
	EnableDocker  bool
}

// NewService creates a new plugin service
func NewService(
	pluginRepo *pg.PluginRepo,
	storage *s3.PluginStorage,
	metricsCollector *metrics.Collector,
	filterEngine *filter.Engine,
	cfg Config,
) (*Service, error) {
	var dockerClient *client.Client
	var err error

	if cfg.EnableDocker {
		// Try standard Docker socket first, then fallback to env
		dockerClient, err = client.NewClientWithOpts(
			client.WithHost("unix:///var/run/docker.sock"),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			// Fallback to env-based configuration
			dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil {
				return nil, fmt.Errorf("failed to create docker client: %w", err)
			}
		}
	}

	b, err := builder.NewBuilder(dockerClient, cfg.BuilderImage)
	if err != nil {
		return nil, fmt.Errorf("failed to create builder: %w", err)
	}

	// Create runtime manager (pass filterEngine)
	runtimeManager, err := NewRuntime(storage, metricsCollector, filterEngine)
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime manager: %w", err)
	}

	return &Service{
		loader:       NewLoader(cfg.BuildDir),
		builder:      b,
		pluginRepo:   pluginRepo,
		storage:      storage,
		runtime:      runtimeManager,
		metrics:      metricsCollector,
		filterEngine: filterEngine,
		dockerClient: dockerClient,
		buildDir:     cfg.BuildDir,
	}, nil
}

// LoadPlugin loads a plugin from a Git repository
func (s *Service) LoadPlugin(ctx context.Context, gitURL, ref string) (*pg.Plugin, error) {
	pluginID := uuid.New()
	pluginName, err := extractPluginName(gitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract plugin name: %w", err)
	}

	// Create plugin record in pending state
	plugin := &pg.Plugin{
		ID:        pluginID,
		Name:      pluginName,
		Version:   "pending",
		GitURL:    gitURL,
		GitBranch: ref,
		Status:    string(pg.PluginStatusPending),
		Manifest:  map[string]any{"name": pluginName, "version": "pending"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save to database
	if err := s.pluginRepo.Create(ctx, plugin); err != nil {
		return nil, fmt.Errorf("failed to save plugin: %w", err)
	}

	// Start async build
	go func() {
		bgCtx := context.Background()
		s.buildPlugin(bgCtx, plugin.ID, gitURL, ref)
	}()

	return plugin, nil
}

// buildPlugin handles the async build process
func (s *Service) buildPlugin(ctx context.Context, pluginID uuid.UUID, gitURL, ref string) {
	// Update status to building
	if err := s.pluginRepo.UpdateStatus(ctx, pluginID, pg.PluginStatusBuilding, "Starting build...", ""); err != nil {
		logError("failed to update status", err)
		return
	}

	// Load plugin from Git
	loadResult, err := s.loader.Load(ctx, LoadConfig{
		GitURL:    gitURL,
		Ref:       ref,
		PluginDir: s.buildDir,
	})
	if err != nil {
		s.handleBuildError(ctx, pluginID, err, "")
		return
	}

	// Build plugin
	buildResult, err := s.builder.Build(ctx, loadResult.PluginDir, loadResult.Manifest.Name)
	if err != nil {
		// Extract build log from BuildError if available
		buildLog := ""
		if buildErr, ok := err.(*builder.BuildError); ok {
			buildLog = buildErr.BuildLog
		}
		s.handleBuildError(ctx, pluginID, err, buildLog)
		return
	}

	if !buildResult.Success {
		s.handleBuildError(ctx, pluginID, fmt.Errorf("build failed: %s", buildResult.BuildLog), buildResult.BuildLog)
		return
	}

	// Upload artifacts to S3
	ebpfData, err := readFile(buildResult.EBPFFile)
	if err != nil {
		s.handleBuildError(ctx, pluginID, fmt.Errorf("failed to read eBPF file: %w", err), "")
		return
	}

	ebpfKey, err := s.storage.UploadEBPF(ctx, pluginID, ebpfData, int64(ebpfData.Len()))
	if err != nil {
		s.handleBuildError(ctx, pluginID, fmt.Errorf("failed to upload eBPF: %w", err), "")
		return
	}

	// Update plugin status to ready
	plugin := &pg.Plugin{
		ID:        pluginID,
		Name:      loadResult.Manifest.Name,
		Version:   loadResult.Manifest.Version,
		GitURL:    gitURL,
		GitCommit: loadResult.GitCommit,
		GitBranch: ref,
		EBPFS3Key: ebpfKey,
		Manifest:  manifestToMap(loadResult.Manifest),
		Status:    string(pg.PluginStatusStopped),
		BuildLog:  pgtype.Text{String: buildResult.BuildLog, Valid: true},
		UpdatedAt: time.Now(),
	}

	if err := s.pluginRepo.Update(ctx, plugin); err != nil {
		logError("failed to update plugin", err)
		return
	}

	// Record success metric
	if s.metrics != nil {
		s.metrics.PluginBuildSuccess(loadResult.Manifest.Name, loadResult.Manifest.Version, buildResult.Duration.Seconds())
	}

	// Start plugin runtime (eBPF only)
	if err := s.runtime.StartPlugin(ctx, pluginID, loadResult.Manifest.Name, loadResult.Manifest.Version, ebpfKey, manifestToMap(loadResult.Manifest)); err != nil {
		logger.Error("Failed to start plugin runtime",
			"plugin_id", pluginID.String(),
			"error", err.Error())
		// Continue anyway - runtime is optional
	}

	// Cleanup build directory
	s.loader.Cleanup(loadResult.Manifest.Name)
}

// handleBuildError updates plugin status on build error
func (s *Service) handleBuildError(ctx context.Context, pluginID uuid.UUID, err error, buildLog string) {
	logError("build error", err)
	errorMsg := err.Error()
	
	// Use provided build log or extract from BuildError
	if buildLog == "" {
		if buildErr, ok := err.(*builder.BuildError); ok {
			buildLog = buildErr.BuildLog
		}
	}
	
	if err := s.pluginRepo.UpdateStatus(ctx, pluginID, pg.PluginStatusError, buildLog, errorMsg); err != nil {
		logError("failed to update error status", err)
	}
}

// GetPlugin retrieves a plugin by ID
func (s *Service) GetPlugin(ctx context.Context, id uuid.UUID) (*pg.Plugin, error) {
	return s.pluginRepo.GetByID(ctx, id)
}

// GetPluginByName retrieves a plugin by name
func (s *Service) GetPluginByName(ctx context.Context, name string) (*pg.Plugin, error) {
	return s.pluginRepo.GetByName(ctx, name)
}

// ListPlugins lists all plugins
func (s *Service) ListPlugins(ctx context.Context, status *pg.PluginStatus) ([]*pg.Plugin, error) {
	return s.pluginRepo.List(ctx, status)
}

// DeletePlugin deletes a plugin and its artifacts
func (s *Service) DeletePlugin(ctx context.Context, id uuid.UUID) error {
	logger.Info("Deleting plugin", "plugin_id", id.String())

	// Stop runtime
	if err := s.runtime.RemovePlugin(id); err != nil {
		logger.Error("Failed to remove plugin runtime",
			"plugin_id", id.String(),
			"error", err.Error())
	}

	// Get plugin to find S3 keys
	plugin, err := s.pluginRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if plugin == nil {
		return fmt.Errorf("plugin not found")
	}

	// Delete from S3
	if err := s.storage.DeletePlugin(ctx, id); err != nil {
		logError("failed to delete from S3", err)
	}

	// Delete from database
	if err := s.pluginRepo.Delete(ctx, id); err != nil {
		return err
	}

	// Record metric
	s.metrics.PluginBuildFailure(plugin.Name, 0)

	logger.Info("✅ Plugin deleted", "plugin_id", id.String())
	return nil
}

// EnablePlugin enables a plugin
func (s *Service) EnablePlugin(ctx context.Context, id uuid.UUID) error {
	logger.Info("Enabling plugin", "plugin_id", id.String())

	// Get plugin from DB
	plugin, err := s.pluginRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get plugin: %w", err)
	}

	if plugin == nil {
		return fmt.Errorf("plugin not found")
	}

	// Update status in DB
	if err := s.pluginRepo.UpdateStatus(ctx, id, pg.PluginStatusReady, "", ""); err != nil {
		return err
	}

	// Start plugin runtime with eBPF from S3
	if plugin.EBPFS3Key != "" {
		if err := s.runtime.StartPlugin(ctx, id, plugin.Name, plugin.Version, plugin.EBPFS3Key, manifestToMapFromPlugin(plugin)); err != nil {
			logger.Error("Failed to start plugin runtime",
				"plugin_id", id.String(),
				"error", err.Error())
			// Continue anyway - runtime is optional
		}
	}

	logger.Info("✅ Plugin enabled", "plugin_id", id.String())
	return nil
}

// manifestToMapFromPlugin converts plugin manifest to map
func manifestToMapFromPlugin(p *pg.Plugin) map[string]any {
	return p.Manifest
}

// DisablePlugin disables a plugin
func (s *Service) DisablePlugin(ctx context.Context, id uuid.UUID) error {
	logger.Info("Disabling plugin", "plugin_id", id.String())

	// Update status in DB
	if err := s.pluginRepo.UpdateStatus(ctx, id, pg.PluginStatusStopped, "", ""); err != nil {
		return err
	}

	// Disable runtime
	if err := s.runtime.DisablePlugin(id); err != nil {
		return err
	}

	logger.Info("✅ Plugin disabled", "plugin_id", id.String())
	return nil
}

// RebuildPlugin rebuilds an existing plugin
func (s *Service) RebuildPlugin(ctx context.Context, id uuid.UUID) error {
	plugin, err := s.pluginRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if plugin == nil {
		return fmt.Errorf("plugin not found")
	}

	// Update status to building
	if err := s.pluginRepo.UpdateStatus(ctx, id, pg.PluginStatusBuilding, "Rebuilding...", ""); err != nil {
		return err
	}

	// Start async rebuild (use background context to avoid cancellation)
	go s.buildPlugin(context.Background(), id, plugin.GitURL, plugin.GitBranch)

	return nil
}

// StartAllPlugins loads and starts all plugins from the database
func (s *Service) StartAllPlugins(ctx context.Context) error {
	logger.Info("Starting all plugins from database...")

	// Get all plugins that are not in pending or building state
	plugins, err := s.pluginRepo.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list plugins: %w", err)
	}

	startedCount := 0
	skippedCount := 0
	errorCount := 0

	for _, plugin := range plugins {
		// Skip plugins without eBPF artifacts
		if plugin.EBPFS3Key == "" {
			logger.Debug("Skipping plugin (no eBPF artifacts)",
				"plugin_id", plugin.ID.String(),
				"name", plugin.Name,
				"status", plugin.Status)
			skippedCount++
			continue
		}

		// Skip plugins in pending or building state
		if plugin.Status == string(pg.PluginStatusPending) || plugin.Status == string(pg.PluginStatusBuilding) {
			logger.Debug("Skipping plugin (still building)",
				"plugin_id", plugin.ID.String(),
				"name", plugin.Name,
				"status", plugin.Status)
			skippedCount++
			continue
		}

		logger.Info("Starting plugin",
			"plugin_id", plugin.ID.String(),
			"name", plugin.Name,
			"version", plugin.Version,
			"status", plugin.Status)

		// Start plugin runtime
		if err := s.runtime.StartPlugin(ctx, plugin.ID, plugin.Name, plugin.Version, plugin.EBPFS3Key, manifestToMapFromPlugin(plugin)); err != nil {
			logger.Error("Failed to start plugin runtime",
				"plugin_id", plugin.ID.String(),
				"name", plugin.Name,
				"error", err.Error())
			errorCount++
			continue
		}

		// Update DB status to ready after successful start
		if err := s.pluginRepo.UpdateStatus(ctx, plugin.ID, pg.PluginStatusReady, "", ""); err != nil {
			logger.Error("Failed to update plugin status in DB",
				"plugin_id", plugin.ID.String(),
				"error", err.Error())
		}

		startedCount++
		logger.Info("✅ Plugin started",
			"plugin_id", plugin.ID.String(),
			"name", plugin.Name)
	}

	logger.Info("Plugin startup complete",
		"total", len(plugins),
		"started", startedCount,
		"skipped", skippedCount,
		"errors", errorCount)

	return nil
}

// Helper functions

func readFile(path string) (*bytes.Reader, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func manifestToMap(m *Manifest) map[string]any {
	// Convert full manifest to map
	result := map[string]any{
		"name":        m.Name,
		"version":     m.Version,
		"description": m.Description,
		"author":      m.Author,
	}

	fmt.Printf("DEBUG manifestToMap: name=%s, metrics count=%d\n", m.Name, len(m.Metrics))
	for i, metric := range m.Metrics {
		fmt.Printf("DEBUG manifestToMap: metric[%d]=%s type=%s\n", i, metric.Name, metric.Type)
	}

	// Convert ebpf config
	if m.EBPF.Entry != "" {
		ebpfMap := map[string]any{
			"entry": m.EBPF.Entry,
		}
		if len(m.EBPF.Programs) > 0 {
			programs := make([]map[string]any, len(m.EBPF.Programs))
			for i, p := range m.EBPF.Programs {
				programs[i] = map[string]any{
					"name":   p.Name,
					"type":   p.Type,
					"attach": p.Attach,
				}
			}
			ebpfMap["programs"] = programs
		}
		result["ebpf"] = ebpfMap
	}

	// Convert events
	fmt.Printf("DEBUG manifestToMap: events count=%d\n", len(m.Events))
	if len(m.Events) > 0 {
		events := make([]map[string]any, len(m.Events))
		for i, e := range m.Events {
			events[i] = map[string]any{
				"type":   e.Type,
				"name":   e.Name,
				"metric": e.Metric,
			}
			fmt.Printf("DEBUG manifestToMap: event[%d] type=%d name=%s metric=%s\n", i, e.Type, e.Name, e.Metric)
		}
		result["events"] = events
		fmt.Printf("DEBUG manifestToMap: added %d events to result\n", len(events))
	} else {
		fmt.Printf("DEBUG manifestToMap: NO events (m.Events is empty)\n")
	}

	// Convert wasm config
	if m.WASM.Entry != "" {
		result["wasm"] = map[string]any{
			"entry":       m.WASM.Entry,
			"sdk_version": m.WASM.SDKVersion,
		}
	}

	// Convert metrics
	if len(m.Metrics) > 0 {
		metrics := make([]map[string]any, len(m.Metrics))
		for i, metric := range m.Metrics {
			metricMap := map[string]any{
				"name": metric.Name,
				"type": metric.Type,
				"help": metric.Help,
			}
			// Debug: log labels before conversion
			fmt.Printf("DEBUG: metric %s has labels: %+v (type: %T)\n", metric.Name, metric.Labels, metric.Labels)
			if len(metric.Labels) > 0 {
				// Copy labels slice
				labelsCopy := make([]string, len(metric.Labels))
				copy(labelsCopy, metric.Labels)
				metricMap["labels"] = labelsCopy
				fmt.Printf("DEBUG: copied labels: %+v\n", labelsCopy)
			} else {
				fmt.Printf("DEBUG: metric %s has NO labels\n", metric.Name)
			}
			metrics[i] = metricMap
		}
		result["metrics"] = metrics
		fmt.Printf("DEBUG manifestToMap: added %d metrics to result\n", len(metrics))
	} else {
		fmt.Printf("DEBUG manifestToMap: NO metrics to add (m.Metrics is empty)\n")
	}

	// Convert filters
	if len(m.Filters) > 0 {
		filters := make([]map[string]any, len(m.Filters))
		for i, filter := range m.Filters {
			filters[i] = map[string]any{
				"name":        filter.Name,
				"expression":  filter.Expression,
				"description": filter.Description,
			}
		}
		result["filters"] = filters
	}

	return result
}

func logError(msg string, err error) {
	// TODO: Use proper logging
	fmt.Printf("ERROR: %s: %v\n", msg, err)
}
