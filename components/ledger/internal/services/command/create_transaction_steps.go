// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// prepareCreateTransaction mints the transaction id, resolves the transaction
// date, records the safe payload shape on the span, rejects a non-positive send
// value and applies the default balance keys to both legs.
func (uc *UseCase) prepareCreateTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, run *createTransactionRun) error {
	transactionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate transaction id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to generate transaction id", libLog.Err(err))

		return err
	}

	transactionDate, err := mtransaction.CheckTransactionDate(ctx, run.input, run.status)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction date validation failed", err)

		return err
	}

	spanattr.RecordSafePayloadAttributes(span, run.input)

	if run.input.Send.Value.LessThanOrEqual(decimal.Zero) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction value must be greater than zero", err)
		logger.Log(ctx, libLog.LevelWarn, "Transaction value must be greater than zero", libLog.String("value", run.input.Send.Value.String()))

		return err
	}

	mtransaction.ApplyDefaultBalanceKeys(run.input.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(run.input.Send.Distribute.To)

	run.transactionID = transactionID
	run.transactionDate = transactionDate

	return nil
}

// claimTransactionIdempotency hashes the request identity and claims the Redis
// idempotency slot. A non-nil first return value is the replayed transaction the
// slot already holds, which the caller answers with instead of posting again.
//
// The hash is intentionally computed over the RAW pre-fee payload (before the fee
// seam mutates run.input.Send). Fees are deterministic given the same raw input +
// the same package configuration, so the raw body is the stable identity of the
// request. Two consequences are accepted by design (P4-T15): (1) package-config
// churn — if package config changes between two identical-key requests, the replay
// returns the FIRST fee-inclusive result (idempotency wins over recomputation); (2)
// deleted-package near-miss — a NON-replay request (different key, same body) issued
// after a package DELETE recomputes against the now-deleted package and yields a
// different fee outcome, which is correct because the hash keys on the raw body, not
// the package version. Package version is deliberately NOT part of the key.
//
// idempotencyHashSource is the optional preimage override: a non-empty value keys the
// hash off the raw body as submitted, an empty one off the canonical serialized
// transaction. The HashSHA256 mechanism is the same regardless of which source is used.
func (uc *UseCase) claimTransactionIdempotency(ctx context.Context, span trace.Span, logger libLog.Logger, run *createTransactionRun, idempotencyHashSource string) (*transaction.Transaction, error) {
	hashSource, err := resolveIdempotencyHashSource(run.input, idempotencyHashSource)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to serialize transaction for idempotency hash", err)
		logger.Log(ctx, libLog.LevelError, "Failed to serialize transaction for idempotency hash", libLog.Err(err))

		return nil, err
	}

	run.idempotencyHash = libCommons.HashSHA256(hashSource)

	idempotencyResult, err := uc.CreateOrCheckTransactionIdempotency(ctx, run.organizationID, run.ledgerID, run.idempotencyKey, run.idempotencyHash, run.idempotencyTTL)
	if err != nil {
		return nil, err
	}

	run.idempotencyInternalKey = idempotencyResult.InternalKey

	return idempotencyResult.Replay, nil
}

