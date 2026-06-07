// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
	constant "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/seaweedfs"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/storage"

	tmS3 "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/s3"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// Repository provides an interface for storage operations
//
//go:generate mockgen --destination=template.mock.go --package=template --copyright_file=../../../COPYRIGHT . Repository
type Repository interface {
	Get(ctx context.Context, objectName string) ([]byte, error)
	Put(ctx context.Context, objectName string, contentType string, data []byte) error
}

// StorageRepository provides access to object storage for template operations.
type StorageRepository struct {
	storage storage.ObjectStorage
}

// Compile-time interface satisfaction check.
var _ Repository = (*StorageRepository)(nil)

// NewStorageRepository creates a new instance of StorageRepository with the given storage client.
func NewStorageRepository(storageClient storage.ObjectStorage) *StorageRepository {
	return &StorageRepository{
		storage: storageClient,
	}
}

// Get the content of a .tpl file from the storage.
// objectName can be passed with or without .tpl extension - it will be normalized.
func (repo *StorageRepository) Get(ctx context.Context, objectName string) ([]byte, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template_storage.get")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	// Normalize: ensure .tpl extension (handles both "uuid" and "uuid.tpl")
	objectName = strings.TrimSuffix(objectName, ".tpl")
	// Add templates prefix, then apply tenant prefix if in multi-tenant mode
	baseKey := fmt.Sprintf("templates/%s.tpl", objectName)

	key, err := tmS3.GetS3KeyStorageContext(ctx, baseKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant key prefix for template download", err)

		return nil, fmt.Errorf("failed to resolve storage key for template: %w", err)
	}

	// Validate that the resolved key belongs to the authenticated tenant.
	// This prevents cross-tenant object access if context propagation fails.
	if err := seaweedfs.ValidateKeyForTenant(ctx, key); err != nil {
		libOpentelemetry.HandleSpanError(span, "S3 key-prefix validation failed for template download", err)

		return nil, fmt.Errorf("storage key validation failed for template: %w", err)
	}

	logger.Log(ctx, log.LevelInfo, "Getting template from storage",
		log.String("object_name", objectName),
		log.String("resolved_s3_key", key),
	)

	reader, err := repo.storage.Download(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			libOpentelemetry.HandleSpanError(span, "Template not found in storage — verify tenant prefix in S3 key", err)
			logger.Log(ctx, log.LevelError, "Template not found in storage — check if tenant prefix matches upload path",
				log.String("object_name", objectName),
				log.String("resolved_s3_key", key),
			)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to download template from storage", err)
			logger.Log(ctx, log.LevelError, "Failed to download template from storage",
				log.String("object_name", objectName),
				log.String("resolved_s3_key", key),
				log.Err(err),
			)
		}

		return nil, pkg.ValidateBusinessError(constant.ErrCommunicateSeaweedFS, "")
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read template data", err)

		return nil, pkg.ValidateBusinessError(constant.ErrCommunicateSeaweedFS, "")
	}

	return data, nil
}

// Put uploads data to the storage with the given object name and content type.
// objectName can be passed with or without .tpl extension - it will be normalized.
func (repo *StorageRepository) Put(ctx context.Context, objectName string, contentType string, data []byte) error {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template_storage.put")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	// Normalize: ensure .tpl extension (handles both "uuid" and "uuid.tpl")
	objectName = strings.TrimSuffix(objectName, ".tpl")
	// Add templates prefix, then apply tenant prefix if in multi-tenant mode
	baseKey := fmt.Sprintf("templates/%s.tpl", objectName)

	key, err := tmS3.GetS3KeyStorageContext(ctx, baseKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant key prefix for template upload", err)

		return fmt.Errorf("failed to resolve storage key for template: %w", err)
	}

	// Validate that the resolved key belongs to the authenticated tenant.
	// This prevents cross-tenant object access if context propagation fails.
	if err := seaweedfs.ValidateKeyForTenant(ctx, key); err != nil {
		libOpentelemetry.HandleSpanError(span, "S3 key-prefix validation failed for template upload", err)

		return fmt.Errorf("storage key validation failed for template: %w", err)
	}

	// Log with objectName (not the tenant-prefixed key) to avoid leaking tenant IDs in logs.
	logger.Log(ctx, log.LevelInfo, "Putting template to storage", log.String("object_name", objectName))

	_, err = repo.storage.Upload(ctx, key, bytes.NewReader(data), contentType)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to upload template to storage", err)
		logger.Log(ctx, log.LevelError, "Error communicating with storage", log.Err(err))

		return pkg.ValidateBusinessError(constant.ErrCommunicateSeaweedFS, "")
	}

	return nil
}
