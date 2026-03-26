package s3

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
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

	// Load default config
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	client := s3.NewFromConfig(awsCfg,
		func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		},
	)

	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// Upload uploads a file to S3
func (c *Client) Upload(ctx context.Context, key string, reader io.Reader, size int64) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.config.BucketName),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(size),
	})

	if err != nil {
		return fmt.Errorf("failed to upload object: %w", err)
	}

	return nil
}

// Download downloads a file from S3
func (c *Client) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.config.BucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	return result.Body, nil
}

// Delete deletes a file from S3
func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.config.BucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

// Exists checks if a file exists in S3
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.config.BucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		// Check if it's a 404 error
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to head object: %w", err)
	}

	return true, nil
}

// List lists objects with a given prefix
func (c *Client) List(ctx context.Context, prefix string) ([]string, error) {
	result, err := c.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.config.BucketName),
		Prefix: aws.String(prefix),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	keys := make([]string, 0, len(result.Contents))
	for _, obj := range result.Contents {
		keys = append(keys, *obj.Key)
	}

	return keys, nil
}

// Health checks S3 connectivity
func (c *Client) Health(ctx context.Context) error {
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.config.BucketName),
	})

	if err != nil {
		return fmt.Errorf("failed to head bucket: %w", err)
	}

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
