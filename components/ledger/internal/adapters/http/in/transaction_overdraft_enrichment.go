// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// companionBalanceLoader abstracts the lookup used by overdraft enrichment to
// fetch the system-managed companion balance. It mirrors the signature of the
// existing query.GetBalances call so the enrichment code can be exercised in
// tests without spinning up the full query use case.
type companionBalanceLoader func(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error)

// rejectInternalScopeBalances returns a business error (0168) when any loaded
// balance is marked BalanceScope=internal. Internal balances (e.g. the
// auto-created overdraft companion) are system-managed and may only be
// mutated by the enrichment engine; direct client-initiated operations must
// be rejected before they enter the Lua atomic path.
//
// Mirrors the scope guard in
// services/command/create_balance_transaction_operations_async.go:312-324
// which runs during state transitions. The CREATE path reaches
// SendTransactionToRedisQueue with nil balances, so its built-in guard is a
// no-op for brand-new transactions — this helper fills the gap by enforcing
// the same invariant once GetBalances has returned.
func rejectInternalScopeBalances(ctx context.Context, balances []*mmodel.Balance) error {
	logger := libCommons.NewLoggerFromContext(ctx)

	for _, b := range balances {
		if b == nil || b.Settings == nil {
			continue
		}

		if b.Settings.BalanceScope != mmodel.BalanceScopeInternal {
			continue
		}

		logger.Log(ctx, libLog.LevelWarn,
			"Rejected transaction targeting internal-scope balance",
			libLog.String("alias", b.Alias),
			libLog.String("key", b.Key))

		// Surface the canonical error so the HTTP layer returns 422/0168 and
		// any upstream client sees a consistent code whether the rejection
		// came from the CREATE path or a state transition.
		return pkg.ValidateBusinessError(constant.ErrDirectOperationOnInternalBalance, constant.EntityBalance, b.Alias)
	}

	return nil
}

