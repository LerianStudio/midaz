// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel/attribute"
)

const (
	// maxBulkMetadataEntries is the maximum number of entries allowed in a single bulk operation.
	// This prevents resource exhaustion from unbounded input.
	maxBulkMetadataEntries = 10000
)

// MetadataEntry represents a single metadata entry for bulk operations.
// It encapsulates the entity ID, collection name, and metadata data.
type MetadataEntry struct {
	EntityID   string
	Collection string
	Data       map[string]any
}

// Validate checks that the MetadataEntry has valid fields.
// Returns an error if EntityID is empty/invalid or Collection is empty.
func (e MetadataEntry) Validate() error {
	if e.EntityID == "" {
		return fmt.Errorf("entity ID is required")
	}

	if _, err := uuid.Parse(e.EntityID); err != nil {
		return fmt.Errorf("invalid entity ID format: %w", err)
	}

	if e.Collection == "" {
		return fmt.Errorf("collection is required")
	}

	return nil
}

// createMetadataBulk creates metadata entries in bulk, grouped by collection.
// For single entries per collection, it uses Create directly (optimization).
// On bulk failure, it falls back to individual Create calls with graceful degradation.
//
// Returns nil if all entries were created successfully (or were empty).
// Returns error if any entries failed to create after fallback.
func (uc *UseCase) createMetadataBulk(ctx context.Context, entries []MetadataEntry) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_metadata_bulk")
	defer span.End()

	// Check input size limit to prevent resource exhaustion
	if len(entries) > maxBulkMetadataEntries {
		err := fmt.Errorf("bulk metadata entries exceed limit: %d > %d", len(entries), maxBulkMetadataEntries)
		libOpentelemetry.HandleSpanError(span, "Input size exceeds limit", err)

		return err
	}

	// Filter and validate entries
	validEntries := make([]MetadataEntry, 0, len(entries))

	for i, entry := range entries {
		// Skip entries with nil Data
		if entry.Data == nil {
			continue
		}

		// Validate entry fields
		if err := entry.Validate(); err != nil {
			libOpentelemetry.HandleSpanError(span, fmt.Sprintf("Invalid entry at index %d", i), err)

			return fmt.Errorf("invalid metadata entry at index %d: %w", i, err)
		}

		validEntries = append(validEntries, entry)
	}

	if len(validEntries) == 0 {
		return nil
	}

	// Group entries by collection
	grouped := groupMetadataByCollection(validEntries)

	var (
		totalFailures  int
		totalAttempted int
	)

	for collection, collectionEntries := range grouped {
		totalAttempted += len(collectionEntries)

		failures, err := uc.createMetadataForCollection(ctx, logger, collection, collectionEntries)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, fmt.Sprintf("Failed to create metadata for collection %s", collection), err)
		}

		totalFailures += failures
	}

	if totalFailures > 0 {
		return fmt.Errorf("failed to create %d of %d metadata entries", totalFailures, totalAttempted)
	}

	return nil
}

// createMetadataForCollection creates metadata entries for a single collection.
// Uses bulk insert for multiple entries, direct Create for single entry.
// Returns (failureCount, error) where failureCount is the number of entries that failed.
func (uc *UseCase) createMetadataForCollection(
	ctx context.Context,
	logger libLog.Logger,
	collection string,
	entries []MetadataEntry,
) (int, error) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // consistent with codebase pattern

	ctx, span := tracer.Start(ctx, "command.create_metadata_for_collection")
	defer span.End()

	// Add span attributes for observability
	span.SetAttributes(
		attribute.String("collection", collection),
		attribute.Int("entry_count", len(entries)),
	)

	if len(entries) == 0 {
		return 0, nil
	}

	// For single entry, use Create directly (optimization)
	if len(entries) == 1 {
		if err := uc.createSingleMetadata(ctx, logger, collection, entries[0]); err != nil {
			return 1, err
		}

		return 0, nil
	}

	// Convert to MongoDB metadata format
	metadataList := make([]*mongodb.Metadata, 0, len(entries))

	for _, entry := range entries {
		metadataList = append(metadataList, &mongodb.Metadata{
			EntityID:   entry.EntityID,
			EntityName: collection,
			Data:       entry.Data,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		})
	}

	// Try bulk insert
	result, err := uc.TransactionMetadataRepo.CreateBulk(ctx, collection, metadataList)
	if err != nil {
		// Classify the error: infrastructure errors (timeout, network, server unavailable)
		// should not fan out into individual writes that will all fail too.
		if isInfrastructureError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Bulk insert failed with infrastructure error, skipping fallback", err)

			if logger != nil {
				logger.Log(ctx, libLog.LevelError, fmt.Sprintf(
					"Bulk metadata insert failed for %s with infrastructure error (no fallback): %v",
					collection, err,
				))
			}

			return len(entries), fmt.Errorf("infrastructure error during bulk insert for %s: %w", collection, err)
		}

		// Document-level errors (duplicate key, validation) — fall back to individual creates.
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Bulk insert failed, falling back to individual creates", err)

		if logger != nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Bulk metadata insert failed for %s, using fallback: %v", collection, err))
		}

		return uc.fallbackToIndividualMetadataCreate(ctx, logger, collection, entries)
	}

	if logger != nil {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
			"Bulk inserted metadata for %s: attempted=%d, inserted=%d, matched=%d",
			collection, result.Attempted, result.Inserted, result.Matched,
		))
	}

	return 0, nil
}

