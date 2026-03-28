package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/google/uuid"
)

// Config holds S3 configuration
type Config struct {
	Endpoint   string
	Region     string
	AccessKey  string
	SecretKey  string
	BucketName string
	UseSSL     bool
}

// DefaultConfig returns default configuration for Garage S3
func DefaultConfig() *Config {
	return &Config{
		Endpoint:   "http://127.0.0.1:3900",
		Region:     "garage",
		AccessKey:  "",
		SecretKey:  "",
		BucketName: "epbf-plugins",
		UseSSL:     false,
	}
}

// Client wraps S3 client
type Client struct {
	client *s3.Client
	config *Config
}

// NewClient creates a new S3 client
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	logger.Debug("Creating S3 client",
		"endpoint", cfg.Endpoint,
		"region", cfg.Region,
		"bucket", cfg.BucketName,
		"use_ssl", cfg.UseSSL)

	// Create S3 client with static credentials (or empty for Garage)
	client := s3.NewFromConfig(aws.Config{
		Region: cfg.Region,
		Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			creds := aws.Credentials{
				AccessKeyID:     cfg.AccessKey,
				SecretAccessKey: cfg.SecretKey,
			}
			if cfg.AccessKey != "" {
				logger.Debug("Using S3 credentials", "access_key_id", cfg.AccessKey[:8]+"...")
			} else {
				logger.Debug("Using anonymous S3 credentials (Garage mode)")
			}
			return creds, nil
		}),
	},
		func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
			logger.Debug("S3 client configured", "base_endpoint", cfg.Endpoint, "path_style", true)
		},
	)

	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// Upload uploads a file to S3
func (c *Client) Upload(ctx context.Context, key string, reader io.Reader, size int64) error {
	logger.Debug("Uploading to S3", "key", key, "size", size, "bucket", c.config.BucketName)

	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.config.BucketName),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(size),
	})

	if err != nil {
		logger.Error("Failed to upload to S3",
			"key", key,
			"bucket", c.config.BucketName,
			"error", err.Error(),
			"error_type", fmt.Sprintf("%T", err))
		return fmt.Errorf("failed to upload object: %w", err)
	}

	logger.Info("✅ Uploaded to S3", "key", key, "size", size)
	return nil
}

// Download downloads a file from S3
func (c *Client) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	logger.Debug("Downloading from S3", "key", key, "bucket", c.config.BucketName)

	result, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.config.BucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		logger.Error("Failed to get object from S3",
			"key", key,
			"bucket", c.config.BucketName,
			"error", err.Error())
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	logger.Debug("Downloaded from S3", "key", key)
	return result.Body, nil
}

// Delete deletes a file from S3
func (c *Client) Delete(ctx context.Context, key string) error {
	logger.Debug("Deleting from S3", "key", key, "bucket", c.config.BucketName)

	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.config.BucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		logger.Error("Failed to delete from S3",
			"key", key,
			"bucket", c.config.BucketName,
			"error", err.Error())
		return fmt.Errorf("failed to delete object: %w", err)
	}

	logger.Info("✅ Deleted from S3", "key", key)
	return nil
}

// Exists checks if a file exists in S3
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	logger.Debug("Checking if object exists", "key", key, "bucket", c.config.BucketName)

	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.config.BucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		if isNotFound(err) {
			logger.Debug("Object not found", "key", key)
			return false, nil
		}
		logger.Error("Failed to check object existence",
			"key", key,
			"bucket", c.config.BucketName,
			"error", err.Error())
		return false, fmt.Errorf("failed to head object: %w", err)
	}

	logger.Debug("Object exists", "key", key)
	return true, nil
}

// List lists objects with a given prefix
func (c *Client) List(ctx context.Context, prefix string) ([]string, error) {
	logger.Debug("Listing objects in S3", "prefix", prefix, "bucket", c.config.BucketName)

	result, err := c.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.config.BucketName),
		Prefix: aws.String(prefix),
	})

	if err != nil {
		logger.Error("Failed to list objects",
			"prefix", prefix,
			"bucket", c.config.BucketName,
			"error", err.Error())
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	keys := make([]string, 0, len(result.Contents))
	for _, obj := range result.Contents {
		keys = append(keys, *obj.Key)
	}

	logger.Debug("Listed objects", "count", len(keys), "prefix", prefix)
	return keys, nil
}

// Health checks S3 connectivity
func (c *Client) Health(ctx context.Context) error {
	logger.Debug("Checking S3 health", "bucket", c.config.BucketName, "endpoint", c.config.Endpoint)

	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.config.BucketName),
	})

	if err != nil {
		logger.Error("S3 health check failed",
			"bucket", c.config.BucketName,
			"endpoint", c.config.Endpoint,
			"error", err.Error(),
			"error_type", fmt.Sprintf("%T", err))
		return fmt.Errorf("failed to head bucket: %w", err)
	}

	logger.Info("✅ S3 health check passed", "bucket", c.config.BucketName)
	return nil
}

// isNotFound checks if error is 404
func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "404"
	}
	return false
}

// PluginStorage provides high-level operations for plugin storage
type PluginStorage struct {
	client *Client
}

// NewPluginStorage creates a new plugin storage service
func NewPluginStorage(client *Client) *PluginStorage {
	logger.Info("Plugin storage initialized", "bucket", client.config.BucketName)
	return &PluginStorage{client: client}
}