// enrichOverdraftOperations implements the transaction enrichment contract
// described in the TRD (2.2 — Enrichment Engine): when a debit on a
// direction=credit balance exceeds available funds and the balance has
// overdraft enabled, we append a *debit* operation on the companion
// "#overdraft" balance for the deficit amount.
//
// Rationale for appending (not splitting the original op):
//   - The Lua atomic script already owns the floor-at-zero math and the
//     overdraft_used accrual on the credit balance. Splitting the amount in
//     Go would have two independent places computing the same split, making
//     it impossible to keep them in lock-step under concurrency (TRD 242-246,
//     "dual source of truth" risk).
//   - Accounting parity: the appended op is an *extra* balance mutation that
//     never participates in ValidateSendSourceAndDistribute (which ran on user
//     input before we reach this point). The user-visible send value therefore
//     still matches the destination credit total, and only the balance layer
//     observes the additional overdraft-companion mutation.
//
// Net effect on the atomic script:
//   - Original op on alias#default triggers the Lua overdraft branch — floors
//     Available at 0 and accrues `overdraft_used` by the deficit on the
//     credit balance.
//   - Appended op on alias#overdraft applies a DEBIT on a direction=debit
//     companion balance, which increments its Available by the deficit
//     (direction=debit semantics: DEBIT grows the liability).
//
// If the companion balance cannot be found (e.g., was never auto-created
// because AllowOverdraft was set before companion provisioning landed), the
// enrichment skips the pair silently; the Lua script's built-in split still
// keeps the default balance correct, only the companion balance's Available
// will lag — surfaced by the existing E2E tests and balance listing.
//
// Side effects on `validate`:
//
// `mtransaction.ValidateBalancesRules` enforces `len(balances) ==
// len(validate.From) + len(validate.To)` as the first check after
// deduplication. Appending a companion op adds a fresh alias to the balance
// list, so we MUST mirror that alias as a pseudo-entry in `validate.From`
// (the companion is always on the source side of the transaction) to keep
// the count invariant. The pseudo-entry re-uses the companion's amount,
// direction, and transaction metadata; it is not billed against the user-
// visible send total because the split happens after
// ValidateSendSourceAndDistribute has already signed off on the totals.
//
// Return values:
//   - enriched balanceOps (primary ops + appended companion ops)
//   - companion FromTo entries that the handler MUST append to its fromTo
//     slice before calling BuildOperations, so the operation-record loop
//     (balances × fromTo) generates a persisted Operation for each companion.
//     Without this, the balance tables converge correctly but the audit trail
//     is missing the overdraft leg (TRD PRD §1 "Enriched Transaction Flow").
func enrichOverdraftOperations(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	balanceOps []mmodel.BalanceOperation,
	validate *mtransaction.Responses,
	loader companionBalanceLoader,
) ([]mmodel.BalanceOperation, []mtransaction.FromTo, error) {
	logger := libCommons.NewLoggerFromContext(ctx)

	debits := collectOverdraftDebitSplits(balanceOps)
	refunds := collectOverdraftRefundSplits(balanceOps)

	if len(debits) == 0 && len(refunds) == 0 {
		return balanceOps, nil, nil
	}

	// Deduplicate companion alias lookups: multiple source ops on the same
	// alias#default balance (e.g. double-entry PENDING splits) share a single
	// companion balance. Loading it once keeps the request path cheap. The
	// same balance can also appear as both a debit and a refund candidate
	// across a transaction (e.g. a multi-leg transfer), so the alias set is
	// built from the union of both.
	aliasSet := make(map[string]struct{}, len(debits)+len(refunds))

	for _, s := range debits {
		aliasSet[s.companionAliasKey] = struct{}{}
	}

	for _, r := range refunds {
		aliasSet[r.companionAliasKey] = struct{}{}
	}

	uniqueAliases := make([]string, 0, len(aliasSet))
	for k := range aliasSet {
		uniqueAliases = append(uniqueAliases, k)
	}

	companions, err := loader(ctx, organizationID, ledgerID, uniqueAliases)
	if err != nil {
		return nil, nil, fmt.Errorf("load overdraft companion balances: %w", err)
	}

	companionByAliasKey := make(map[string]*mmodel.Balance, len(companions))
	for _, b := range companions {
		companionByAliasKey[b.Alias+"#"+b.Key] = b
	}

	companionFromTos := make([]mtransaction.FromTo, 0, len(debits)+len(refunds))

	for _, s := range debits {
		companion, ok := companionByAliasKey[s.companionAliasKey]
		if !ok {
			// No companion found — log and skip. This is recoverable at the
			// Lua layer (default balance still gets overdraft_used accrual)
			// but the overdraft balance will not see the liability increase
			// until a future reconciliation path picks it up.
			logger.Log(ctx, libLog.LevelWarn,
				"Overdraft companion balance not found; skipping debit enrichment",
				libLog.String("alias_key", s.companionAliasKey))

			continue
		}

		companionOp := buildCompanionDebitOp(organizationID, ledgerID, s, companion)
		balanceOps = append(balanceOps, companionOp)

		// Inherit the primary source op's RouteID (if any) so the companion
		// carries an accounting route through validation. Mirrors the
		// hold/commit/cancel pattern: same routeId, different action
		// determines the rubric. Pulled from validate.OperationRoutesFrom
		// (keyed by the concat alias of the primary op).
		primaryRouteID := lookupRouteID(validate, s.source.Alias, true /* isFrom */)

		if validate != nil {
			registerCompanionInValidate(validate, s.source, companionOp, primaryRouteID)
		}

		companionFromTos = append(companionFromTos,
			buildCompanionFromTo(s.source, companionOp, true /* isFrom */, primaryRouteID))
	}

	for _, r := range refunds {
		companion, ok := companionByAliasKey[r.companionAliasKey]
		if !ok {
			logger.Log(ctx, libLog.LevelWarn,
				"Overdraft companion balance not found; skipping refund enrichment",
				libLog.String("alias_key", r.companionAliasKey))

			continue
		}

		companionOp := buildCompanionCreditOp(organizationID, ledgerID, r, companion)
		balanceOps = append(balanceOps, companionOp)

		// Mirror of the debit path: the refund companion inherits the
		// destination primary's RouteID from validate.OperationRoutesTo.
		primaryRouteID := lookupRouteID(validate, r.destination.Alias, false /* isFrom */)

		if validate != nil {
			registerCompanionInValidateTo(validate, r.destination, companionOp, primaryRouteID)
		}

		companionFromTos = append(companionFromTos,
			buildCompanionFromTo(r.destination, companionOp, false /* isFrom */, primaryRouteID))
	}

	return balanceOps, companionFromTos, nil
}

