package plugin

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Manifest represents plugin manifest file
type Manifest struct {
	// Metadata
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`

	// eBPF configuration
	EBPF EBPFConfig `yaml:"ebpf"`

	// WASM configuration
	WASM WASMConfig `yaml:"wasm"`

	// Metrics definitions
	Metrics []MetricDef `yaml:"metrics"`

	// Filters (optional)
	Filters []FilterDef `yaml:"filters,omitempty"`
}

// EBPFConfig holds eBPF program configuration
type EBPFConfig struct {
	Entry    string        `yaml:"entry"`
	Programs []EBPFProgram `yaml:"programs"`
}

// EBPFProgram represents a single eBPF program
type EBPFProgram struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"` // kprobe, tracepoint, uprobe, etc.
	Attach string `yaml:"attach"`
}

// WASMConfig holds WASM module configuration
type WASMConfig struct {
	Entry      string `yaml:"entry"`
	SDKVersion string `yaml:"sdk_version"`
}

// MetricDef represents a metric definition
type MetricDef struct {
	Name   string   `yaml:"name"`
	Type   string   `yaml:"type"` // counter, gauge, histogram, summary
	Help   string   `yaml:"help"`
	Labels []string `yaml:"labels,omitempty"`
}

// FilterDef represents a filter definition
type FilterDef struct {
	Name        string `yaml:"name"`
	Expression  string `yaml:"expression"`
	Description string `yaml:"description,omitempty"`
}

// ParseManifest parses manifest.yml content
func ParseManifest(data []byte) (*Manifest, error) {
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &manifest, nil
}

// LoadManifest loads manifest from file
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	return ParseManifest(data)
}

// Validate validates the manifest
func (m *Manifest) Validate() error {
	// Required fields
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}

	if m.Version == "" {
		return fmt.Errorf("version is required")
	}

	// eBPF validation
	if m.EBPF.Entry == "" {
		return fmt.Errorf("ebpf.entry is required")
	}

	if len(m.EBPF.Programs) == 0 {
		return fmt.Errorf("ebpf.programs must contain at least one program")
	}

	for i, prog := range m.EBPF.Programs {
		if prog.Name == "" {
			return fmt.Errorf("ebpf.programs[%d].name is required", i)
		}
		if prog.Type == "" {
			return fmt.Errorf("ebpf.programs[%d].type is required", i)
		}
		if prog.Attach == "" {
			return fmt.Errorf("ebpf.programs[%d].attach is required", i)
		}
	}

	// WASM validation
	if m.WASM.Entry == "" {
		return fmt.Errorf("wasm.entry is required")
	}

	// Metrics validation
	if len(m.Metrics) == 0 {
		return fmt.Errorf("metrics must contain at least one metric")
	}

	for i, metric := range m.Metrics {
		if metric.Name == "" {
			return fmt.Errorf("metrics[%d].name is required", i)
		}
		if metric.Type == "" {
			return fmt.Errorf("metrics[%d].type is required", i)
		}
		if !isValidMetricType(metric.Type) {
			return fmt.Errorf("metrics[%d].type must be counter, gauge, histogram, or summary", i)
		}
	}

	return nil
}

// isValidMetricType checks if metric type is valid
func isValidMetricType(t string) bool {
	validTypes := map[string]bool{
		"counter":   true,
		"gauge":     true,
		"histogram": true,
		"summary":   true,
	}
	return validTypes[t]
}

// FindFile searches for a file in the plugin directory
func FindFile(pluginDir string, fileName string) (string, error) {
	var found string

	err := filepath.WalkDir(pluginDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && d.Name() == fileName {
			found = path
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", err
	}

	if found == "" {
		return "", fmt.Errorf("file %s not found", fileName)
	}

	return found, nil
}
