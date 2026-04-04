package builder

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// BuildResult holds the result of a plugin build
type BuildResult struct {
	Success  bool
	EBPFFile string
	BuildLog string
	Duration time.Duration
}

// BuildError represents a build error with logs
type BuildError struct {
	error
	BuildLog string
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("build failed: %v", e.error)
}

// Builder handles plugin building in Docker containers
type Builder struct {
	dockerClient *client.Client
	imageName    string
}

// NewBuilder creates a new plugin builder
func NewBuilder(dockerClient *client.Client, imageName string) (*Builder, error) {
	if imageName == "" {
		imageName = "epbf-monitor-builder:latest"
	}

	return &Builder{
		dockerClient: dockerClient,
		imageName:    imageName,
	}, nil
}

// Build compiles a plugin from source
func (b *Builder) Build(ctx context.Context, pluginDir, pluginName string) (*BuildResult, error) {
	startTime := time.Now()
	result := &BuildResult{}

	var logBuffer bytes.Buffer
	logger := &logWriter{&logBuffer}

	// Create build container
	containerName := fmt.Sprintf("epbf-build-%s-%d", pluginName, time.Now().UnixNano())

	// Prepare mount paths
	hostSourceDir := pluginDir
	containerSourceDir := "/workspace/plugin"
	containerOutputDir := "" // Not mounting output, will use docker cp

	// Create output directory
	outputDir := filepath.Join(pluginDir, "build")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create build script
	buildScript := `
set -e
echo "🔨 Building plugin..."
cd /workspace/plugin

# Create temp output directory
mkdir -p /tmp/epbf-build-output

# Build eBPF
echo "📦 Building eBPF program..."
if [ -f ebpf/Makefile ]; then
    make -C ebpf
    cp ebpf/build/program.o /tmp/epbf-build-output/program.o
elif [ -f ebpf/main.c ]; then
    clang -O2 -g -Wall -Wextra \
        -target bpf \
        -D__TARGET_ARCH_x86_64 \
        -I/usr/include \
        -c ebpf/main.c \
        -o /tmp/epbf-build-output/program.o
    echo "✅ eBPF: /tmp/epbf-build-output/program.o"
else
    echo "❌ No eBPF source found"
    exit 1
fi

# Copy artifacts to temp directory (docker cp will copy to host)
echo "📊 Build artifacts:"
ls -lh /tmp/epbf-build-output/
echo "✅ Build complete!"
`

	// Container configuration
	config := &container.Config{
		Image:        b.imageName,
		Cmd:          []string{"sh", "-c", buildScript},
		WorkingDir:   containerSourceDir,
		AttachStdout: true,
		AttachStderr: true,
		// Run as root to avoid permission issues with bind mounts on macOS
		// User:         "builder",
		Env: []string{
			"PLUGIN_NAME=" + pluginName,
			"OUTPUT_DIR=" + containerOutputDir,
		},
	}

	hostConfig := &container.HostConfig{
		AutoRemove: false, // Need to keep container alive for docker cp
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   hostSourceDir,
				Target:   containerSourceDir,
				ReadOnly: false,
			},
		},
		Resources: container.Resources{
			Memory:   512 * 1024 * 1024, // 512MB limit
			NanoCPUs: 1000000000,         // 1 CPU
		},
	}

	// Create container
	resp, err := b.dockerClient.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Ensure container removal
	defer func() {
		_ = b.dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
	}()

	// Start container
	if err := b.dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Get logs
	logsReader, err := b.dockerClient.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}
	defer logsReader.Close()

	// Copy logs to buffer
	_, err = stdcopy.StdCopy(logger, logger, logsReader)
	if err != nil && err != io.EOF {
		fmt.Printf("Warning: log copy error: %v\n", err)
	}

	// Wait for container to finish
	statusCh, errCh := b.dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			result.Success = false
			result.BuildLog = logBuffer.String()
			return result, &BuildError{
				error:    fmt.Errorf("container wait error: %w", err),
				BuildLog: logBuffer.String(),
			}
		}
	case status := <-statusCh:
		result.BuildLog = logBuffer.String()
		result.Duration = time.Since(startTime)

		if status.StatusCode != 0 {
			result.Success = false
			return result, &BuildError{
				error:    fmt.Errorf("build failed with exit code %d", status.StatusCode),
				BuildLog: logBuffer.String(),
			}
		}
	}

	// Copy artifacts from container using docker cp (avoids macOS bind mount issues)
	ebpfFile := filepath.Join(outputDir, "program.o")

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return result, &BuildError{error: fmt.Errorf("failed to create output directory: %w", err), BuildLog: logBuffer.String()}
	}

	// Copy eBPF object
	ebpfData, err := copyFromContainer(ctx, b.dockerClient, resp.ID, "/tmp/epbf-build-output/program.o")
	if err != nil {
		result.Success = false
		return result, &BuildError{error: fmt.Errorf("failed to copy eBPF from container: %w", err), BuildLog: logBuffer.String()}
	}
	if err := os.WriteFile(ebpfFile, ebpfData, 0644); err != nil {
		result.Success = false
		return result, &BuildError{error: fmt.Errorf("failed to write eBPF file: %w", err), BuildLog: logBuffer.String()}
	}

	result.Success = true
	result.EBPFFile = ebpfFile

	// Cleanup container
	_ = b.dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	return result, nil
}

