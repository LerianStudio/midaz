// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/skip"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// CreateTransaction is the transport-neutral create core. The shells read the path
// params + idempotency headers off the request envelope and project the result onto the
// typed Out; this core returns the built transaction and the idempotency `replayed` flag
// so the caller can set X-Idempotency-Replayed itself. The orchestration lives in
// executeCreateTransaction — this is the thin boundary in front of it.
func (uc *UseCase) CreateTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput mtransaction.Transaction, transactionStatus, idempotencyKey string, idempotencyTTL time.Duration, policy RouteVersionPolicy, idempotencyHashSource ...string) (*transaction.Transaction, bool, error) {
	params := &transactionPathParams{OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: uuid.Nil}

	return uc.executeCreateTransaction(ctx, params, transactionInput, transactionStatus, false, idempotencyKey, idempotencyTTL, policy, idempotencyHashSource...)
}

// IdempotencyDiscriminatorSep joins an action discriminator to the rest of an idempotency
// hash preimage (v2IdempotencyHashSource). A NUL byte can appear in neither an action label
// nor a JSON body, so two preimages built with it can never collide by concatenation.
const IdempotencyDiscriminatorSep = "\x00"

// resolveIdempotencyHashSource returns the string the idempotency hash is computed over:
// the non-empty override when supplied, else the canonical serialized transaction. Keying
// off a raw pre-translation body via the override is the v2 idempotency contract.
func resolveIdempotencyHashSource(transactionInput mtransaction.Transaction, override ...string) (string, error) {
	if len(override) > 0 && override[0] != "" {
		return override[0], nil
	}

	return libCommons.StructToJSONString(transactionInput)
}

// CreateRevertTransaction creates a reversal transaction. The action is forced
// to "revert" so that accounting route lookups use the revert rubrics instead
// of the status-derived action. Transport-neutral, mirroring CreateTransaction.
//
// The policy has to be carried in rather than fixed here, even though applyFees
// ignores it on a revert (it no-ops on isRevert=true regardless): the reserve
// anchor has no isRevert gate, so a /v2 revert must still reserve capacity while
// a /v1 revert must not reach the tracer at all.
func (uc *UseCase) CreateRevertTransaction(ctx context.Context, organizationID, ledgerID, parentTransactionID uuid.UUID, transactionInput mtransaction.Transaction, transactionStatus, idempotencyKey string, idempotencyTTL time.Duration, policy RouteVersionPolicy) (*transaction.Transaction, bool, error) {
	params := &transactionPathParams{OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: parentTransactionID}

	return uc.executeCreateTransaction(ctx, params, transactionInput, transactionStatus, true, idempotencyKey, idempotencyTTL, policy)
}

// resolveTransactionSkips resolves the two per-call control skips (fees, tracer)
// off the already-read ledger settings, with no extra I/O. Each skip is honored
// only when the request asks for it AND the ledger opts in via its override; a
// skip requested without the matching opt-in returns the 422 business error plus
// the log/span label naming the rejected control, so the caller emits a single
// error branch for both controls.
func resolveTransactionSkips(input mtransaction.Transaction, settings mmodel.LedgerSettings) (feeSkip, tracerSkip bool, rejectLabel string, err error) {
	feeSkip, err = skip.ResolveSkipFor("fees", input.Skip != nil && input.Skip.Fees, settings.Overrides.AllowFeeSkip)
	if err != nil {
		return false, false, "Fee skip not permitted", err
	}

	tracerSkip, err = skip.ResolveSkipFor("tracer", input.Skip != nil && input.Skip.Tracer, settings.Overrides.AllowTracerSkip)
	if err != nil {
		return false, false, "Tracer skip not permitted", err
	}

	return feeSkip, tracerSkip, "", nil
}

