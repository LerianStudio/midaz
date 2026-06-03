// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package storage provides object storage adapters for templates and reports.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
)

// S3Config contains configuration for S3-compatible storage.
// Works with AWS S3, MinIO, SeaweedFS S3, and other S3-compatible services.
type S3Config struct {
	Endpoint        string // For SeaweedFS: http://localhost:8333, for MinIO: http://localhost:9000
	Region          string // Default: us-east-1
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool // Required for SeaweedFS/MinIO
	DisableSSL      bool
}

// DefaultSeaweedS3Config returns a configuration suitable for local SeaweedFS S3 development.
func DefaultSeaweedS3Config(bucket string) S3Config {
	return S3Config{
		Endpoint:     "http://localhost:8333",
		Region:       "us-east-1",
		Bucket:       bucket,
		UsePathStyle: true,
		DisableSSL:   true,
	}
}

// S3Client provides S3-compatible object storage operations.
type S3Client struct {
	s3     *s3.Client
	bucket string
}

var (
	// ErrBucketRequired indicates bucket name is missing.
	ErrBucketRequired = constant.ErrBucketRequired
	// ErrKeyRequired indicates object key is missing.
	ErrKeyRequired = constant.ErrObjectKeyRequired
	// ErrObjectNotFound indicates the object does not exist.
	ErrObjectNotFound = constant.ErrObjectNotFound
	// ErrTTLNotSupported indicates TTL is not supported by S3 (use lifecycle policies instead).
	ErrTTLNotSupported = constant.ErrTTLNotSupported
)

// NewS3Client creates a new S3 client with the given configuration.
func NewS3Client(ctx context.Context, cfg S3Config) (*S3Client, error) {
	if cfg.Bucket == "" {
		return nil, ErrBucketRequired
	}

	var opts []func(*config.LoadOptions) error

	if cfg.Region != "" {
		opts = append(opts, config.WithRegion(cfg.Region))
	}

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading aws config: %w", err)
	}

	clientOpts := []func(*s3.Options){}

	if cfg.Endpoint != "" {
		endpoint := normalizeEndpointScheme(cfg.Endpoint, cfg.DisableSSL)
		if endpoint != "" {
			clientOpts = append(clientOpts, func(o *s3.Options) {
				o.BaseEndpoint = aws.String(endpoint)
			})
		}
	}

	if cfg.UsePathStyle {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	s3Client := s3.NewFromConfig(awsCfg, clientOpts...)

	return &S3Client{
		s3:     s3Client,
		bucket: cfg.Bucket,
	}, nil
}

func normalizeEndpointScheme(endpoint string, disableSSL bool) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return trimmed
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return trimmed
	}

	if idx := strings.Index(trimmed, "://"); idx != -1 {
		trimmed = strings.TrimLeft(trimmed[idx+3:], "/")
		if trimmed == "" {
			return ""
		}
	}

	if disableSSL {
		return "http://" + trimmed
	}

	return "https://" + trimmed
}

// Upload stores content from a reader at the given key.
func (client *S3Client) Upload(ctx context.Context, key string, reader io.Reader, contentType string) (string, error) {
	return client.UploadWithTTL(ctx, key, reader, contentType, "")
}

// UploadWithTTL stores content with a time-to-live.
// Note: S3 does not support per-object TTL via upload parameters.
// TTL parameter is ignored - use S3 bucket lifecycle policies instead.
// This method exists for interface compatibility with SeaweedFS.
func (client *S3Client) UploadWithTTL(ctx context.Context, key string, reader io.Reader, contentType string, ttl string) (string, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)
	ctx, span := tracer.Start(ctx, "repository.storage.upload")

	defer span.End()

	if key == "" {
		return "", ErrKeyRequired
	}

	// Log warning if TTL is provided (not supported in S3)
	if ttl != "" && logger != nil {
		logger.Log(ctx, log.LevelWarn, "TTL parameter ignored for S3 storage - configure bucket lifecycle policies instead", log.String("ttl", ttl))
	}

	// Read all data into memory (required for S3 SDK)
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("reading data: %w", err)
	}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(client.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	if _, err := client.s3.PutObject(ctx, input); err != nil {
		libOpentelemetry.HandleSpanError(span, "failed to upload object", err)

		if logger != nil {
			logger.Log(ctx, log.LevelError, "Failed to upload object", log.String("key", key), log.Err(err))
		}

		return "", fmt.Errorf("uploading object: %w", err)
	}

	if logger != nil {
		logger.Log(ctx, log.LevelInfo, "Uploaded object", log.String("key", key), log.String("bucket", client.bucket))
	}

	return key, nil
}

