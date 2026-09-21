// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"
)

const (
	// MaxRecoveryScanCount bounds the COUNT hint of one recovery scan. Redis treats
	// COUNT as a hint, so a returned batch can be larger; the page reports its own
	// size and never drops an entry to fit a bound.
	MaxRecoveryScanCount = 1000

	// DefaultRecoveryScanCount is the COUNT hint used when a caller passes none.
	DefaultRecoveryScanCount = 200
)

// RecoveryScanRecord is one raw record of a recovery hash, exactly as persisted.
// The scan reads: it never rewrites a record, removes one, or touches an
// acknowledgment.
type RecoveryScanRecord struct {
	Field   string
	Payload string
}

// RecoveryScanPage is one bounded page of one recovery hash.
//
// Cursor is where the walk continues; zero means this hash returned its terminal
// cursor. An empty page is NOT terminal on its own — HSCAN may return no entry for
// a bucket it passed — so only the terminal cursor completes a walk.
type RecoveryScanPage struct {
	Source  RecoveryQueueSource
	Records []RecoveryScanRecord
	Cursor  uint64
	Bytes   int
}

// Complete reports whether this page closed the walk of its hash.
func (page RecoveryScanPage) Complete() bool {
	return page.Cursor == 0
}

// ScanRecoveryMessages reads one bounded page of one recovery hash with HSCAN.
//
// It exists so a caller can walk the existing recovery records without HGETALL:
// the cursor is the caller's, the COUNT hint is bounded, and the page reports the
// byte size it carries so the caller can enforce its own budget. Cursors of the
// two hashes are independent and must never be mixed, because the same
// transaction/execution field can exist in both.
func (rr *RedisConsumerRepository) ScanRecoveryMessages(ctx context.Context, source RecoveryQueueSource, cursor uint64, count int64) (RecoveryScanPage, error) {
	if err := ctx.Err(); err != nil {
		return RecoveryScanPage{}, fmt.Errorf("scan recovery messages: %w", err)
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.scan_recovery_messages")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.recovery_source", string(source)))

	queueKey, err := recoveryQueueKey(source)
	if err != nil {
		return RecoveryScanPage{}, err
	}

	prefixedQueue, err := tenantKeyFromContextOrError(ctx, queueKey)
	if err != nil {
		return RecoveryScanPage{}, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get Redis client", err)

		return RecoveryScanPage{}, fmt.Errorf("get recovery scan client: %w", err)
	}

	if count <= 0 {
		count = DefaultRecoveryScanCount
	}

	if count > MaxRecoveryScanCount {
		count = MaxRecoveryScanCount
	}

	values, nextCursor, err := rds.HScan(ctx, prefixedQueue, cursor, "", count).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to scan recovery messages", err)

		return RecoveryScanPage{}, fmt.Errorf("scan recovery messages: %w", err)
	}

	page := RecoveryScanPage{Source: source, Cursor: nextCursor, Records: make([]RecoveryScanRecord, 0, len(values)/2)}

	// HSCAN answers a flat field/value sequence. An odd-length answer would pair a
	// field with the wrong payload, so it is reported instead of being trimmed.
	if len(values)%2 != 0 {
		err := fmt.Errorf("recovery scan returned %d unpaired values", len(values))
		libOpentelemetry.HandleSpanError(span, "Failed to pair recovery scan values", err)

		return RecoveryScanPage{}, err
	}

	for index := 0; index+1 < len(values); index += 2 {
		page.Records = append(page.Records, RecoveryScanRecord{Field: values[index], Payload: values[index+1]})
		page.Bytes += len(values[index]) + len(values[index+1])
	}

	span.SetAttributes(
		attribute.Int("db.rows_returned", len(page.Records)),
		attribute.Int("app.recovery_scan.page_bytes", page.Bytes),
	)

	return page, nil
}