// normalizeSendLegs brings both legs into the form the downstream balance and
// operation matching needs: IsFrom on every source, default balance keys, concat
// aliases. It is also what folds fee legs — which the engine appends with BARE
// aliases and without IsFrom — into that same form. Both mutators are idempotent
// (ApplyDefaultBalanceKeys only fills empty keys, MutateConcatAliases skips
// already-concat'd aliases), so legs that were already normalized are untouched.
func normalizeSendLegs(run *createTransactionRun) {
	for i := range run.input.Send.Source.From {
		run.input.Send.Source.From[i].IsFrom = true
	}

	mtransaction.ApplyDefaultBalanceKeys(run.input.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(run.input.Send.Distribute.To)

	mtransaction.MutateConcatAliases(run.input.Send.Source.From)
	mtransaction.MutateConcatAliases(run.input.Send.Distribute.To)
}

// stageBalances seeds the backup queue, loads the balances behind the validated
// aliases, rejects direct operations on internal-scope balances, builds the balance
// operations, enriches them with the overdraft companions and enforces the ledger's
// accounting routes. It returns a ctx marked for primary reads so every later
// transactional read in the flow is served from the primary rather than a possibly
// stale replica.
//
// Each failure rolls back exactly what it has to: before the seed only the
// idempotency claim exists, after it the seed goes too.
func (uc *UseCase) stageBalances(ctx context.Context, span trace.Span, logger libLog.Logger, run *createTransactionRun) (context.Context, error) {
	err := uc.SendTransactionToRedisQueue(ctx, run.organizationID, run.ledgerID, run.transactionID, run.input, run.validate, run.status, run.action, run.transactionDate, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to send transaction to backup cache", err)
		logger.Log(ctx, libLog.LevelError, "Failed to send transaction to backup cache", libLog.Err(err))

		uc.rollbackCreateClaim(ctx, run)

		return ctx, pkg.ValidateBusinessError(err, constant.EntityTransaction)
	}

	// Mark the transactional-flow balance reads below so they can be served from
	// the primary, avoiding a stale replica read before the commit.
	ctx = readrouting.WithPrimaryRead(ctx)

	balances, err := uc.TransactionReader.GetBalances(ctx, run.organizationID, run.ledgerID, run.validate.Aliases)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balances", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get balances", libLog.Err(err))

		uc.rollbackCreateSeed(ctx, logger, run)

		return ctx, err
	}

	// Scope protection on the CREATE path: SendTransactionToRedisQueue above
	// runs with nil balances (the queue seed precedes GetBalances), so its
	// built-in scope guard is a no-op for user-created transactions. Re-check
	// here now that balances are loaded. Rejecting a direct operation on an
	// internal-scope balance BEFORE enrichment runs keeps the companion
	// balance isolated from client-initiated mutations.
	if err := rejectInternalScopeBalances(ctx, balances); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rejected transaction targeting internal-scope balance", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected transaction targeting internal-scope balance", libLog.Err(err))

		uc.rollbackCreateSeed(ctx, logger, run)

		return ctx, err
	}

	balanceOps := BuildBalanceOperations(ctx, run.organizationID, run.ledgerID, run.validate, balances)

	// Overdraft enrichment: when a source debit exceeds available funds on a
	// credit-direction balance with AllowOverdraft=true, append a debit op on
	// the companion #overdraft balance for the deficit. See
	// transaction_overdraft_enrichment.go for the full rationale. Disabled
	// balances and out-of-scope operations fall through as a no-op so legacy
	// transaction flows remain untouched.
	//
	// `companionFromTos` are returned so the caller can splice them into the
	// `fromTo` slice; without this, BuildOperations' match loop never emits an
	// Operation record for the companion balance and the audit trail is missing
	// the overdraft leg (DB balances still converge correctly, but
	// `response.operations` and Postgres `operation` rows do not include the
	// companion).
	balanceOps, companionFromTos, err := EnrichOverdraftOperations(ctx, run.organizationID, run.ledgerID, balanceOps,
		run.validate, uc.TransactionReader.GetBalances)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to enrich overdraft operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to enrich overdraft operations", libLog.Err(err))

		uc.rollbackCreateSeed(ctx, logger, run)

		return ctx, err
	}

	routeCache, err := uc.TransactionReader.ValidateAccountingRules(ctx, run.organizationID, run.ledgerID, balanceOps, run.validate, run.action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate accounting rules", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate accounting rules", libLog.Err(err))

		uc.rollbackCreateSeed(ctx, logger, run)

		return ctx, err
	}

	run.balances = balances
	run.balanceOps = balanceOps
	run.companionFromTos = companionFromTos
	run.routeCache = routeCache

	return ctx, nil
}