// BuildInPlace builds plugin without Docker (for development)
func (b *Builder) BuildInPlace(ctx context.Context, pluginDir string) (*BuildResult, error) {
	startTime := time.Now()
	result := &BuildResult{}

	buildDir := filepath.Join(pluginDir, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create build directory: %w", err)
	}

	// TODO: Implement local build using clang directly
	// For now, return placeholder
	result.Success = true
	result.EBPFFile = filepath.Join(buildDir, "program.o")
	result.Duration = time.Since(startTime)
	result.BuildLog = "Local build not yet implemented, using placeholder"

	return result, nil
}

// logWriter implements io.Writer for logging
type logWriter struct {
	buffer *bytes.Buffer
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	return w.buffer.Write(p)
}

// ContainerInfo holds Docker container information
type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	Status  string
	Created time.Time
}

// ListBuilderImages lists available builder images
func (b *Builder) ListBuilderImages(ctx context.Context) ([]string, error) {
	images, err := b.dockerClient.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", "epbf-monitor-builder:*")),
	})
	if err != nil {
		return nil, err
	}

	imageNames := make([]string, 0, len(images))
	for _, img := range images {
		for _, tag := range img.RepoTags {
			imageNames = append(imageNames, tag)
		}
	}

	return imageNames, nil
}

// PullBuilderImage pulls the builder image
func (b *Builder) PullBuilderImage(ctx context.Context) error {
	reader, err := b.dockerClient.ImagePull(ctx, b.imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	// Read pull output
	decoder := json.NewDecoder(reader)
	for {
		var msg map[string]interface{}
		if err := decoder.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		// Log pull progress if needed
	}

	return nil
}

// BuildImage builds the builder Docker image
func (b *Builder) BuildImage(ctx context.Context, dockerfilePath string) error {
	buildCtx, err := createTarContext(filepath.Dir(dockerfilePath))
	if err != nil {
		return err
	}
	defer buildCtx.Close()

	resp, err := b.dockerClient.ImageBuild(ctx, buildCtx, types.ImageBuildOptions{
		Dockerfile: filepath.Base(dockerfilePath),
		Tags:       []string{b.imageName},
		Remove:     true,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read build output
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

// createTarContext creates a tar archive for Docker build context
func createTarContext(dir string) (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip .git and other directories
			if info.IsDir() && (info.Name() == ".git" || strings.HasPrefix(info.Name(), ".")) {
				return nil
			}

			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}

			relPath, _ := filepath.Rel(dir, path)
			header.Name = relPath

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if !info.IsDir() {
				data, err := os.Open(path)
				if err != nil {
					return err
				}
				defer data.Close()

				if _, err := io.Copy(tw, data); err != nil {
					return err
				}
			}

			return nil
		})
		tw.Close()
		pw.Close()
	}()
	return pr, nil
}

// extractTarContent extracts file content from tar archive
func extractTarContent(tarData []byte) []byte {
	// Docker cp returns tar archive, extract first file
	tr := tar.NewReader(bytes.NewReader(tarData))
	header, err := tr.Next()
	if err != nil {
		return tarData // Return original if not tar
	}
	
	content := make([]byte, header.Size)
	if _, err := io.ReadFull(tr, content); err != nil {
		return tarData
	}
	return content
}

// copyFromContainer copies a file from a Docker container and returns its content
func copyFromContainer(ctx context.Context, client *client.Client, containerID, path string) ([]byte, error) {
	reader, _, err := client.CopyFromContainer(ctx, containerID, path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Read tar archive
	tarData, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	// Extract file content from tar
	return extractTarContent(tarData), nil
}