//nolint:gocyclo // Orchestration step with conditional branches per transaction type; refactor candidate.
func (uc *UseCase) executeCreateTransaction(ctx context.Context, params *transactionPathParams, transactionInput mtransaction.Transaction, transactionStatus string, isRevert bool, idempotencyKey string, idempotencyTTL time.Duration, policy RouteVersionPolicy, idempotencyHashSource ...string) (*transaction.Transaction, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction.orchestrate")
	defer span.End()

	transactionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate transaction id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to generate transaction id", libLog.Err(err))

		return nil, false, err
	}

	transactionDate, err := mtransaction.CheckTransactionDate(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction date validation failed", err)

		return nil, false, err
	}

	spanattr.RecordSafePayloadAttributes(span, transactionInput)

	if transactionInput.Send.Value.LessThanOrEqual(decimal.Zero) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction value must be greater than zero", err)
		logger.Log(ctx, libLog.LevelWarn, "Transaction value must be greater than zero", libLog.String("value", transactionInput.Send.Value.String()))

		return nil, false, err
	}

	mtransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	// Idempotency: extract key/TTL from HTTP headers, hash the request body,
	// then check or claim the idempotency slot in Redis.
	//
	// The hash is intentionally computed over the RAW pre-fee payload (before
	// the fee seam below mutates transactionInput.Send). Fees are deterministic
	// given the same raw input + the same package configuration, so the raw
	// body is the stable identity of the request. Two consequences are accepted
	// by design (P4-T15): (1) package-config churn — if package config changes
	// between two identical-key requests, the replay returns the FIRST
	// fee-inclusive result (idempotency wins over recomputation); (2)
	// deleted-package near-miss — a NON-replay request (different key, same
	// body) issued after a package DELETE recomputes against the now-deleted
	// package and yields a different fee outcome, which is correct because the
	// hash keys on the raw body, not the package version. Package version is
	// deliberately NOT part of the key.
	//
	// idempotencyKey/idempotencyTTL are resolved by the transport and passed in; the hash
	// is computed here over the idempotency hash SOURCE resolved by
	// resolveIdempotencyHashSource. An optional override keys the hash off the raw body as
	// submitted; with no override the source is the canonical transactionInput.
	// The HashSHA256 mechanism is the same regardless of which source is used.
	hashSource, err := resolveIdempotencyHashSource(transactionInput, idempotencyHashSource...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to serialize transaction for idempotency hash", err)
		logger.Log(ctx, libLog.LevelError, "Failed to serialize transaction for idempotency hash", libLog.Err(err))

		return nil, false, err
	}

	idempotencyHash := libCommons.HashSHA256(hashSource)

	idempotencyResult, err := uc.CreateOrCheckTransactionIdempotency(ctx, params.OrganizationID, params.LedgerID, idempotencyKey, idempotencyHash, idempotencyTTL)
	if err != nil {
		return nil, false, err
	}

	if idempotencyResult.Replay != nil {
		return idempotencyResult.Replay, true, nil
	}

	// First validate: rejects malformed source/distribute on the RAW input
	// before fees are computed. Its Responses value is intentionally superseded
	// by the post-fee re-validation below (the fee engine mutates the send), so
	// only the error is consumed here. The binding is kept (not `_, err`) to
	// preserve the single-`:=`/single-`=` seam shape the structural gate
	// (transaction_fee_seam_structure_test.go) enforces.
	//nolint:staticcheck,wastedassign // first validate's value is deliberately superseded by the post-fee re-validation; only its error gates malformed input before fees run.
	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		return nil, false, err
	}

	// Ledger settings (Redis cache-aside) are read once here, above the fee seam.
	// They carry the per-call skip opt-ins (Overrides), so resolving the skips
	// off this single read keeps the gate free of extra I/O and lets an honored
	// fee skip bypass the engine entirely — before any package lookup.
	ledgerSettings, err := uc.TransactionReader.GetParsedLedgerSettings(ctx, params.OrganizationID, params.LedgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		return nil, false, err
	}

	// Resolve the two per-call skips (fees, tracer) once, off the settings just
	// read — no extra I/O. An honored fee skip short-circuits applyFees below
	// before the fee package lookup; an honored tracer skip short-circuits the
	// reserve anchor. A skip requested without the per-ledger opt-in is a 422:
	// release the idempotency key and reject.
	honoredFeeSkip, honoredTracerSkip, skipRejectLabel, err := resolveTransactionSkips(transactionInput, ledgerSettings)
	if err != nil {
		spanattr.HandleSpanByErrorClass(span, skipRejectLabel, err)
		logger.Log(ctx, libLog.LevelWarn, skipRejectLabel, libLog.Err(err))

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		return nil, false, err
	}

	// Record the resolved skips as system observations (not request inputs): they
	// reflect what the two-key gate actually honored, and they are persisted to the
	// transaction row below for the durable audit trail.
	// The two *_route_eligible attributes are deliberately NOT folded into the matching
	// *_skipped flags: those are persisted on the transaction row as the audit trail of a
	// skip the CLIENT asked for, so marking them true on every /v1 create would record a
	// claim never made. The two reasons a control did not run stay distinguishable.
	span.SetAttributes(
		attribute.Bool("app.transaction.fees_skipped", honoredFeeSkip),
		attribute.Bool("app.transaction.tracer_skipped", honoredTracerSkip),
		attribute.Bool("app.transaction.fees_route_eligible", policy == RouteV2),
		attribute.Bool("app.transaction.tracer_route_eligible", policy == RouteV2),
	)

	// Fee seam: drive the in-process fee engine over the validated transaction,
	// mutating transactionInput.Send.* (fee legs + moved Send.Value on
	// deductible fees). No-op on RouteV1 (the /v1 contract carries no fee
	// engine), on isRevert (the reverse transaction already carries reversed fee
	// legs from TransactionRevert) and on an honored fee skip (which bypasses the
	// engine before its package lookup). The settings read + skip resolution above
	// precede this seam; the seam still runs before the single validate
	// reassignment below, which is upstream of
	// PropagateRouteValidation — that mutator decorates the post-fee validate,
	// and every downstream consumer reads the same pointer. applyFees resolves
	// the tenant's fee DB internally, only once it has decided fees actually
	// apply, so the MT tenant resolution rides inside the same gate as the fee
	// computation.
	if err = uc.applyFees(ctx, &transactionInput, params.OrganizationID, params.LedgerID, policy, isRevert, transactionStatus == constant.NOTED, honoredFeeSkip); err != nil {
		spanattr.HandleSpanByErrorClass(span, "Failed to apply fees", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to apply fees", libLog.Err(err))

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		return nil, false, err
	}

	// Normalize the fee-mutated send: applyFees rebuilds Source.From/Distribute.To
	// from the engine output with BARE aliases and without IsFrom, so the same
	// normalization the raw input received (default balance keys, IsFrom on
	// sources, concat aliases) must run again over the fee-inclusive legs before
	// the second validate. Both mutators are idempotent — ApplyDefaultBalanceKeys
	// only fills empty keys and MutateConcatAliases skips already-concat'd aliases
	// — so the original legs are untouched and only the appended fee legs are
	// brought into the concat form the downstream balance/operation matching needs.
	for i := range transactionInput.Send.Source.From {
		transactionInput.Send.Source.From[i].IsFrom = true
	}

	mtransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	mtransaction.MutateConcatAliases(transactionInput.Send.Source.From)
	mtransaction.MutateConcatAliases(transactionInput.Send.Distribute.To)

	// Re-run validation on the fee-mutated input. This is a single = reassignment
	// of the existing validate variable (a *mtransaction.Responses pointer), so
	// the fee-inclusive state by construction reaches every downstream reader of
	// validate through WriteTransaction. It MUST NOT be a := rebind.
	validate, err = mtransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate fee-inclusive send source and distribute", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate fee-inclusive send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		return nil, false, err
	}

	// Build the concat-form fromTo from the FEE-INCLUSIVE, normalized send. This
	// runs after applyFees + the second validate so the slice carries the fee
	// legs in the same "<index>#alias#balanceKey" form that BuildBalanceOperations
	// keys the validate maps by and that the Lua-returned balances carry — without
	// it the `balances × fromTo` match loop in BuildOperations never emits the fee
	// Operation rows. The aliases are already concat'd in place above; this read
	// is idempotent.
	var fromTo []mtransaction.FromTo

	fromTo = append(fromTo, mtransaction.MutateConcatAliases(transactionInput.Send.Source.From)...)
	to := mtransaction.MutateConcatAliases(transactionInput.Send.Distribute.To)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	if ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, validate, transactionStatus)
	}

	action := mtransaction.StatusToAction(transactionStatus)
	if isRevert {
		action = constant.ActionRevert
	}

	err = uc.SendTransactionToRedisQueue(ctx, params.OrganizationID, params.LedgerID, transactionID, transactionInput, validate, transactionStatus, action, transactionDate, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to send transaction to backup cache", err)
		logger.Log(ctx, libLog.LevelError, "Failed to send transaction to backup cache", libLog.Err(err))

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)

		return nil, false, pkg.ValidateBusinessError(err, constant.EntityTransaction)
	}

	// Mark the transactional-flow balance reads below so they can be served from
	// the primary, avoiding a stale replica read before the commit.
	ctx = readrouting.WithPrimaryRead(ctx)

	balances, err := uc.TransactionReader.GetBalances(ctx, params.OrganizationID, params.LedgerID, validate.Aliases)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balances", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get balances", libLog.Err(err))

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		uc.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		return nil, false, err
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

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		uc.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		return nil, false, err
	}

	balanceOps := BuildBalanceOperations(ctx, params.OrganizationID, params.LedgerID, validate, balances)

	// Overdraft enrichment: when a source debit exceeds available funds on a
	// credit-direction balance with AllowOverdraft=true, append a debit op on
	// the companion #overdraft balance for the deficit. See
	// transaction_overdraft_enrichment.go for the full rationale. Disabled
	// balances and out-of-scope operations fall through as a no-op so legacy
	// transaction flows remain untouched.
	//
	// `companionFromTos` are returned so the caller can splice them into the
	// `fromTo` slice built below; without this, BuildOperations' match loop
	// never emits an Operation record for the companion balance and the
	// audit trail is missing the overdraft leg (DB balances still converge
	// correctly, but `response.operations` and Postgres `operation` rows do
	// not include the companion).
	balanceOps, companionFromTos, err := EnrichOverdraftOperations(ctx, params.OrganizationID, params.LedgerID, balanceOps,
		validate, uc.TransactionReader.GetBalances)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to enrich overdraft operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to enrich overdraft operations", libLog.Err(err))

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		uc.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		return nil, false, err
	}

	routeCache, err := uc.TransactionReader.ValidateAccountingRules(ctx, params.OrganizationID, params.LedgerID, balanceOps, validate, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate accounting rules", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate accounting rules", libLog.Err(err))

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		uc.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		return nil, false, err
	}

	// Reserve anchor (F3-T13): hold usage-limit capacity against the
	// FEE-INCLUSIVE transaction immediately before the balance commit. No-op on
	// RouteV1 (the /v1 contract carries no tracer), which is the anchor's first
	// gate — a /v1 create builds no reserve request and dials nothing. This
	// observes the validated fee-inclusive send amount; it never mutates
	// Send.Value or balance state. A DENIED decision (enforce) or a fail-closed
	// unavailable tracer rejects here, before ProcessBalanceOperations moves any
	// balance, releasing the idempotency key and the Redis-queue seed exactly as
	// the ProcessBalanceOperations failure path does below. The returned handle
	// is confirmed on success / released on abort at the post-commit transport.
	reservation := uc.reserveTransaction(ctx, span, logger, ledgerSettings.Tracer, transactionID,
		transactionInput.Send.Value, transactionInput.Send.Asset, firstSourceAccountID(validate.Sources, balances),
		transactionDate, reservationTTLForStatus(transactionStatus), policy, honoredTracerSkip)
	if reservation.Kind == reservationReject {
		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		uc.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		return nil, false, reservation.Err
	}

	result, err := uc.ProcessBalanceOperations(ctx, ProcessBalanceOperationsInput{
		OrganizationID:    params.OrganizationID,
		LedgerID:          params.LedgerID,
		TransactionID:     transactionID,
		TransactionInput:  &transactionInput,
		Validate:          validate,
		BalanceOperations: balanceOps,
		TransactionStatus: transactionStatus,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to process balance operations", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to process balance operations", libLog.Err(err))

		uc.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)
		uc.RemoveTransactionFromRedisQueue(ctx, logger, params.OrganizationID, params.LedgerID, transactionID.String())

		// The balance commit failed (no funds moved), so return the held
		// reservation capacity. Best-effort: a transport failure here is
		// reconciled by the TTL reaper.
		uc.releaseReservations(ctx, span, logger, reservation.Handle)

		return nil, false, err
	}

	// Confirm anchor (F3-T14, success phase): the balance commit succeeded, so
	// the held capacity is consumed. PENDING transactions defer the confirm to
	// /commit (and release to /cancel) — see F3-T15 — so the reservation stays
	// open here for them. Downstream BuildOperations/WriteTransaction failures
	// do NOT release: the balance has already moved and the backup queue
	// reconstructs the transaction, so the consumed capacity stands.
	if transactionStatus != constant.PENDING {
		uc.confirmReservations(ctx, span, logger, reservation.Handle)
	}

	balancesBefore, balancesAfter := result.Before, result.After

	fromTo = append(fromTo, mtransaction.MutateSplitAliases(transactionInput.Send.Source.From)...)
	to = mtransaction.MutateSplitAliases(transactionInput.Send.Distribute.To)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	// Splice the enrichment-produced companion FromTo entries into the slice
	// BEFORE BuildOperations runs. Each companion carries an AccountAlias in
	// concat form ("<i>#@alias#overdraft") that matches the Lua-returned
	// `balance.Alias`, so the `balances × fromTo` loop in BuildOperations
	// now emits one Operation record per companion balance mutation. This
	// is the audit-trail half of the enrichment contract; the balance-state
	// half is handled by the enrichment engine up above.
	fromTo = append(fromTo, companionFromTos...)

	amount := transactionInput.Send.Value

	tran := &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      buildParentTransactionID(params.TransactionID),
		OrganizationID:           params.OrganizationID.String(),
		LedgerID:                 params.LedgerID.String(),
		Description:              transactionInput.Description,
		Amount:                   &amount,
		AssetCode:                transactionInput.Send.Asset,
		ChartOfAccountsGroupName: transactionInput.ChartOfAccountsGroupName,
		CreatedAt:                transactionDate,
		UpdatedAt:                time.Now(),
		Route:                    transactionInput.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		RouteID:                  transactionInput.RouteID,
		FeesSkipped:              honoredFeeSkip,
		TracerSkipped:            honoredTracerSkip,
		Metadata:                 transactionInput.Metadata,
		Status: transaction.Status{
			Code:        transactionStatus,
			Description: &transactionStatus,
		},
	}

	operations, _, err := uc.BuildOperations(ctx, balancesBefore, balancesAfter, fromTo, transactionInput, *tran, validate, transactionDate, transactionStatus == constant.NOTED, ledgerSettings.Accounting.ValidateRoutes, routeCache, action)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build operations", libLog.Err(err))

		// Idempotency key and backup queue entry are intentionally preserved here.
		// Balances were already mutated by the Lua script (ProcessBalanceOperations),
		// so the backup queue is the recovery mechanism — the Kiwi consumer will
		// reconstruct and persist the transaction from the backup entry.
		// Deleting the idempotency key would allow duplicate balance mutations on retry.
		return nil, false, err
	}

	// The companion overdraft balances participate in the transaction at the
	// ledger layer but are NOT user-facing sources or destinations — they are
	// system-managed liability ledgers. Filter them out of the alias-key
	// lists before stripping `#key` so `tran.Source` / `tran.Destination`
	// reflect only the client-submitted accounts (and do not produce duplicates
	// like `[@alice, @alice]` when the companion's bare alias collapses to
	// the same value after the strip).
	tran.Source = GetAliasWithoutKey(FilterCompanionAliases(validate.Sources))
	tran.Destination = GetAliasWithoutKey(FilterCompanionAliases(validate.Destinations))
	tran.Operations = operations

	uc.UpdateTransactionBackupOperations(ctx, params.OrganizationID, params.LedgerID, transactionID.String(), operations, action)

	// Build a shallow copy with the promoted status for persistence and cache.
	// CREATED is a transient status that the DB layer promotes to APPROVED;
	// the cache must reflect the final status for consistent GET reads.
	// The original tran keeps CREATED for the HTTP response and idempotency key.
	writeTran := *tran

	if transactionStatus == constant.CREATED {
		approved := constant.APPROVED
		writeTran.Status = transaction.Status{Code: approved, Description: &approved}
	}

	uc.CreateWriteBehindTransaction(ctx, params.OrganizationID, params.LedgerID, &writeTran, transactionInput)

	err = uc.WriteTransaction(ctx, params.OrganizationID, params.LedgerID, &transactionInput, validate, balancesBefore, balancesAfter, &writeTran)
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

		return nil, false, sanitizedErr
	}

	bgCtx := tmcore.ContextWithTenantID(context.Background(), tmcore.GetTenantIDContext(ctx))

	go uc.SetTransactionIdempotencyValue(bgCtx, params.OrganizationID, params.LedgerID, idempotencyKey, idempotencyHash, *tran, idempotencyTTL)

	go uc.SendLogTransactionAuditQueue(bgCtx, operations, params.OrganizationID, params.LedgerID, tran.IDtoUUID())

	return tran, false, nil
}

func (uc *UseCase) deleteIdempotencyKey(ctx context.Context, internalKey *string) {
	if internalKey != nil {
		_ = uc.TransactionRedisRepo.Del(ctx, *internalKey)
	}
}