// lookupRouteID returns the primary op's routeID from validate's operation
// route maps, or "" when not present (route validation disabled or legacy
// transaction shape without routeId). The `primaryAlias` is the concat-form
// key used both as the Alias on the BalanceOperation and as the key in
// validate.From / validate.OperationRoutesFrom.
//
// Returning an empty string when the primary has no routeID is the correct
// fallback: downstream helpers (buildCompanionFromTo,
// registerCompanionInValidate) treat "" as "no route" and leave the
// FromTo.RouteID pointer nil so transactions without route validation keep
// working unchanged.
func lookupRouteID(validate *mtransaction.Responses, primaryAlias string, isFrom bool) string {
	if validate == nil {
		return ""
	}

	if isFrom {
		return validate.OperationRoutesFrom[primaryAlias]
	}

	return validate.OperationRoutesTo[primaryAlias]
}

// registerCompanionInValidate mirrors the companion op into `validate.From`
// and `validate.Sources` so downstream count-based invariants
// (ValidateBalancesRules len check, alias lookup paths) treat the companion
// as a first-class source balance.
//
// Key-shape convention (must match user ops to avoid duplicate counters):
//   - `validate.From` is keyed by the concat-form alias ("0#@alice#…") — so
//     the key is `companionOp.Alias` as-is (the builder now emits the prefix).
//   - `validate.Sources` / `validate.Aliases` use the bare alias-key form
//     ("@alice#overdraft") — matching how `CalculateTotal` populates them
//     for user-submitted entries via `AliasKey(SplitAlias(...), BalanceKey)`.
//
// `primaryRouteID` carries the routeID from the primary op's FromTo entry
// (as resolved by ValidateSendSourceAndDistribute). When non-empty we mirror
// it into `validate.OperationRoutesFrom` keyed by the companion's concat
// alias so the route-validation step (validateAccountRules) finds a matching
// route instead of rejecting with 0117 (Accounting Route Not Found). Passing
// an empty string keeps the map entry absent, preserving the legacy no-route
// behaviour for transactions where route validation is disabled.
func registerCompanionInValidate(validate *mtransaction.Responses, _ mmodel.BalanceOperation, companionOp mmodel.BalanceOperation, primaryRouteID string) {
	if validate.From == nil {
		validate.From = make(map[string]mtransaction.Amount, 1)
	}

	// First-wins: double-entry splits can produce two source ops on the same
	// alias (e.g. DEBIT + ONHOLD for PENDING). Both would lead us here but
	// only one companion entry is needed — the amount is identical.
	if _, exists := validate.From[companionOp.Alias]; !exists {
		validate.From[companionOp.Alias] = companionOp.Amount
	}

	if primaryRouteID != "" {
		if validate.OperationRoutesFrom == nil {
			validate.OperationRoutesFrom = make(map[string]string, 1)
		}

		// First-wins mirrors the From-map convention above: double-entry
		// splits emitted on the same companion alias carry the same
		// routeID, so overwriting would be a no-op anyway.
		if _, exists := validate.OperationRoutesFrom[companionOp.Alias]; !exists {
			validate.OperationRoutesFrom[companionOp.Alias] = primaryRouteID
		}
	}

	bareAlias := stripIndexPrefix(companionOp.Alias)

	for _, existing := range validate.Sources {
		if existing == bareAlias {
			return
		}
	}

	validate.Sources = append(validate.Sources, bareAlias)

	if !containsString(validate.Aliases, bareAlias) {
		validate.Aliases = append(validate.Aliases, bareAlias)
	}
}

// indexPrefix extracts the "<index>#" segment from an alias such as
// "0#@alice#default". If the alias has no leading index (defensive path —
// should not happen in production), an empty prefix is returned so the
// companion entry still lands in validate.From, albeit without a positional
// tag. Returning "" rather than panicking keeps the enrichment non-fatal in
// the face of unexpected alias shapes.
func indexPrefix(alias string) string {
	for i := 0; i < len(alias); i++ {
		if alias[i] == '#' {
			return alias[:i+1]
		}
	}

	return ""
}

// containsString is a tiny helper kept local to the enrichment file so we do
// not bloat the http/in package surface with another utility.
func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}

	return false
}

