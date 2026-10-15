// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
)

const (
	// maxBulkMetadataEntries is the maximum number of entries allowed in a single bulk operation.
	// This prevents resource exhaustion from unbounded input.
	maxBulkMetadataEntries = 10000
)

// MetadataEntry represents a single metadata entry for bulk operations.
// It encapsulates the entity ID, collection name, and metadata data.
// TransactionID is the transaction that owns the entry (the entity itself for
// transaction metadata, the parent transaction for operation metadata); it is
// used only to attribute a write failure and is not validated.
type MetadataEntry struct {
	EntityID      string
	Collection    string
	Data          map[string]any
	TransactionID string
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
// An entry with nil or empty Data carries nothing to persist: no document is written
// for it and it is never reported as not confirmed.
//
// Returns (nil, nil) if all entries were created successfully (or carried no data).
// Otherwise returns the entries whose write was not confirmed together with an
// aggregate error. An invalid entry aborts the batch before any write, so every
// entry carrying data is returned as not confirmed.
func (uc *UseCase) createMetadataBulk(ctx context.Context, entries []MetadataEntry) ([]MetadataEntry, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_metadata_bulk")
	defer span.End()

	// Filter and validate entries
	validEntries := make([]MetadataEntry, 0, len(entries))

	for i, entry := range entries {
		// Skip entries with nothing to persist
		if len(entry.Data) == 0 {
			continue
		}

		// Validate entry fields
		if err := entry.Validate(); err != nil {
			libOpentelemetry.HandleSpanError(span, fmt.Sprintf("Invalid entry at index %d", i), err)

			return entriesWithData(entries), fmt.Errorf("invalid metadata entry at index %d: %w", i, err)
		}

		validEntries = append(validEntries, entry)
	}

	if len(validEntries) == 0 {
		return nil, nil
	}

	// Process in chunks of maxBulkMetadataEntries to bound per-batch resource usage.
	// Each chunk is grouped by collection and processed independently.
	var (
		failedEntries  []MetadataEntry
		totalAttempted int
	)

	for chunkStart := 0; chunkStart < len(validEntries); chunkStart += maxBulkMetadataEntries {
		chunkEnd := min(chunkStart+maxBulkMetadataEntries, len(validEntries))
		chunk := validEntries[chunkStart:chunkEnd]

		grouped := groupMetadataByCollection(chunk)

		for collection, collectionEntries := range grouped {
			totalAttempted += len(collectionEntries)

			failed, err := uc.createMetadataForCollection(ctx, logger, collection, collectionEntries)
			if err != nil {
				libOpentelemetry.HandleSpanError(span, fmt.Sprintf("Failed to create metadata for collection %s", collection), err)
			}

			failedEntries = append(failedEntries, failed...)
		}
	}

	if len(failedEntries) > 0 {
		return failedEntries, fmt.Errorf("failed to create %d of %d metadata entries", len(failedEntries), totalAttempted)
	}

	return nil, nil
}

// createMetadataForCollection creates metadata entries for a single collection.
// Uses bulk insert for multiple entries, direct Create for single entry.
// Returns the entries whose write was not confirmed and the error that caused it.
func (uc *UseCase) createMetadataForCollection(
	ctx context.Context,
	logger libLog.Logger,
	collection string,
	entries []MetadataEntry,
) ([]MetadataEntry, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_metadata_for_collection")
	defer span.End()

	// Add span attributes for observability
	span.SetAttributes(
		attribute.String("collection", collection),
		attribute.Int("entry_count", len(entries)),
	)

	if len(entries) == 0 {
		return nil, nil
	}

	// For single entry, use Create directly (optimization)
	if len(entries) == 1 {
		if err := uc.createSingleMetadata(ctx, logger, collection, entries[0]); err != nil {
			return entries, err
		}

		return nil, nil
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
				logger.Log(ctx, libLog.LevelError, "Bulk metadata insert failed with infrastructure error, no fallback",
					libLog.String("collection", collection), libLog.Err(err))
			}

			return entries, fmt.Errorf("infrastructure error during bulk insert for %s: %w", collection, err)
		}

		// Document-level errors (duplicate key, validation) — fall back to individual creates.
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Bulk insert failed, falling back to individual creates", err)

		if logger != nil {
			logger.Log(ctx, libLog.LevelWarn, "Bulk metadata insert failed, using fallback",
				libLog.String("collection", collection), libLog.Err(err))
		}

		return uc.fallbackToIndividualMetadataCreate(ctx, logger, collection, entries)
	}

	if logger != nil {
		logger.Log(ctx, libLog.LevelDebug, "Bulk inserted metadata",
			libLog.String("collection", collection),
			libLog.Int("attempted", int(result.Attempted)),
			libLog.Int("inserted", int(result.Inserted)),
			libLog.Int("matched", int(result.Matched)))
	}

	return nil, nil
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
			logger.Log(ctx, libLog.LevelError, "Failed to create metadata",
				libLog.String("collection", collection), libLog.String("entity_id", entry.EntityID), libLog.Err(err))
		}

		return err
	}

	return nil
}