// createSingleMetadata creates a single metadata entry using the standard Create method.
func (uc *UseCase) createSingleMetadata(
	ctx context.Context,
	logger libLog.Logger,
	collection string,
	entry MetadataEntry,
) error {
	meta := &mongodb.Metadata{
		EntityID:   entry.EntityID,
		EntityName: collection,
		Data:       entry.Data,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := uc.TransactionMetadataRepo.Create(ctx, collection, meta); err != nil {
		if logger != nil {
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create metadata for %s entity %s: %v", collection, entry.EntityID, err))
		}

		return err
	}

	return nil
}

// fallbackToIndividualMetadataCreate creates metadata entries one by one.
// Returns (failureCount, error) where failureCount is the number of entries that failed.
func (uc *UseCase) fallbackToIndividualMetadataCreate(
	ctx context.Context,
	logger libLog.Logger,
	collection string,
	entries []MetadataEntry,
) (int, error) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // consistent with codebase pattern

	ctx, span := tracer.Start(ctx, "command.fallback_individual_metadata_create")
	defer span.End()

	var (
		failureCount int
		successCount int
	)

	for _, entry := range entries {
		meta := &mongodb.Metadata{
			EntityID:   entry.EntityID,
			EntityName: collection,
			Data:       entry.Data,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.TransactionMetadataRepo.Create(ctx, collection, meta); err != nil {
			if logger != nil {
				logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf(
					"Fallback: failed to create metadata for %s entity %s: %v",
					collection, entry.EntityID, err,
				))
			}

			failureCount++

			continue
		}

		successCount++
	}

	if logger != nil {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
			"Fallback metadata create for %s: %d/%d succeeded",
			collection, successCount, len(entries),
		))
	}

	if failureCount > 0 {
		return failureCount, fmt.Errorf("failed to create %d of %d metadata entries in fallback", failureCount, len(entries))
	}

	return 0, nil
}

// groupMetadataByCollection groups metadata entries by their collection name.
func groupMetadataByCollection(entries []MetadataEntry) map[string][]MetadataEntry {
	grouped := make(map[string][]MetadataEntry)

	for _, entry := range entries {
		grouped[entry.Collection] = append(grouped[entry.Collection], entry)
	}

	return grouped
}

// processMetadataAndEventsBulk processes metadata for multiple transaction payloads using bulk operations.
// It collects all metadata entries and creates them in a single batch per collection.
// Skips duplicate transactions that were not actually inserted (based on insertedTxIDs).
// Logs warnings on failure but does not return errors to maintain backward compatibility.
func (uc *UseCase) processMetadataAndEventsBulk(
	ctx context.Context,
	logger libLog.Logger,
	payloads []transaction.TransactionProcessingPayload,
	insertedTxIDs map[string]struct{},
) {
	// Collect all metadata entries from payloads
	entries := collectMetadataFromPayloads(payloads, insertedTxIDs)

	// Create metadata in bulk (handles batching and fallback internally)
	if err := uc.createMetadataBulk(ctx, entries); err != nil {
		if logger != nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to create bulk metadata: %v", err))
		}
	}
}

// collectMetadataFromPayloads extracts metadata entries from transaction payloads.
// It collects both transaction and operation metadata, skipping duplicates based on insertedTxIDs.
// When insertedTxIDs is empty, all payloads are processed (fallback/status-update scenarios).
func collectMetadataFromPayloads(
	payloads []transaction.TransactionProcessingPayload,
	insertedTxIDs map[string]struct{},
) []MetadataEntry {
	// Pre-allocate with estimated capacity (1 tx + avg 2 ops per payload)
	entries := make([]MetadataEntry, 0, len(payloads)*3)

	transactionTypeName := reflect.TypeFor[transaction.Transaction]().Name()
	operationTypeName := reflect.TypeFor[operation.Operation]().Name()

	for _, payload := range payloads {
		if payload.Transaction == nil {
			continue
		}

		tx := payload.Transaction

		// Determine if this transaction was actually inserted (not a duplicate).
		// If insertedTxIDs is empty, process all (fallback or status-update scenarios).
		txWasInserted := len(insertedTxIDs) == 0
		if !txWasInserted {
			_, txWasInserted = insertedTxIDs[tx.ID]
		}

		// Collect transaction-level metadata only for newly inserted transactions.
		// Status-transitioned (updated) transactions already have their metadata persisted.
		if txWasInserted && tx.Metadata != nil {
			entries = append(entries, MetadataEntry{
				EntityID:   tx.ID,
				Collection: transactionTypeName,
				Data:       tx.Metadata,
			})
		}

		// Always collect operation metadata regardless of transaction insert status.
		// Operations may be newly created even when the parent transaction was a
		// status-transition (update) rather than a fresh insert.
		for _, op := range tx.Operations {
			if op == nil || op.Metadata == nil {
				continue
			}

			entries = append(entries, MetadataEntry{
				EntityID:   op.ID,
				Collection: operationTypeName,
				Data:       op.Metadata,
			})
		}
	}

	return entries
}

// isInfrastructureError returns true when the error indicates an infrastructure-level
// failure (timeout, network, context cancellation) rather than a document-level issue
// (duplicate key, validation). Infrastructure errors should not trigger per-entry
// fallback because individual writes would fail with the same root cause.
func isInfrastructureError(err error) bool {
	if err == nil {
		return false
	}

	// Context cancellation / deadline exceeded
	if err == context.Canceled || err == context.DeadlineExceeded {
		return true
	}

	// MongoDB driver timeout (covers both client and server-side timeouts)
	if mongo.IsTimeout(err) {
		return true
	}

	// MongoDB driver network errors (connection refused, reset, DNS, etc.)
	if mongo.IsNetworkError(err) {
		return true
	}

	return false
}
