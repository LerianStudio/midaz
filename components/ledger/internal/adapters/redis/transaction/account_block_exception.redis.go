// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
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

// GetAccountBlockException reads one exception from the cache. See the interface
// contract on RedisRepository for the (nil, nil) miss convention and for why
// this is a read only.
func (rr *RedisConsumerRepository) GetAccountBlockException(ctx context.Context, organizationID, ledgerID, exceptionID uuid.UUID) (*mmodel.AccountBlockExceptionRedis, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_account_block_exception")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_block_exception_id", exceptionID.String()),
	)

	key, err := tenantKeyFromContextOrError(ctx,
		utils.AccountBlockExceptionInternalKey(organizationID, ledgerID, exceptionID))
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace account block exception key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace account block exception key", libLog.Err(err))

		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get Redis client", libLog.Err(err))

		return nil, err
	}

	raw, err := rds.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			span.SetAttributes(attribute.Bool("app.account_block_exception_found", false))

			return nil, nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to read account block exception from redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to read account block exception from Redis", libLog.Err(err))

		return nil, err
	}

	var exception mmodel.AccountBlockExceptionRedis
	if err := json.Unmarshal([]byte(raw), &exception); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode account block exception", err)
		logger.Log(ctx, libLog.LevelError, "Failed to decode account block exception", libLog.Err(err))

		return nil, fmt.Errorf("failed to decode account block exception: %w", err)
	}

	span.SetAttributes(attribute.Bool("app.account_block_exception_found", true))

	return &exception, nil
}

// accountBlockExceptionEval is what one presented grant contributes to the
// balance-mutation EVAL: the exception key the script validates and deletes, the
// two values it compares the CACHED grant against, and the tenant-namespaced
// balance keys the grant bypasses the account block on.
//
// The BIND itself — which debit leg the grant authorizes, and which
// system-derived overdraft companions come with it — is decided by
// mtransaction.ResolveAccountBlockExceptionBinding in the balance step, so the
// balances the Go pre-validation stops fast-failing are exactly the ones the
// script bypasses. This type only namespaces that decision for the wire.
type accountBlockExceptionEval struct {
	// key is the tenant-namespaced exception key, passed as KEYS[4].
	key string
	// alias is the source account alias the transaction debits.
	alias string
	// amount is the canonical decimal string of that debit.
	amount string
	// balanceKeys are the tenant-namespaced balance keys the grant bypasses the
	// account block on: the debited source balance plus the overdraft companions
	// the system derived from it. Never empty when a grant was presented.
	balanceKeys []string
}

// resolveAccountBlockExceptionEval namespaces a resolved binding for the EVAL.
//
// Every balance key goes through the SAME helper the plan builder runs over each
// operation, so the bypass list the script matches against is byte-identical to
// what it reads out of the per-operation ARGV groups. A mismatch there would be
// invisible: the guard would simply not find the key and reject a valid grant.
func resolveAccountBlockExceptionEval(ctx context.Context, organizationID, ledgerID uuid.UUID, binding *mtransaction.AccountBlockExceptionBinding) (*accountBlockExceptionEval, error) {
	if binding == nil {
		return nil, nil
	}

	key, err := tenantKeyFromContextOrError(ctx,
		utils.AccountBlockExceptionInternalKey(organizationID, ledgerID, binding.ID))
	if err != nil {
		return nil, err
	}

	internalKeys := binding.InternalKeys()

	balanceKeys := make([]string, 0, len(internalKeys))

	for _, internalKey := range internalKeys {
		prefixed, err := tenantKeyFromContextOrError(ctx, internalKey)
		if err != nil {
			return nil, err
		}

		balanceKeys = append(balanceKeys, prefixed)
	}

	return &accountBlockExceptionEval{
		key:         key,
		alias:       binding.Alias,
		amount:      binding.Amount,
		balanceKeys: balanceKeys,
	}, nil
}

// headerWidth is the number of ARGV slots the eval occupies ahead of the first
// balance operation group: the two expected values, the bypass-list count, the
// idempotency marker key, and one slot per bypassed balance key.
//
// A nil receiver is the no-grant case and still occupies the four fixed slots,
// so the script always reads its header from the same four positions and never
// has to branch on whether a grant exists before it can compute its own stride.
func (e *accountBlockExceptionEval) headerWidth() int {
	if e == nil {
		return luaArgsHeaderFixedSize
	}

	return luaArgsHeaderFixedSize + len(e.balanceKeys)
}

// bypassedCount is the number of balances the grant bypasses the account block
// on, nil-safe so the no-grant span attribute needs no guard at the call site.
func (e *accountBlockExceptionEval) bypassedCount() int {
	if e == nil {
		return 0
	}

	return len(e.balanceKeys)
}

// writeHeader fills the reserved header slots of args in place. args must have
// been allocated with at least headerWidth() leading slots — the plan builder
// reserves exactly that many, so the header costs no allocation and no copy on
// either path.
//
// applyMarkerKey is the tenant-namespaced idempotency marker key of this
// execution. It is written on both paths because the script's replay gate runs
// ahead of every grant concern.
func (e *accountBlockExceptionEval) writeHeader(args []any, applyMarkerKey string) {
	if e == nil {
		args[0] = ""
		args[1] = ""
		args[2] = "0"
		args[3] = applyMarkerKey

		return
	}

	args[0] = e.alias
	args[1] = e.amount
	args[2] = strconv.Itoa(len(e.balanceKeys))
	args[3] = applyMarkerKey

	for i, balanceKey := range e.balanceKeys {
		args[luaArgsHeaderFixedSize+i] = balanceKey
	}
}
