package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// LoadConfig holds plugin load configuration
type LoadConfig struct {
	GitURL    string
	Ref       string // branch, tag, or commit
	PluginDir string // base directory for plugin storage
}

// LoadResult holds the result of plugin loading
type LoadResult struct {
	PluginDir  string
	Manifest   *Manifest
	GitCommit  string
	GitRef     string
	LoadTime   time.Duration
}

// Loader handles plugin loading from Git repositories
type Loader struct {
	baseDir string
}

// NewLoader creates a new plugin loader
func NewLoader(baseDir string) *Loader {
	return &Loader{
		baseDir: baseDir,
	}
}

// Load clones a Git repository and loads the plugin
func (l *Loader) Load(ctx context.Context, cfg LoadConfig) (*LoadResult, error) {
	startTime := time.Now()

	// Validate Git URL
	if cfg.GitURL == "" {
		return nil, fmt.Errorf("git URL is required")
	}

	// Generate plugin directory name from URL
	pluginName, err := extractPluginName(cfg.GitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract plugin name: %w", err)
	}

	pluginDir := filepath.Join(l.baseDir, pluginName)

	// Remove existing directory if it exists
	if _, err := os.Stat(pluginDir); err == nil {
		if err := os.RemoveAll(pluginDir); err != nil {
			return nil, fmt.Errorf("failed to remove existing plugin directory: %w", err)
		}
	}

	// Create base directory
	if err := os.MkdirAll(l.baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	// Clone repository
	cloneOpts := &git.CloneOptions{
		URL:      cfg.GitURL,
		Progress: os.Stdout,
	}

	if cfg.Ref != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(cfg.Ref)
	}

	repo, err := git.PlainCloneContext(ctx, pluginDir, false, cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Get commit hash
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get repository head: %w", err)
	}

	gitCommit := head.Hash().String()
	gitRef := head.Name().Short()

	// Load and validate manifest
	manifestPath := filepath.Join(pluginDir, "manifest.yml")
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load manifest: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	// Verify required files exist
	requiredFiles := []string{
		filepath.Join(pluginDir, manifest.EBPF.Entry),
		filepath.Join(pluginDir, manifest.WASM.Entry),
	}

	for _, file := range requiredFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return nil, fmt.Errorf("required file not found: %s", file)
		}
	}

	loadTime := time.Since(startTime)

	return &LoadResult{
		PluginDir: pluginDir,
		Manifest:  manifest,
		GitCommit: gitCommit,
		GitRef:    gitRef,
		LoadTime:  loadTime,
	}, nil
}

// extractPluginName extracts plugin name from Git URL
func extractPluginName(gitURL string) (string, error) {
	// Remove protocol prefix
	url := gitURL
	for _, prefix := range []string{"https://", "http://", "git://", "ssh://"} {
		if len(url) > len(prefix) && url[:len(prefix)] == prefix {
			url = url[len(prefix):]
			break
		}
	}

	// Remove user info (user@)
	if idx := filepath.IndexByte(url, '@'); idx != -1 {
		url = url[idx+1:]
	}

	// Remove host
	if idx := filepath.IndexByte(url, '/'); idx != -1 {
		url = url[idx+1:]
	}

	// Remove .git suffix
	if len(url) > 4 && url[len(url)-4:] == ".git" {
		url = url[:len(url)-4]
	}

	// Validate result
	if url == "" {
		return "", fmt.Errorf("invalid Git URL")
	}

	return url, nil
}

// Cleanup removes a plugin directory
func (l *Loader) Cleanup(pluginName string) error {
	pluginDir := filepath.Join(l.baseDir, pluginName)
	return os.RemoveAll(pluginDir)
}
