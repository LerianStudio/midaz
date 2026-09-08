// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// AccountBlockException is one grant to write, already validated and
// canonicalized by the command layer: ID is the minted identifier, Alias the
// authorized source account, Amount the canonical decimal string the
// transaction script compares against, and TTL the lifetime to apply.
type AccountBlockException struct {
	ID     uuid.UUID
	Alias  string
	Amount string
	TTL    time.Duration
}

// CreateAccountBlockExceptions writes a batch of single-use account-block
// exceptions to the cache, one key per exception, each with its own native
// Redis expiry (SET ... EX ttl). It is the ONLY storage an exception has.
//
// The cache is the complete store by design: an exception is an ephemeral,
// single-use authorization, so expiry is delegated entirely to Redis — there is
// no table, no sweeper, and no re-hydration. A cache flush therefore DROPS every
// outstanding exception; that is accepted, because a dropped grant fails closed
// (the account stays blocked) and the operator simply mints another.
//
// Writes go out in ONE TRANSACTIONAL pipeline (MULTI/EXEC), so the batch costs a
// single round trip and is NEVER PARTIALLY APPLIED: the caller can never be
// handed identifiers of which only some exist. The transaction is legal in
// cluster mode because every exception key carries the same literal
// {transactions} hash tag and therefore lands in one slot — the same
// co-location the transaction script's multi-key EVAL depends on.
//
// An error does NOT mean the batch was not applied — only that it was not
// applied in part. A connection lost after Redis executed EXEC but before the
// client read the acknowledgement fails here on a batch that fully landed, so
// the outcome is UNKNOWN to the caller. That is safe to surface as a failure
// rather than reconcile: the caller reports no identifiers, so nothing can
// present a grant it never received, and any orphaned key expires on its own
// TTL.
//
// Amounts are financial values, so no amount reaches a log field or a span
// attribute — only counts do.
func (rr *RedisConsumerRepository) CreateAccountBlockExceptions(ctx context.Context, organizationID, ledgerID uuid.UUID, exceptions []AccountBlockException) error {
	if len(exceptions) == 0 {
		return nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.create_account_block_exceptions")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.Int("app.request.exceptions_count", len(exceptions)),
	)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get Redis client", libLog.Err(err))

		return err
	}

	pipe := rds.TxPipeline()

	for _, exception := range exceptions {
		key, value, err := buildAccountBlockExceptionEntry(ctx, organizationID, ledgerID, exception)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to build account block exception entry", err)
			logger.Log(ctx, libLog.LevelError, "Failed to build account block exception entry", libLog.Err(err))

			return err
		}

		pipe.Set(ctx, key, value, exception.TTL)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to write account block exceptions on redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to write account block exceptions on Redis", libLog.Err(err))

		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Account block exceptions written to cache",
		libLog.Int("count", len(exceptions)))

	return nil
}

// buildAccountBlockExceptionEntry returns the tenant-namespaced key and the JSON
// value for one exception. The value carries CamelCase keys so the transaction
// Lua script decodes a grant with the same casing contract it already applies to
// the balance blobs.
func buildAccountBlockExceptionEntry(ctx context.Context, organizationID, ledgerID uuid.UUID, exception AccountBlockException) (string, []byte, error) {
	key, err := tenantKeyFromContextOrError(ctx,
		utils.AccountBlockExceptionInternalKey(organizationID, ledgerID, exception.ID))
	if err != nil {
		return "", nil, err
	}

	value, err := json.Marshal(mmodel.AccountBlockExceptionRedis{
		Alias:  exception.Alias,
		Amount: exception.Amount,
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal account block exception: %w", err)
	}

	return key, value, nil
}