// UploadEBPF uploads eBPF object file
func (s *PluginStorage) UploadEBPF(ctx context.Context, pluginID uuid.UUID, data io.Reader, size int64) (string, error) {
	key := fmt.Sprintf("plugins/%s/ebpf.o", pluginID.String())
	logger.Info("Uploading eBPF object", "plugin_id", pluginID.String(), "key", key, "size", size)

	if err := s.client.Upload(ctx, key, data, size); err != nil {
		logger.Error("Failed to upload eBPF object",
			"plugin_id", pluginID.String(),
			"key", key,
			"error", err.Error())
		return "", err
	}

	logger.Info("✅ eBPF object uploaded", "plugin_id", pluginID.String(), "key", key)
	return key, nil
}

// UploadWASM uploads WASM module file
func (s *PluginStorage) UploadWASM(ctx context.Context, pluginID uuid.UUID, data io.Reader, size int64) (string, error) {
	key := fmt.Sprintf("plugins/%s/plugin.wasm", pluginID.String())
	logger.Info("Uploading WASM module", "plugin_id", pluginID.String(), "key", key, "size", size)

	if err := s.client.Upload(ctx, key, data, size); err != nil {
		logger.Error("Failed to upload WASM module",
			"plugin_id", pluginID.String(),
			"key", key,
			"error", err.Error())
		return "", err
	}

	logger.Info("✅ WASM module uploaded", "plugin_id", pluginID.String(), "key", key)
	return key, nil
}

// DownloadEBPF downloads eBPF object file
func (s *PluginStorage) DownloadEBPF(ctx context.Context, s3Key string) (io.ReadCloser, error) {
	logger.Debug("Downloading eBPF object", "key", s3Key)
	return s.client.Download(ctx, s3Key)
}

// DownloadWASM downloads WASM module file
func (s *PluginStorage) DownloadWASM(ctx context.Context, s3Key string) (io.ReadCloser, error) {
	logger.Debug("Downloading WASM module", "key", s3Key)
	return s.client.Download(ctx, s3Key)
}

// DeletePlugin deletes all plugin artifacts
func (s *PluginStorage) DeletePlugin(ctx context.Context, pluginID uuid.UUID) error {
	logger.Info("Deleting plugin artifacts", "plugin_id", pluginID.String())

	prefix := fmt.Sprintf("plugins/%s", pluginID.String())
	objects, err := s.client.List(ctx, prefix)
	if err != nil {
		logger.Error("Failed to list plugin objects for deletion",
			"plugin_id", pluginID.String(),
			"prefix", prefix,
			"error", err.Error())
		return err
	}

	logger.Debug("Found objects to delete", "count", len(objects), "prefix", prefix)

	for _, key := range objects {
		if err := s.client.Delete(ctx, key); err != nil {
			logger.Error("Failed to delete object",
				"plugin_id", pluginID.String(),
				"key", key,
				"error", err.Error())
			return err
		}
	}

	logger.Info("✅ Deleted all plugin artifacts", "plugin_id", pluginID.String(), "count", len(objects))
	return nil
}

// PluginExists checks if plugin artifacts exist
func (s *PluginStorage) PluginExists(ctx context.Context, pluginID uuid.UUID) (bool, error) {
	ebpfKey := fmt.Sprintf("plugins/%s/ebpf.o", pluginID.String())
	wasmKey := fmt.Sprintf("plugins/%s/plugin.wasm", pluginID.String())

	logger.Debug("Checking plugin existence", "plugin_id", pluginID.String())

	ebpfExists, err := s.client.Exists(ctx, ebpfKey)
	if err != nil {
		logger.Error("Failed to check eBPF object existence",
			"plugin_id", pluginID.String(),
			"key", ebpfKey,
			"error", err.Error())
		return false, err
	}

	wasmExists, err := s.client.Exists(ctx, wasmKey)
	if err != nil {
		logger.Error("Failed to check WASM object existence",
			"plugin_id", pluginID.String(),
			"key", wasmKey,
			"error", err.Error())
		return false, err
	}

	exists := ebpfExists && wasmExists
	logger.Debug("Plugin existence check",
		"plugin_id", pluginID.String(),
		"ebpf_exists", ebpfExists,
		"wasm_exists", wasmExists,
		"exists", exists)

	return exists, nil
}

// Upload uploads a file to S3 with retry
func (c *Client) UploadWithRetry(ctx context.Context, key string, data []byte, maxRetries int) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.Debug("S3 upload attempt", "key", key, "attempt", attempt, "max_retries", maxRetries)

		reader := bytes.NewReader(data)
		err := c.Upload(ctx, key, reader, int64(len(data)))
		if err == nil {
			logger.Info("✅ S3 upload succeeded", "key", key, "attempt", attempt)
			return nil
		}

		lastErr = err
		logger.Warn("S3 upload failed, retrying...",
			"key", key,
			"attempt", attempt,
			"error", err.Error())
	}

	logger.Error("❌ S3 upload failed after all retries",
		"key", key,
		"max_retries", maxRetries,
		"last_error", lastErr.Error())
	return fmt.Errorf("upload failed after %d attempts: %w", maxRetries, lastErr)
}