// finalizeCreatedTransaction turns the committed balance result into the
// transaction row: it splices the split-alias legs and the overdraft companions
// into fromTo, builds the operation records, updates the backup entry, writes the
// transaction and hands the idempotency slot and the audit queue their background
// work.
//
// Nothing here rolls back. The balance has already moved, so the idempotency key
// keeps duplicate mutations away and the backup queue is what reconstructs the
// transaction on failure.
func (uc *UseCase) finalizeCreatedTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, run *createTransactionRun) (*transaction.Transaction, error) {
	balancesBefore, balancesAfter := run.result.Before, run.result.After

	run.fromTo = append(run.fromTo, mtransaction.MutateSplitAliases(run.input.Send.Source.From)...)
	to := mtransaction.MutateSplitAliases(run.input.Send.Distribute.To)

	if run.status != constant.PENDING {
		run.fromTo = append(run.fromTo, to...)
	}

	// Splice the enrichment-produced companion FromTo entries into the slice
	// BEFORE BuildOperations runs. Each companion carries an AccountAlias in
	// concat form ("<i>#@alias#overdraft") that matches the Lua-returned
	// `balance.Alias`, so the `balances × fromTo` loop in BuildOperations
	// now emits one Operation record per companion balance mutation. This
	// is the audit-trail half of the enrichment contract; the balance-state
	// half is handled by the enrichment engine up above.
	run.fromTo = append(run.fromTo, run.companionFromTos...)

	amount := run.input.Send.Value

	tran := &transaction.Transaction{
		ID:                       run.transactionID.String(),
		ParentTransactionID:      buildParentTransactionID(run.parentTransactionID),
		OrganizationID:           run.organizationID.String(),
		LedgerID:                 run.ledgerID.String(),
		Description:              run.input.Description,
		Amount:                   &amount,
		AssetCode:                run.input.Send.Asset,
		ChartOfAccountsGroupName: run.input.ChartOfAccountsGroupName,
		CreatedAt:                run.transactionDate,
		UpdatedAt:                time.Now(),
		Route:                    run.input.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		RouteID:                  run.input.RouteID,
		FeesSkipped:              run.honoredFeeSkip,
		TracerSkipped:            run.honoredTracerSkip,
		Metadata:                 run.input.Metadata,
		Status: transaction.Status{
			Code:        run.status,
			Description: &run.status,
		},
	}

	operations, _, err := uc.BuildOperations(ctx, balancesBefore, balancesAfter, run.fromTo, run.input, *tran, run.validate, run.transactionDate, run.status == constant.NOTED, run.ledgerSettings.Accounting.ValidateRoutes, run.routeCache, run.action)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build operations", libLog.Err(err))

		// Idempotency key and backup queue entry are intentionally preserved here.
		// Balances were already mutated by the Lua script (ProcessBalanceOperations),
		// so the backup queue is the recovery mechanism — the Kiwi consumer will
		// reconstruct and persist the transaction from the backup entry.
		// Deleting the idempotency key would allow duplicate balance mutations on retry.
		return nil, err
	}

	// The companion overdraft balances participate in the transaction at the
	// ledger layer but are NOT user-facing sources or destinations — they are
	// system-managed liability ledgers. Filter them out of the alias-key
	// lists before stripping `#key` so `tran.Source` / `tran.Destination`
	// reflect only the client-submitted accounts (and do not produce duplicates
	// like `[@alice, @alice]` when the companion's bare alias collapses to
	// the same value after the strip).
	tran.Source = GetAliasWithoutKey(FilterCompanionAliases(run.validate.Sources))
	tran.Destination = GetAliasWithoutKey(FilterCompanionAliases(run.validate.Destinations))
	tran.Operations = operations

	uc.UpdateTransactionBackupOperations(ctx, run.organizationID, run.ledgerID, run.transactionID.String(), operations, run.action)

	// Build a shallow copy with the promoted status for persistence and cache.
	// CREATED is a transient status that the DB layer promotes to APPROVED;
	// the cache must reflect the final status for consistent GET reads.
	// The original tran keeps CREATED for the HTTP response and idempotency key.
	writeTran := *tran

	if run.status == constant.CREATED {
		approved := constant.APPROVED
		writeTran.Status = transaction.Status{Code: approved, Description: &approved}
	}

	uc.CreateWriteBehindTransaction(ctx, run.organizationID, run.ledgerID, &writeTran, run.input)

	err = uc.WriteTransaction(ctx, run.organizationID, run.ledgerID, &run.input, run.validate, balancesBefore, balancesAfter, &writeTran)
	if err != nil {
		// Log the original error for debugging. WriteTransaction may fail due to:
		// - msgpack serialization error
		// - RabbitMQ publish failure + DB fallback failure (async mode)
		// - Direct DB write failure (sync mode)
		// The sanitized error uses ErrMessageBrokerUnavailable as a generic
		// "persistence failed" signal — a more accurate error code should be
		// introduced to cover the sync/DB failure cases as well.
		libOpentelemetry.HandleSpanError(span, "Failed to write transaction", err)
		logger.Log(ctx, libLog.LevelError, "Failed to write transaction", libLog.String("transaction_id", tran.ID), libLog.Err(err))

		sanitizedErr := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, constant.EntityTransaction)

		return nil, sanitizedErr
	}

	bgCtx := tmcore.ContextWithTenantID(context.Background(), tmcore.GetTenantIDContext(ctx))

	go uc.SetTransactionIdempotencyValue(bgCtx, run.organizationID, run.ledgerID, run.idempotencyKey, run.idempotencyHash, *tran, run.idempotencyTTL)

	go uc.SendLogTransactionAuditQueue(bgCtx, operations, run.organizationID, run.ledgerID, tran.IDtoUUID())

	return tran, nil
}

// rollbackCreateClaim releases the idempotency slot. It is the compensation for
// every failure between the claim and the backup-queue seed, where nothing else
// has been written yet.
func (uc *UseCase) rollbackCreateClaim(ctx context.Context, run *createTransactionRun) {
	uc.deleteIdempotencyKey(ctx, run.idempotencyInternalKey)
}

// rollbackCreateSeed releases the idempotency slot and removes the backup-queue
// seed. It is the compensation for every failure between the seed and the balance
// commit. After the commit nothing is rolled back: the balance has already moved,
// the backup queue reconstructs the transaction and the idempotency key is what
// keeps a retry from mutating balances twice.
func (uc *UseCase) rollbackCreateSeed(ctx context.Context, logger libLog.Logger, run *createTransactionRun) {
	uc.deleteIdempotencyKey(ctx, run.idempotencyInternalKey)
	uc.RemoveTransactionFromRedisQueue(ctx, logger, run.organizationID, run.ledgerID, run.transactionID.String())
}

func (uc *UseCase) deleteIdempotencyKey(ctx context.Context, internalKey *string) {
	if internalKey != nil {
		_ = uc.TransactionRedisRepo.Del(ctx, *internalKey)
	}
}
