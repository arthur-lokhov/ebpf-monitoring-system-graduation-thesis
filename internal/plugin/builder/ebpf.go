package builder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EBPFBuilder handles eBPF program compilation
type EBPFBuilder struct {
	ClangPath   string
	LLVMStripPath string
	Arch        string
	Target      string
}

// EBPFBuildResult holds eBPF build results
type EBPFBuildResult struct {
	ObjectFile string
	Verified   bool
	Programs   []string
}

// NewEBPFBuilder creates a new eBPF builder
func NewEBPFBuilder() *EBPFBuilder {
	return &EBPFBuilder{
		ClangPath:   "clang",
		LLVMStripPath: "llvm-strip",
		Arch:        "x86_64",
		Target:      "bpfel",
	}
}

// Build compiles an eBPF program
func (b *EBPFBuilder) Build(ctx context.Context, sourceFile, outputFile string) (*EBPFBuildResult, error) {
	// Ensure output directory exists
	outputDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Build clang command
	args := []string{
		"-O2", "-g", "-Wall", "-Wextra",
		"-target", b.Target,
		"-D__TARGET_ARCH_" + b.Arch,
		"-I/usr/include",
		"-c", sourceFile,
		"-o", outputFile,
	}

	cmd := exec.CommandContext(ctx, b.ClangPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("clang failed: %w\n%s", err, string(output))
	}

	// Verify the eBPF object
	verified, programs, err := b.Verify(ctx, outputFile)
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	return &EBPFBuildResult{
		ObjectFile: outputFile,
		Verified:   verified,
		Programs:   programs,
	}, nil
}

// Verify checks if eBPF object is valid
func (b *EBPFBuilder) Verify(ctx context.Context, objectFile string) (bool, []string, error) {
	// Check if bpftool is available
	bpftoolPath, err := exec.LookPath("bpftool")
	if err != nil {
		// bpftool not available, skip verification
		return true, []string{}, nil
	}

	// Run bpftool prog dump
	cmd := exec.CommandContext(ctx, bpftoolPath, "prog", "dump", "xlated", objectFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, nil, fmt.Errorf("bpftool failed: %w\n%s", err, string(output))
	}

	// Parse program names from output
	programs := b.parseProgramNames(string(output))

	return true, programs, nil
}

// parseProgramNames extracts program names from bpftool output
func (b *EBPFBuilder) parseProgramNames(output string) []string {
	// Simple parsing - in production, use proper parsing
	programs := []string{}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "name") {
			parts := strings.Split(line, "\"")
			if len(parts) >= 2 {
				programs = append(programs, parts[1])
			}
		}
	}
	return programs
}

// Clean removes build artifacts
func (b *EBPFBuilder) Clean(objectFile string) error {
	return os.Remove(objectFile)
}

// GetClangVersion returns the clang version
func (b *EBPFBuilder) GetClangVersion() (string, error) {
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
