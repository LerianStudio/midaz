// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CreateAccountBlockExceptions mints a batch of single-use account-block
// exceptions and stores them in the cache with a native TTL.
//
// The batch is ALL-OR-NOTHING: every item is validated before anything is
// written, and the first invalid item rejects the whole request naming its
// index. Validation is strictly READ-ONLY — no account, balance or block state
// is mutated here, so a rejected (or an accepted) batch leaves the ledger
// exactly as it was. The alias existence check is one batched SELECT over the
// distinct aliases, not one query per item.
//
// Exceptions have no database row: the cache is their complete storage and Redis
// owns expiry. See RedisRepository.CreateAccountBlockExceptions.
func (uc *UseCase) CreateAccountBlockExceptions(ctx context.Context, organizationID, ledgerID uuid.UUID, input *mmodel.CreateAccountBlockExceptionsInput) (_ *mmodel.AccountBlockExceptions, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account_block_exceptions")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_account_block_exceptions", start, err)
	}()

	if input == nil || len(input.Exceptions) == 0 {
		err := pkg.ValidateBusinessError(constant.ErrAccountBlockExceptionsRequired, constant.EntityAccountBlockException)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty account block exception batch", err)

		return nil, err
	}

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.Int("app.request.exceptions_count", len(input.Exceptions)),
	)

	if len(input.Exceptions) > mmodel.AccountBlockExceptionMaxBatchSize {
		err := pkg.ValidateBusinessError(constant.ErrAccountBlockExceptionsBatchTooLarge, constant.EntityAccountBlockException)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account block exception batch above the limit", err)

		return nil, err
	}

	entries, err := normalizeAccountBlockExceptions(input.Exceptions)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account block exception batch", err)
		logger.Log(ctx, libLog.LevelWarn, "Invalid account block exception batch", libLog.Err(err))

		return nil, err
	}

	if err := uc.assertAccountBlockExceptionAliasesExist(ctx, organizationID, ledgerID, input.Exceptions); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Unknown alias in account block exception batch", err)
		logger.Log(ctx, libLog.LevelWarn, "Unknown alias in account block exception batch", libLog.Err(err))

		return nil, err
	}

	// One instant for every expiresAt in the response, so the batch reports a
	// single, consistent window rather than drifting item by item.
	now := time.Now().UTC()

	exceptions := make([]txRedis.AccountBlockException, 0, len(entries))
	created := make([]mmodel.AccountBlockException, 0, len(entries))

	for _, entry := range entries {
		id, err := libCommons.GenerateUUIDv7()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to generate account block exception id", err)
			logger.Log(ctx, libLog.LevelError, "Failed to generate account block exception id", libLog.Err(err))

			return nil, err
		}

		exceptions = append(exceptions, txRedis.AccountBlockException{
			ID:     id,
			Alias:  entry.alias,
			Amount: entry.canonicalAmount,
			TTL:    entry.ttl,
		})

		created = append(created, mmodel.AccountBlockException{
			AccountBlockExceptionID: id.String(),
			AccountAlias:            entry.alias,
			Amount:                  entry.requestedAmount,
			ExpiresAt:               now.Add(entry.ttl),
		})
	}

	if err := uc.TransactionRedisRepo.CreateAccountBlockExceptions(ctx, organizationID, ledgerID, exceptions); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to store account block exceptions", err)
		logger.Log(ctx, libLog.LevelError, "Failed to store account block exceptions", libLog.Err(err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, "Account block exceptions created",
		libLog.Int("count", len(created)))

	return &mmodel.AccountBlockExceptions{Exceptions: created}, nil
}

// accountBlockExceptionEntry is one validated batch item. requestedAmount is the
// caller's decimal string, echoed back verbatim so the response mirrors the
// request; canonicalAmount is the normalized form (decimal.String()) written to
// the cache, which is the form the transaction script receives its operation
// amounts in and therefore the form it can compare against.
type accountBlockExceptionEntry struct {
	alias           string
	requestedAmount string
	canonicalAmount string
	ttl             time.Duration
}

// normalizeAccountBlockExceptions validates every batch item and returns the
// normalized entries. It runs to the FIRST failure and names the offending
// item's zero-based index, so a caller can fix the batch without bisecting it.
// It performs no I/O.
func normalizeAccountBlockExceptions(items []mmodel.CreateAccountBlockExceptionInput) ([]accountBlockExceptionEntry, error) {
	entries := make([]accountBlockExceptionEntry, 0, len(items))

	for i, item := range items {
		amount, err := decimal.NewFromString(item.Amount)
		if err != nil || amount.LessThanOrEqual(decimal.Zero) {
			return nil, pkg.ValidateBusinessError(constant.ErrAccountBlockExceptionInvalidAmount,
				constant.EntityAccountBlockException, i)
		}

		ttlSeconds := mmodel.AccountBlockExceptionDefaultTTLSeconds

		if item.TTL != nil {
			if *item.TTL <= 0 || *item.TTL > mmodel.AccountBlockExceptionMaxTTLSeconds {
				return nil, pkg.ValidateBusinessError(constant.ErrAccountBlockExceptionInvalidTTL,
					constant.EntityAccountBlockException, i)
			}

			ttlSeconds = *item.TTL
		}

		entries = append(entries, accountBlockExceptionEntry{
			alias:           item.AccountAlias,
			requestedAmount: item.Amount,
			canonicalAmount: amount.String(),
			ttl:             time.Duration(ttlSeconds) * time.Second,
		})
	}

	return entries, nil
}

// assertAccountBlockExceptionAliasesExist rejects the batch when any item names
// an alias the organization and ledger do not have. Aliases are unique per
// ledger, so one batched read over the distinct aliases answers the whole batch;
// the read is the only database access this command performs and it mutates
// nothing.
func (uc *UseCase) assertAccountBlockExceptionAliasesExist(ctx context.Context, organizationID, ledgerID uuid.UUID, items []mmodel.CreateAccountBlockExceptionInput) error {
	aliases := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))

	for _, item := range items {
		if _, ok := seen[item.AccountAlias]; ok {
			continue
		}

		seen[item.AccountAlias] = struct{}{}
		aliases = append(aliases, item.AccountAlias)
	}

	accounts, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, aliases)
	if err != nil {
		return err
	}

	found := make(map[string]struct{}, len(accounts))

	for _, acc := range accounts {
		if acc != nil && acc.Alias != nil {
			found[*acc.Alias] = struct{}{}
		}
	}

	// Reported in REQUEST order rather than in the order of the deduplicated
	// lookup, so the index in the error is the index the caller sent.
	for i, item := range items {
		if _, ok := found[item.AccountAlias]; !ok {
			return pkg.ValidateBusinessError(constant.ErrAccountBlockExceptionAliasNotFound,
				constant.EntityAccountBlockException, i, item.AccountAlias)
		}
	}

	return nil
}