// overdraftSplit captures the information required to enrich a single source
// operation with its companion debit op. It is intentionally a plain value
// type so the enrichment step has a single, testable seam.
type overdraftSplit struct {
	// source is the original user-initiated op on the credit balance. We keep
	// a reference so the companion op can inherit transaction metadata
	// (TransactionType, RouteID pointers, etc.) without re-deriving them.
	source mmodel.BalanceOperation
	// deficit is the positive amount (= source.Amount.Value - balance.Available)
	// that overflows the available funds and must be routed to the overdraft
	// companion. Always strictly greater than zero when the split is emitted.
	deficit decimal.Decimal
	// companionAliasKey is the "<alias>#overdraft" string used both as the
	// alias lookup key on the loaded companion balance and as the alias
	// attached to the emitted BalanceOperation.
	companionAliasKey string
}

// collectOverdraftDebitSplits walks the primary balance operations and returns
// the split descriptors for every source op whose debit amount exceeds the
// credit balance's available funds with overdraft enabled.
//
// The function is deliberately read-only: it does not mutate balanceOps. All
// mutations (appending the companion op) happen once on the caller side, so
// the rest of the pipeline still sees the primary ops in their original form.
func collectOverdraftDebitSplits(balanceOps []mmodel.BalanceOperation) []overdraftSplit {
	splits := make([]overdraftSplit, 0)

	for _, op := range balanceOps {
		if op.Balance == nil {
			continue
		}

		// DetectOverdraftSplit consumes the transaction-layer Balance shape,
		// so we convert lazily only when the op is a DEBIT. This keeps the
		// common (non-overdraft) path free of allocation.
		if op.Amount.Operation != libConstants.DEBIT {
			continue
		}

		txBalance := op.Balance.ToTransactionBalance()
		if txBalance == nil {
			continue
		}

		split, deficit := mtransaction.DetectOverdraftSplit(op.Amount, *txBalance)
		if !split {
			continue
		}

		splits = append(splits, overdraftSplit{
			source:            op,
			deficit:           deficit,
			companionAliasKey: op.Balance.Alias + "#" + constant.OverdraftBalanceKey,
		})
	}

	return splits
}

// buildCompanionDebitOp constructs the BalanceOperation that mirrors the
// overdraft deficit onto the direction=debit companion balance. The operation
// inherits transaction metadata (TransactionType, route ids, pending flag)
// from the source op so downstream consumers (aggregation, accounting entries)
// treat the companion as part of the same logical transaction.
//
// Direction is set to debit explicitly: the companion balance is always
// direction=debit (TRD 2.2 table row 2), and passing the direction lets the
// state machine skip the inference path.
//
// Alias shape: the companion's Alias is built in the SAME positional-prefix
// form as user-initiated ops (e.g. "0#@alice#overdraft") — sharing the
// originating source's positional index. The Lua script copies this string
// verbatim into the returned balance record, and BuildOperations (later in
// the pipeline) matches it against `fromTo[i].AccountAlias`. Without the
// positional prefix the match loop `balance.Alias == fromTo[i].AccountAlias`
// fails and no operation record is emitted for the companion balance — the
// audit trail would converge on the wrong answer even though Lua did the
// right thing.
func buildCompanionDebitOp(organizationID, ledgerID uuid.UUID, s overdraftSplit, companion *mmodel.Balance) mmodel.BalanceOperation {
	companionAmount := s.source.Amount
	companionAmount.Value = s.deficit
	companionAmount.Operation = libConstants.DEBIT
	companionAmount.Direction = constant.DirectionDebit

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID,
		companion.Alias+"#"+companion.Key)

	return mmodel.BalanceOperation{
		Balance:     companion,
		Alias:       indexPrefix(s.source.Alias) + s.companionAliasKey,
		Amount:      companionAmount,
		InternalKey: internalKey,
	}
}

// overdraftRefund captures the information required to enrich a destination
// credit op with its companion repayment. Mirrors overdraftSplit but carries
// the repayment amount (min(credit, overdraft_used)) instead of the debit
// deficit.
type overdraftRefund struct {
	// destination is the original user-initiated CREDIT op on the credit
	// balance that currently carries outstanding overdraft. We keep it so
	// the companion op can inherit metadata (asset, TransactionType, etc.).
	destination mmodel.BalanceOperation
	// repayAmount is min(credit, overdraft_used) — the portion of the credit
	// that flows to the companion balance (reducing its Available under
	// direction=debit CREDIT semantics). Always greater than zero when the
	// refund is emitted.
	repayAmount decimal.Decimal
	// companionAliasKey is "<alias>#overdraft" for the balance that carries
	// the outstanding liability we are repaying.
	companionAliasKey string
}

