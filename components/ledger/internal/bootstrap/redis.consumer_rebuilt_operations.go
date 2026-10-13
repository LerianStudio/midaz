// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// legacyRecordOperationsField is the backup record key that carries the
// materialized operations; a record with operations replays them as recorded.
const legacyRecordOperationsField = "operations"

// storeRebuiltOperations writes the operations a legacy replay rebuilt into the
// backup record it read, before anything reaches PostgreSQL. Operations are
// deduplicated only by ID, so a replay retried after its commit (a metadata
// failure, a lost entry removal, a crash) must find these IDs in the record
// instead of rebuilding new ones beside the committed legs.
//
// It returns the record now stored and true when the replay may write. On false
// nothing may be written: a Redis failure leaves the record for the next cycle
// without counting toward quarantine, and a record removed or rewritten since
// it was read belongs to whoever changed it and is left untouched.
func (r *RedisQueueConsumer) storeRebuiltOperations(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	key, rawPayload string,
	m mmodel.TransactionRedisQueue,
	operations []*operation.Operation,
) (string, bool) {
	replacement, err := withRebuiltOperations(rawPayload, operations)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to encode rebuilt operations into backup record", err)
		logger.Log(ctx, libLog.LevelError, "Rebuilt operations cannot be stored in backup record; routing to quarantine flow",
			libLog.String("key", key), libLog.Err(err))

		r.quarantinePoisonRecord(ctx, span, logger, key, m.OrganizationID, m.LedgerID, m.TransactionID, []byte(rawPayload), "rebuilt_operations_encoding_failure")

		return "", false
	}

	outcome, err := r.queue.ReplaceMessageInQueueIfValue(ctx, key, rawPayload, replacement)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to store rebuilt operations in backup record", err)
		logger.Log(ctx, libLog.LevelError, "Failed to store rebuilt operations in backup record; record left in backup queue",
			libLog.String("key", key), libLog.Err(err))

		return "", false
	}

	if outcome != txRedis.BackupRecordReplaced {
		libOpentelemetry.HandleSpanEvent(span, "Backup record changed before rebuilt operations were stored")
		logger.Log(ctx, libLog.LevelWarn, "Backup record was removed or rewritten after it was read; replay skipped",
			libLog.String("key", key), libLog.Bool("record_missing", outcome == txRedis.BackupRecordMissing))

		return "", false
	}

	libOpentelemetry.HandleSpanEvent(span, "Rebuilt operations stored in backup record")

	return replacement, true
}

// withRebuiltOperations returns raw with only its operations key replaced.
// Every other key keeps its recorded value, so the record keeps its legacy,
// version-less shape, its ttl (and with it its replay eligibility), and any
// field this build does not decode. Keys come out sorted and values compacted,
// which preserves their meaning but not the record's exact bytes.
func withRebuiltOperations(raw string, operations []*operation.Operation) (string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return "", fmt.Errorf("decode backup record: %w", err)
	}

	if fields == nil {
		return "", errors.New("backup record is not a JSON object")
	}

	recorded := make([]mmodel.OperationRedis, 0, len(operations))
	for _, op := range operations {
		recorded = append(recorded, op.ToRedis())
	}

	encodedOperations, err := encodeJSONWithoutHTMLEscape(recorded)
	if err != nil {
		return "", fmt.Errorf("encode rebuilt operations: %w", err)
	}

	fields[legacyRecordOperationsField] = encodedOperations

	encoded, err := encodeJSONWithoutHTMLEscape(fields)
	if err != nil {
		return "", fmt.Errorf("encode backup record: %w", err)
	}

	return string(encoded), nil
}

func encodeJSONWithoutHTMLEscape(value any) ([]byte, error) {
	var buffer bytes.Buffer

	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(value); err != nil {
		return nil, err
	}

	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}
