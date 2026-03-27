package builder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Verifier handles eBPF program verification
type Verifier struct {
	MaxInstructions int
	AllowedHelpers  []int
	StrictMode      bool
}

// VerificationResult holds verification results
type VerificationResult struct {
	Valid    bool
	Errors   []string
	Warnings []string
	Info     VerifierInfo
}

// VerifierInfo holds program information
type VerifierInfo struct {
	InstructionCount int
	MemoryUsage      int
	HelperFunctions  []int
	LoopDetected     bool
}

// NewVerifier creates a new eBPF verifier
func NewVerifier() *Verifier {
	return &Verifier{
		MaxInstructions: 1000000, // Linux kernel default
		AllowedHelpers:  []int{}, // Empty means all allowed
		StrictMode:      false,
	}
}

// Verify validates an eBPF object file
func (v *Verifier) Verify(ctx context.Context, objectFile string) (*VerificationResult, error) {
	result := &VerificationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Check if file exists
	if _, err := os.Stat(objectFile); os.IsNotExist(err) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("file not found: %s", objectFile))
		return result, nil
	}

	// Check file size
	fileInfo, err := os.Stat(objectFile)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	if fileInfo.Size() == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "eBPF object file is empty")
		return result, nil
	}

	// Try to use bpftool for verification
	if err := v.verifyWithBpftool(ctx, objectFile, result); err != nil {
		// bpftool not available or failed, use basic checks
		v.basicChecks(objectFile, result)
	}

	return result, nil
}

// verifyWithBpftool uses bpftool for verification
func (v *Verifier) verifyWithBpftool(ctx context.Context, objectFile string, result *VerificationResult) error {
	// Check if bpftool is available
	bpftoolPath, err := exec.LookPath("bpftool")
	if err != nil {
		return fmt.Errorf("bpftool not found")
	}

	// Run bpftool prog dump
	cmd := exec.CommandContext(ctx, bpftoolPath, "prog", "dump", "xlated", objectFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)

		// Check for specific errors
		if strings.Contains(outputStr, "Error") {
			result.Valid = false
			result.Errors = append(result.Errors, extractError(outputStr))
		}
		return fmt.Errorf("bpftool failed: %w", err)
	}

	// Parse output for info
	v.parseBpftoolOutput(string(output), result)

	return nil
}

// basicChecks performs basic validation without bpftool
func (v *Verifier) basicChecks(objectFile string, result *VerificationResult) {
	// Read file header
	data, err := os.ReadFile(objectFile)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to read file: %v", err))
		return
	}

	// Check ELF magic number
	if len(data) < 16 || !isELF(data) {
		result.Valid = false
		result.Errors = append(result.Errors, "invalid ELF file")
		return
	}

	// Check for eBPF section
	hasEBPFSection := false
	sections := []string{".text", ".tracepoint", ".kprobe", ".kretprobe", ".uprobe", ".uretprobe", ".socket_filter"}

	for _, section := range sections {
		if strings.Contains(string(data), section) {
			hasEBPFSection = true
			break
		}
	}

	if !hasEBPFSection {
		result.Warnings = append(result.Warnings, "no standard eBPF sections found")
	}

	result.Info.InstructionCount = len(data) / 8 // Rough estimate
}

// isELF checks if data starts with ELF magic number
func isELF(data []byte) bool {
	return len(data) >= 4 && data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F'
}

// parseBpftoolOutput extracts information from bpftool output
func (v *Verifier) parseBpftoolOutput(output string, result *VerificationResult) {
	// Count instructions
	lines := strings.Split(output, "\n")
	instructionCount := 0

	instructionPattern := regexp.MustCompile(`^\s*\d+:`)
	for _, line := range lines {
		if instructionPattern.MatchString(line) {
			instructionCount++
		}
	}

	result.Info.InstructionCount = instructionCount

	// Check for loops
	if strings.Contains(output, "back-edge") || strings.Contains(output, "loop") {
		result.Info.LoopDetected = true
		if v.StrictMode {
			result.Valid = false
			result.Errors = append(result.Errors, "loops detected (strict mode)")
		} else {
			result.Warnings = append(result.Warnings, "loops detected - may fail verification in kernel")
		}
	}

	// Check instruction limit
	if instructionCount > v.MaxInstructions {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf(
			"instruction count %d exceeds limit %d",
			instructionCount, v.MaxInstructions,
		))
	}
}

// extractError extracts error message from bpftool output
func extractError(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Error:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Error:"))
		}
	}
	return "unknown error"
}

// ValidateManifest checks if manifest programs match built object
func (v *Verifier) ValidateManifest(objectFile string, programs []string) error {
	// TODO: Implement manifest validation
	// Check that all programs defined in manifest exist in the object file
	return nil
}