// collectOverdraftRefundSplits walks the primary balance operations and
// returns a refund descriptor for every destination op whose credit amount
// should repay outstanding overdraft on a direction=credit balance with
// OverdraftUsed > 0. The function is read-only; the caller appends the
// companion CREDIT op once the companion balance has been loaded.
func collectOverdraftRefundSplits(balanceOps []mmodel.BalanceOperation) []overdraftRefund {
	refunds := make([]overdraftRefund, 0)

	for _, op := range balanceOps {
		if op.Balance == nil {
			continue
		}

		if op.Amount.Operation != libConstants.CREDIT {
			continue
		}

		txBalance := op.Balance.ToTransactionBalance()
		if txBalance == nil {
			continue
		}

		split, repay, _ := mtransaction.DetectRefundSplit(op.Amount, *txBalance)
		if !split {
			continue
		}

		refunds = append(refunds, overdraftRefund{
			destination:       op,
			repayAmount:       repay,
			companionAliasKey: op.Balance.Alias + "#" + constant.OverdraftBalanceKey,
		})
	}

	return refunds
}

// buildCompanionCreditOp constructs the CREDIT op on the direction=debit
// overdraft companion. A CREDIT on a direction=debit balance DECREASES its
// Available (the liability is being paid down), so this operation matches
// the causal ordering described in the PRD (§ Overdraft Repayment): the
// overdraft is repaid first, then the remainder (if any) lands on the
// default balance via the primary op.
//
// Direction is set to "credit" (the operation semantics — this is a
// repayment), NOT the balance direction ("debit"). This ensures that
// downstream rubric resolution (resolveAccountingRubric) picks
// Overdraft.Credit without special-casing in the resolver.
//
// Alias shape: same positional-prefix convention as the debit path (see
// buildCompanionDebitOp) — uses the destination op's index so that
// `BuildOperations` can match the companion balance against the companion
// FromTo entry produced alongside this op.
func buildCompanionCreditOp(organizationID, ledgerID uuid.UUID, r overdraftRefund, companion *mmodel.Balance) mmodel.BalanceOperation {
	companionAmount := r.destination.Amount
	companionAmount.Value = r.repayAmount
	companionAmount.Operation = libConstants.CREDIT
	companionAmount.Direction = constant.DirectionCredit

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID,
		companion.Alias+"#"+companion.Key)

	return mmodel.BalanceOperation{
		Balance:     companion,
		Alias:       indexPrefix(r.destination.Alias) + r.companionAliasKey,
		Amount:      companionAmount,
		InternalKey: internalKey,
	}
}

