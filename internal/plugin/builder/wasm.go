package builder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WASMBuilder handles WASM module compilation
type WASMBuilder struct {
	ClangPath string
	SDKDir    string
}

// WASMBuildResult holds WASM build results
type WASMBuildResult struct {
	WASMFile   string
	Exports    []string
	Size       int64
}

// NewWASMBuilder creates a new WASM builder
func NewWASMBuilder() *WASMBuilder {
	return &WASMBuilder{
		ClangPath: "clang",
		SDKDir:    "./pkg/wasmsdk",
	}
}

// Build compiles a WASM module
func (b *WASMBuilder) Build(ctx context.Context, sourceFile, outputFile string) (*WASMBuildResult, error) {
	// Ensure output directory exists
	outputDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Build clang command for WASM
	args := []string{
		"-O2", "-g", "-Wall", "-Wextra",
		"--target=wasm32",
		"-nostdlib",
		"-Wl,--no-entry",
		"-Wl,--export=epbf_init",
		"-Wl,--export=__data_end",
		"-Wl,--export=__heap_base",
		"-Wl,--strip-debug",
		"-Wl,--allow-undefined",
		"-I" + filepath.Join(b.SDKDir, "include"),
		sourceFile,
		"-o", outputFile,
	}

	cmd := exec.CommandContext(ctx, b.ClangPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("clang failed: %w\n%s", err, string(output))
	}

	// Get file info
	fileInfo, err := os.Stat(outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output file: %w", err)
	}

	// Parse exports
	exports, err := b.ParseExports(ctx, outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse exports: %w", err)
	}

	return &WASMBuildResult{
		WASMFile: outputFile,
		Exports:  exports,
		Size:     fileInfo.Size(),
	}, nil
}

// ParseExports extracts exported symbols from WASM module
func (b *WASMBuilder) ParseExports(ctx context.Context, wasmFile string) ([]string, error) {
	// Try wasm-objdump first
	objdumpPath, err := exec.LookPath("wasm-objdump")
	if err == nil {
		cmd := exec.CommandContext(ctx, objdumpPath, "-x", wasmFile)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return b.parseWasmExports(string(output))
		}
	}

	// Fallback: use wasm2wat if available
	wasm2watPath, err := exec.LookPath("wasm2wat")
	if err == nil {
		cmd := exec.CommandContext(ctx, wasm2watPath, wasmFile)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return b.parseWatExports(string(output))
		}
	}

	// Return default exports if tools not available
	return []string{"epbf_init", "__data_end", "__heap_base"}, nil
}

// parseWasmExports extracts exports from wasm-objdump output
func (b *WASMBuilder) parseWasmExports(output string) ([]string, error) {
	exports := []string{}
	inExportSection := false

	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "export[") {
			inExportSection = true
			continue
		}
		if inExportSection && strings.Contains(line, "- func") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				name := strings.Trim(parts[2], "\"")
				exports = append(exports, name)
			}
		}
	}

	return exports, nil
}

// parseWatExports extracts exports from WAT format
func (b *WASMBuilder) parseWatExports(output string) ([]string, error) {
	exports := []string{}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "(export") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := strings.Trim(parts[1], "\"")
				exports = append(exports, name)
			}
		}
	}

	return exports, nil
}

// Validate checks if WASM module has required exports
func (b *WASMBuilder) Validate(ctx context.Context, wasmFile string) error {
	exports, err := b.ParseExports(ctx, wasmFile)
	if err != nil {
		return err
	}

	// Check for required export
	hasInit := false
	for _, exp := range exports {
		if exp == "epbf_init" {
			hasInit = true
			break
		}
	}

	if !hasInit {
		return fmt.Errorf("WASM module missing required export: epbf_init")
	}

	return nil
}

// Clean removes build artifacts
func (b *WASMBuilder) Clean(wasmFile string) error {
	return os.Remove(wasmFile)
}

// GetClangVersion returns the clang version
func (b *WASMBuilder) GetClangVersion() (string, error) {
	cmd := exec.Command(b.ClangPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return lines[0], nil
	}
	return "", fmt.Errorf("no version output")
}

// HasWasmTarget checks if clang supports WASM target
func (b *WASMBuilder) HasWasmTarget() bool {
	cmd := exec.Command(b.ClangPath, "--target=wasm32", "-E", "-x", "c", "-", "-v")
	cmd.Stdin = strings.NewReader("")
	output, _ := cmd.CombinedOutput()
	return strings.Contains(string(output), "wasm32")
}