// fallbackToIndividualMetadataCreate creates metadata entries one by one.
// Returns the entries whose write failed and an aggregate error when any did.
func (uc *UseCase) fallbackToIndividualMetadataCreate(
	ctx context.Context,
	logger libLog.Logger,
	collection string,
	entries []MetadataEntry,
) ([]MetadataEntry, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.fallback_individual_metadata_create")
	defer span.End()

	var (
		failedEntries []MetadataEntry
		successCount  int
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
				logger.Log(ctx, libLog.LevelWarn, "Fallback: failed to create metadata",
					libLog.String("collection", collection), libLog.String("entity_id", entry.EntityID), libLog.Err(err))
			}

			failedEntries = append(failedEntries, entry)

			continue
		}

		successCount++
	}

	if logger != nil {
		logger.Log(ctx, libLog.LevelDebug, "Fallback metadata create complete",
			libLog.String("collection", collection),
			libLog.Int("succeeded", successCount),
			libLog.Int("total", len(entries)))
	}

	if len(failedEntries) > 0 {
		return failedEntries, fmt.Errorf("failed to create %d of %d metadata entries in fallback", len(failedEntries), len(entries))
	}

	return nil, nil
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
// Returns the IDs of the transactions whose own metadata or whose operations'
// metadata was not confirmed, logging one Warn per such transaction. The set is
// empty when every entry was confirmed.
func (uc *UseCase) processMetadataAndEventsBulk(
	ctx context.Context,
	logger libLog.Logger,
	payloads []transaction.TransactionProcessingPayload,
) map[string]struct{} {
	// Collect all metadata entries from payloads
	entries := collectMetadataFromPayloads(payloads)

	// Create metadata in bulk (handles batching and fallback internally)
	failedEntries, err := uc.createMetadataBulk(ctx, entries)

	failedTxIDs := make(map[string]struct{}, len(failedEntries))

	for _, entry := range failedEntries {
		if _, seen := failedTxIDs[entry.TransactionID]; seen {
			continue
		}

		failedTxIDs[entry.TransactionID] = struct{}{}

		if logger != nil {
			logger.Log(ctx, libLog.LevelWarn, "Transaction metadata not confirmed",
				libLog.String("transaction_id", entry.TransactionID), libLog.Err(err))
		}
	}

	return failedTxIDs
}

// collectMetadataFromPayloads extracts metadata entries from transaction payloads.
// It collects both transaction and operation metadata for every payload, whether
// or not the transaction was inserted by this batch, so a redelivery can repair
// metadata that a previous attempt did not persist. The writes are insert-if-absent,
// so metadata already present is left untouched. Each entry carries the ID of the
// transaction that owns it. Nil or empty metadata carries nothing to persist and yields
// no entry, so a transaction without metadata is confirmed trivially.
func collectMetadataFromPayloads(
	payloads []transaction.TransactionProcessingPayload,
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

		if len(tx.Metadata) > 0 {
			entries = append(entries, MetadataEntry{
				EntityID:      tx.ID,
				Collection:    transactionTypeName,
				Data:          tx.Metadata,
				TransactionID: tx.ID,
			})
		}

		for _, op := range tx.Operations {
			if op == nil || len(op.Metadata) == 0 {
				continue
			}

			entries = append(entries, MetadataEntry{
				EntityID:      op.ID,
				Collection:    operationTypeName,
				Data:          op.Metadata,
				TransactionID: tx.ID,
			})
		}
	}

	return entries
}

// entriesWithData returns the entries that carry metadata to persist (non-empty Data).
func entriesWithData(entries []MetadataEntry) []MetadataEntry {
	withData := make([]MetadataEntry, 0, len(entries))

	for _, entry := range entries {
		if len(entry.Data) > 0 {
			withData = append(withData, entry)
		}
	}

	return withData
}

// isInfrastructureError returns true when the error indicates an infrastructure-level
// failure (timeout, network, context cancellation) rather than a document-level issue
// (duplicate key, validation). Infrastructure errors should not trigger per-entry
// fallback because individual writes would fail with the same root cause.
func isInfrastructureError(err error) bool {
	if err == nil {
		return false
	}

	// Context cancellation / deadline exceeded (errors.Is handles wrapped sentinels)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
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