// buildCompanionFromTo constructs the synthetic FromTo entry that tells
// BuildOperations to emit an Operation record for the companion balance.
// Without this entry, the `balances × fromTo` loop in BuildOperations never
// matches the companion (its alias is not in the user-submitted transaction
// DSL) and the persisted audit trail is missing the overdraft leg — the
// failure mode observed in the feature before this fix: DB balances correct,
// response.operations incomplete, Postgres `operation` table missing rows.
//
// The returned entry:
//   - Carries `AccountAlias` in the same concat form as the companion
//     BalanceOperation so `blc.Alias == fromTo[i].AccountAlias` matches.
//   - Inherits `ChartOfAccounts`, `Route`, `RouteID`, `Metadata`, and
//     `Description` from the user-submitted primary op so the companion
//     record shows up in accounting reports alongside the primary leg.
//   - Sets `IsFrom` based on which side of the transaction triggered the
//     enrichment (debit split → source side; refund split → destination
//     side). The state machine downstream reads this flag when deciding
//     double-entry splitting and polarity.
//   - Carries an `Amount` pointer with the deficit/repayment value — this
//     is what the Operation record will display as the companion's amount.
//     The value is independent of the primary op's amount; they are two
//     distinct balance mutations with two distinct amounts.
//
// `primaryRouteID` carries the routeID from the primary op's FromTo entry.
// When non-empty it is propagated onto both FromTo.Route and FromTo.RouteID
// so downstream consumers — BuildOperations (which copies ft.RouteID to
// op.RouteID) and resolveRouteCodesFromCache (which looks the routeID up in
// the transaction route cache) — see the companion as a routed operation.
// Mirrors the hold/commit/cancel pattern where companion operations reuse
// the direct op's routeId and the action determines the rubric.
func buildCompanionFromTo(primary mmodel.BalanceOperation, companionOp mmodel.BalanceOperation, isFrom bool, primaryRouteID string) mtransaction.FromTo {
	amount := mtransaction.Amount{
		Asset:                  companionOp.Amount.Asset,
		Value:                  companionOp.Amount.Value,
		Operation:              companionOp.Amount.Operation,
		TransactionType:        companionOp.Amount.TransactionType,
		Direction:              companionOp.Amount.Direction,
		RouteValidationEnabled: companionOp.Amount.RouteValidationEnabled,
	}

	// `primary` is the user-submitted BalanceOperation; we reach through its
	// Balance (user-facing credit balance) to inherit metadata the companion
	// should carry. Defensive nil-check: the enrichment's precondition is
	// Balance != nil on the primary, but we fall back to empty metadata
	// rather than panicking if someone later changes that invariant.
	var (
		chartOfAccounts string
		metadata        map[string]any
	)

	if primary.Balance != nil {
		// There is no Route/ChartOfAccounts on the Balance type directly —
		// those live on the user's DSL entry. For companion entries we
		// currently leave them empty so they do not leak user-facing
		// accounting rubrics onto the system-managed companion op. Future
		// work (T-008 AccountingEntries Extension) may fill these in from
		// an `overdraft`/`refund` accounting-route rubric.
		_ = primary.Balance // intentionally unused — placeholder for T-008
	}

	ft := mtransaction.FromTo{
		AccountAlias:    companionOp.Alias,
		BalanceKey:      constant.OverdraftBalanceKey,
		Amount:          &amount,
		ChartOfAccounts: chartOfAccounts,
		Metadata:        metadata,
		IsFrom:          isFrom,
	}

	// Only propagate routeID when the primary carries one. A nil RouteID on
	// the companion preserves the legacy no-route behaviour (route validation
	// disabled or transaction without routeId) — setting a non-nil empty
	// pointer here would incorrectly signal "routed with empty id" and trip
	// the 0117 guard in validateAccountRules.
	if primaryRouteID != "" {
		routeID := primaryRouteID
		ft.Route = routeID
		ft.RouteID = &routeID
	}

	return ft
}

// registerCompanionInValidateTo mirrors the refund companion op into
// validate.To so ValidateBalancesRules sees matching counts. Follows the
// same key-shape convention as registerCompanionInValidate: concat-form for
// the `To` map key, bare alias-key for the `Destinations` / `Aliases` slices.
//
// `primaryRouteID` propagation: when non-empty we mirror it into
// `validate.OperationRoutesTo` under the companion's concat alias — this is
// what lets validateAccountRules look up a non-empty routeID for the
// companion and resolve it against the destination / bidirectional route
// caches. See registerCompanionInValidate for the same mechanism on the
// source side.
func registerCompanionInValidateTo(validate *mtransaction.Responses, _ mmodel.BalanceOperation, companionOp mmodel.BalanceOperation, primaryRouteID string) {
	if validate.To == nil {
		validate.To = make(map[string]mtransaction.Amount, 1)
	}

	if _, exists := validate.To[companionOp.Alias]; !exists {
		validate.To[companionOp.Alias] = companionOp.Amount
	}

	if primaryRouteID != "" {
		if validate.OperationRoutesTo == nil {
			validate.OperationRoutesTo = make(map[string]string, 1)
		}

		if _, exists := validate.OperationRoutesTo[companionOp.Alias]; !exists {
			validate.OperationRoutesTo[companionOp.Alias] = primaryRouteID
		}
	}

	bareAlias := stripIndexPrefix(companionOp.Alias)

	for _, existing := range validate.Destinations {
		if existing == bareAlias {
			return
		}
	}

	validate.Destinations = append(validate.Destinations, bareAlias)

	if !containsString(validate.Aliases, bareAlias) {
		validate.Aliases = append(validate.Aliases, bareAlias)
	}
}

// stripIndexPrefix is the inverse of indexPrefix: given an alias like
// "0#@alice#overdraft" it returns "@alice#overdraft". If no leading
// "<digits>#" prefix is present the alias is returned unchanged, so the
// helper is safe to call on values of either shape.
func stripIndexPrefix(alias string) string {
	prefix := indexPrefix(alias)
	if prefix == "" {
		return alias
	}

	return alias[len(prefix):]
}
