package s3

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/google/uuid"
)

// PluginStorage provides high-level operations for plugin storage
type PluginStorage struct {
	client *Client
}

// NewPluginStorage creates a new plugin storage service
func NewPluginStorage(client *Client) *PluginStorage {
	return &PluginStorage{client: client}
}

// UploadEBPF uploads eBPF object file
func (s *PluginStorage) UploadEBPF(ctx context.Context, pluginID uuid.UUID, data io.Reader, size int64) (string, error) {
	key := path.Join("plugins", pluginID.String(), "ebpf.o")
	
	if err := s.client.Upload(ctx, key, data, size); err != nil {
		return "", fmt.Errorf("failed to upload eBPF object: %w", err)
	}

	return key, nil
}

// UploadWASM uploads WASM module file
func (s *PluginStorage) UploadWASM(ctx context.Context, pluginID uuid.UUID, data io.Reader, size int64) (string, error) {
	key := path.Join("plugins", pluginID.String(), "plugin.wasm")
	
	if err := s.client.Upload(ctx, key, data, size); err != nil {
		return "", fmt.Errorf("failed to upload WASM module: %w", err)
	}

	return key, nil
}

// DownloadEBPF downloads eBPF object file
func (s *PluginStorage) DownloadEBPF(ctx context.Context, s3Key string) (io.ReadCloser, error) {
	return s.client.Download(ctx, s3Key)
}

// DownloadWASM downloads WASM module file
func (s *PluginStorage) DownloadWASM(ctx context.Context, s3Key string) (io.ReadCloser, error) {
	return s.client.Download(ctx, s3Key)
}

// DeletePlugin deletes all plugin artifacts
func (s *PluginStorage) DeletePlugin(ctx context.Context, pluginID uuid.UUID) error {
	prefix := path.Join("plugins", pluginID.String())
	
	objects, err := s.client.List(ctx, prefix)
	if err != nil {
		return fmt.Errorf("failed to list plugin objects: %w", err)
	}

	for _, key := range objects {
		if err := s.client.Delete(ctx, key); err != nil {
			return fmt.Errorf("failed to delete object %s: %w", key, err)
		}
	}

	return nil
}

// PluginExists checks if plugin artifacts exist
func (s *PluginStorage) PluginExists(ctx context.Context, pluginID uuid.UUID) (bool, error) {
	ebpfKey := path.Join("plugins", pluginID.String(), "ebpf.o")
	wasmKey := path.Join("plugins", pluginID.String(), "plugin.wasm")

	ebpfExists, err := s.client.Exists(ctx, ebpfKey)
	if err != nil {
		return false, err
	}

	wasmExists, err := s.client.Exists(ctx, wasmKey)
	if err != nil {
		return false, err
	}

	return ebpfExists && wasmExists, nil
}