// Download retrieves content from the given key.
func (client *S3Client) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)
	ctx, span := tracer.Start(ctx, "repository.storage.download")

	defer span.End()

	if key == "" {
		return nil, ErrKeyRequired
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	}

	result, err := client.s3.GetObject(ctx, input)
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			libOpentelemetry.HandleSpanError(span, "object not found in storage", err)

			if logger != nil {
				logger.Log(ctx, log.LevelWarn, "Object not found in storage",
					log.String("key", key),
					log.String("bucket", client.bucket),
				)
			}

			return nil, ErrObjectNotFound
		}

		libOpentelemetry.HandleSpanError(span, "failed to download object", err)

		if logger != nil {
			logger.Log(ctx, log.LevelError, "Failed to download object", log.String("key", key), log.Err(err))
		}

		return nil, fmt.Errorf("downloading object: %w", err)
	}

	return result.Body, nil
}

// Delete removes an object by key.
func (client *S3Client) Delete(ctx context.Context, key string) error {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)
	ctx, span := tracer.Start(ctx, "repository.storage.delete")

	defer span.End()

	if key == "" {
		return ErrKeyRequired
	}

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	}

	if _, err := client.s3.DeleteObject(ctx, input); err != nil {
		libOpentelemetry.HandleSpanError(span, "failed to delete object", err)

		if logger != nil {
			logger.Log(ctx, log.LevelError, "Failed to delete object", log.String("key", key), log.Err(err))
		}

		return fmt.Errorf("deleting object: %w", err)
	}

	if logger != nil {
		logger.Log(ctx, log.LevelInfo, "Deleted object", log.String("key", key), log.String("bucket", client.bucket))
	}

	return nil
}

// GeneratePresignedURL creates a time-limited download URL.
func (client *S3Client) GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)
	ctx, span := tracer.Start(ctx, "repository.storage.generate_presigned_url")

	defer span.End()

	if key == "" {
		return "", ErrKeyRequired
	}

	presigner := s3.NewPresignClient(client.s3)

	input := &s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	}

	result, err := presigner.PresignGetObject(ctx, input, s3.WithPresignExpires(expiry))
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "failed to generate presigned url", err)

		if logger != nil {
			logger.Log(ctx, log.LevelError, "Failed to generate presigned URL", log.String("key", key), log.Err(err))
		}

		return "", fmt.Errorf("generating presigned url: %w", err)
	}

	return result.URL, nil
}

// Exists checks if an object exists at the given key.
func (client *S3Client) Exists(ctx context.Context, key string) (bool, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)
	ctx, span := tracer.Start(ctx, "repository.storage.exists")

	defer span.End()

	if key == "" {
		return false, ErrKeyRequired
	}

	input := &s3.HeadObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	}

	if _, err := client.s3.HeadObject(ctx, input); err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return false, nil
		}

		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}

		libOpentelemetry.HandleSpanError(span, "failed to check object existence", err)

		if logger != nil {
			logger.Log(ctx, log.LevelError, "Failed to check object existence", log.String("key", key), log.Err(err))
		}

		return false, fmt.Errorf("checking object existence: %w", err)
	}

	return true, nil
}

// Compile-time interface check.
var _ ObjectStorage = (*S3Client)(nil)
