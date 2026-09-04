// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package readseam decides the read source for a request: a direct connection
// (replica) or a read-only transaction pinned to the primary. It isolates the
// dbresolver coupling so entity adapters depend only on repository.DBReader.
package readseam

import (
	"context"
	"database/sql"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/bxcodec/dbresolver/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// ReadSource identifies which database served a read. It is used as both the
// AcquireReadFrom return value and the span-attribute / metric-label string.
type ReadSource string

const (
	// ReadSourcePrimary marks a read served by the primary (read-only tx).
	ReadSourcePrimary ReadSource = "primary"
	// ReadSourceReplica marks a read served directly by the resolved handle (replica).
	ReadSourceReplica ReadSource = "replica"
)

// readSourceAttributeKey is the span attribute recording the served read source.
const readSourceAttributeKey = "db.read_source"

// readSourceLabels holds the precomputed counter label map per read source so the
// hot read path reuses one map per branch instead of allocating a fresh map on
// every read. Safe to share: the metrics builder's WithLabels only reads the map
// (copies entries into its own attribute slice) and never retains or mutates it.
var readSourceLabels = map[ReadSource]map[string]string{
	ReadSourcePrimary: {"source": string(ReadSourcePrimary)},
	ReadSourceReplica: {"source": string(ReadSourceReplica)},
}

// ReadConn is a connection handle that can serve reads and open a read-only tx.
// dbresolver.DB satisfies it directly (no wrapper needed).
type ReadConn interface {
	repository.DBReader
	BeginTx(ctx context.Context, opts *sql.TxOptions) (dbresolver.Tx, error)
}

// Compile-time guarantees that both dbresolver handles satisfy the read surface,
// that dbresolver.Tx is a finalizable read tx, and that dbresolver.DB satisfies
// the acquire seam's handle interface.
var (
	_ repository.DBReader = dbresolver.DB(nil)
	_ repository.DBReader = dbresolver.Tx(nil)
	_ repository.DBReadTx = dbresolver.Tx(nil)
	_ ReadConn            = dbresolver.DB(nil)
)

// AcquireReadFrom decides the read source and returns a release func the caller
// MUST defer.
//
// When routeToPrimary is enabled AND the context carries the primary-read intent,
// the read is wrapped in a read-only transaction so dbresolver routes it to the
// primary (BeginTx always targets the primary). Otherwise it returns the handle
// directly and a no-op release, preserving replica read behavior byte-for-byte.
//
// Fail-closed: when the read-only transaction cannot be opened, the error is
// returned and NO direct/replica reader is handed back. For a ledger, reading
// wrong state is worse than failing.
func AcquireReadFrom(ctx context.Context, conn ReadConn, routeToPrimary bool) (repository.DBReader, func() error, ReadSource, error) {
	if !routeToPrimary || !readrouting.IsPrimaryRead(ctx) {
		recordReadSource(ctx, ReadSourceReplica)

		return conn, func() error { return nil }, ReadSourceReplica, nil
	}

	// Fail fast on an already-cancelled context: a caller cancellation is not a
	// technical failure, so it must not flip the span red (telemetry T5). Return
	// the wrapped ctx error and no reader (fail-closed) before the BeginTx round-trip.
	if err := ctx.Err(); err != nil {
		return nil, nil, "", fmt.Errorf("primary read aborted before opening transaction: %w", err)
	}

	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		libOpentelemetry.HandleSpanError(trace.SpanFromContext(ctx), "Failed to open read-only transaction for primary read", err)

		return nil, nil, "", fmt.Errorf("open read-only primary read transaction: %w", err)
	}

	recordReadSource(ctx, ReadSourcePrimary)

	// A read-only tx performs no writes; committing just releases the connection.
	release := func() error {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit read-only primary read transaction: %w", err)
		}

		return nil
	}

	return tx, release, ReadSourcePrimary, nil
}

// recordReadSource emits the read-source signal for a served read: the
// db.read_source attribute on the caller's existing span and the
// db_read_source_total counter. It is best-effort — an unavailable metrics
// factory is swallowed at Debug and never fails the read; it never panics and
// never opens a child span.
func recordReadSource(ctx context.Context, source ReadSource) {
	trace.SpanFromContext(ctx).SetAttributes(attribute.String(readSourceAttributeKey, string(source)))

	logger, _, _, factory := libObservability.NewTrackingFromContext(ctx)
	if factory == nil {
		return
	}

	counter, err := factory.Counter(utils.DBReadSourceTotal)
	if err == nil {
		err = counter.WithLabels(readSourceLabels[source]).Add(ctx, 1)
	}

	if err != nil && logger != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit read-source counter", libLog.Err(err))
	}
}
