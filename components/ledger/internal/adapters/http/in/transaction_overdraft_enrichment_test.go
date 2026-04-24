// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// overdraftEnabledBalance builds a credit-direction balance with overdraft
// turned on and the provided limit. It mirrors the shape produced by
// getBalancesFromCache (Settings materialized from the Redis payload).
func overdraftEnabledBalance(t *testing.T, alias string, available decimal.Decimal, limit string) *mmodel.Balance {
	t.Helper()

	limitCopy := limit

	return &mmodel.Balance{
		ID:             uuid.New().String(),
		AccountID:      uuid.New().String(),
		Alias:          alias,
		Key:            constant.DefaultBalanceKey,
		AssetCode:      "BRL",
		Available:      available,
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.Zero,
		Settings: &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: limit != "" && limit != "0",
			OverdraftLimit:        ptrIfNotEmpty(&limitCopy),
		},
	}
}

func ptrIfNotEmpty(s *string) *string {
	if s == nil || *s == "" || *s == "0" {
		return nil
	}

	return s
}

// companionOverdraftBalance builds the direction=debit companion for an alias.
func companionOverdraftBalance(alias string) *mmodel.Balance {
	return &mmodel.Balance{
		ID:             uuid.New().String(),
		AccountID:      uuid.New().String(),
		Alias:          alias,
		Key:            constant.OverdraftBalanceKey,
		AssetCode:      "BRL",
		Available:      decimal.Zero,
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionDebit,
		OverdraftUsed:  decimal.Zero,
	}
}

// TestEnrichOverdraftOperations_SourceDebitSplit is the functional spec for
// the happy path: a source op that overflows a credit balance with overdraft
// enabled must yield one additional DEBIT op on the companion overdraft
// balance for the deficit amount.
func TestEnrichOverdraftOperations_SourceDebitSplit(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	source := overdraftEnabledBalance(t, "@alice", decimal.NewFromInt(50), "100")
	companion := companionOverdraftBalance("@alice")

	primary := mmodel.BalanceOperation{
		Balance: source,
		Alias:   "0#@alice#default",
		Amount: mtransaction.Amount{
			Asset:           "BRL",
			Value:           decimal.NewFromInt(100),
			Operation:       libConstants.DEBIT,
			TransactionType: libConstants.CREATED,
			Direction:       constant.DirectionCredit,
		},
		InternalKey: utils.BalanceInternalKey(orgID, ledgerID, "@alice#default"),
	}

	var loaderCalls [][]string

	loader := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
		loaderCalls = append(loaderCalls, append([]string(nil), aliases...))
		return []*mmodel.Balance{companion}, nil
	}

	validate := &mtransaction.Responses{
		From: map[string]mtransaction.Amount{
			"0#@alice#default": primary.Amount,
		},
		Sources: []string{"@alice#default"},
		Aliases: []string{"@alice#default"},
	}

	enriched, companionFromTos, err := enrichOverdraftOperations(context.Background(), orgID, ledgerID,
		[]mmodel.BalanceOperation{primary}, validate, loader)
	require.NoError(t, err)

	// Loader must have been asked for exactly the companion alias.
	require.Len(t, loaderCalls, 1)
	assert.Equal(t, []string{"@alice#overdraft"}, loaderCalls[0])

	// Enriched output: primary op first, companion op appended. Order matters
	// because the Lua script relies on a stable alphabetical internal-key sort
	// done later by the caller; see buildBalanceOperations.
	require.Len(t, enriched, 2)
	assert.Equal(t, primary, enriched[0], "primary op must pass through untouched")

	companionOp := enriched[1]
	assert.Same(t, companion, companionOp.Balance,
		"companion op must reference the loaded overdraft balance")
	// Companion Alias MUST be in concat form matching the source's positional
	// prefix — this is what lets BuildOperations later match the companion
	// against its synthetic FromTo entry via `blc.Alias == fromTo[i].AccountAlias`.
	assert.Equal(t, "0#@alice#overdraft", companionOp.Alias,
		"companion alias must carry the source's positional prefix so BuildOperations matches it against the companion FromTo entry")
	assert.Equal(t, libConstants.DEBIT, companionOp.Amount.Operation,
		"companion op must be a DEBIT so direction=debit semantics grow the liability")
	assert.Equal(t, constant.DirectionDebit, companionOp.Amount.Direction,
		"companion direction must be explicit to skip state-machine inference")
	assert.True(t, companionOp.Amount.Value.Equal(decimal.NewFromInt(50)),
		"companion amount must equal the deficit (100 - 50 = 50), got %s", companionOp.Amount.Value)
	assert.Equal(t, utils.BalanceInternalKey(orgID, ledgerID, "@alice#overdraft"),
		companionOp.InternalKey,
		"internal key must use the companion alias+key so Redis targets the right blob")

	// Transaction metadata must be inherited so downstream consumers treat
	// the companion as part of the same logical transaction.
	assert.Equal(t, libConstants.CREATED, companionOp.Amount.TransactionType)
	assert.Equal(t, "BRL", companionOp.Amount.Asset)

	// validate must be mirrored so ValidateBalancesRules sees matching counts.
	// The concat-form key matches the companion BalanceOperation.Alias so
	// there is exactly one canonical key per companion entry.
	companionKey := "0#@alice#overdraft"
	companionEntry, ok := validate.From[companionKey]
	require.True(t, ok, "companion alias must be present in validate.From under its positional key")
	assert.True(t, companionEntry.Value.Equal(decimal.NewFromInt(50)),
		"validate.From entry must carry the deficit amount")
	assert.Equal(t, libConstants.DEBIT, companionEntry.Operation)

	// Sources and Aliases carry the BARE form (no positional prefix) because
	// that is how CalculateTotal emits entries for user-submitted ops — keeping
	// the slice shape consistent lets getAliasWithoutKey strip `#key` in a
	// single pass without any special handling for enriched entries.
	assert.Contains(t, validate.Sources, "@alice#overdraft",
		"companion alias-key must join the sources list so downstream maps have a slot")
	assert.Contains(t, validate.Aliases, "@alice#overdraft",
		"companion alias-key must join the aliases list so secondary lookups find it")

	// Companion FromTo entries are the audit-trail half of the enrichment
	// contract: the caller appends these to its `fromTo` slice so
	// BuildOperations emits one Operation record per companion balance
	// mutation. The AccountAlias MUST match the BalanceOperation.Alias so
	// the `balances × fromTo` match loop produces a persisted Operation.
	require.Len(t, companionFromTos, 1, "one companion FromTo per debit split")

	ft := companionFromTos[0]
	assert.Equal(t, "0#@alice#overdraft", ft.AccountAlias,
		"FromTo.AccountAlias must match companion BalanceOperation.Alias exactly — otherwise BuildOperations cannot match them")
	assert.Equal(t, constant.OverdraftBalanceKey, ft.BalanceKey,
		"FromTo.BalanceKey must be 'overdraft' so operation record reflects the companion ledger")
	require.NotNil(t, ft.Amount, "FromTo.Amount pointer must be set for BuildOperations to dereference")
	assert.True(t, ft.Amount.Value.Equal(decimal.NewFromInt(50)),
		"companion FromTo amount must equal the deficit")
	assert.True(t, ft.IsFrom, "debit enrichment companion lives on the source side")
}

// TestEnrichOverdraftOperations_NoSplitForNonOverflow guards the common path:
// when the debit fits within the available balance, no enrichment happens and
// the loader is not called.
func TestEnrichOverdraftOperations_NoSplitForNonOverflow(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	source := overdraftEnabledBalance(t, "@alice", decimal.NewFromInt(500), "100")

	primary := mmodel.BalanceOperation{
		Balance: source,
		Alias:   "0#@alice#default",
		Amount: mtransaction.Amount{
			Value:     decimal.NewFromInt(100),
			Operation: libConstants.DEBIT,
			Direction: constant.DirectionCredit,
		},
		InternalKey: utils.BalanceInternalKey(orgID, ledgerID, "@alice#default"),
	}

	loader := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
		t.Fatalf("loader must not be invoked when the debit fits within available funds")
		return nil, nil
	}

	enriched, _, err := enrichOverdraftOperations(context.Background(), orgID, ledgerID,
		[]mmodel.BalanceOperation{primary}, nil, loader)
	require.NoError(t, err)
	assert.Equal(t, []mmodel.BalanceOperation{primary}, enriched,
		"non-overflowing ops must pass through unchanged")
}

// TestEnrichOverdraftOperations_NoSplitWhenOverdraftDisabled verifies that a
// credit balance without AllowOverdraft falls back to the legacy
// insufficient-funds path — the Lua layer rejects the transaction at 0018 —
// and never triggers the enrichment loader.
func TestEnrichOverdraftOperations_NoSplitWhenOverdraftDisabled(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	source := &mmodel.Balance{
		ID:             uuid.New().String(),
		AccountID:      uuid.New().String(),
		Alias:          "@alice",
		Key:            constant.DefaultBalanceKey,
		AssetCode:      "BRL",
		Available:      decimal.NewFromInt(50),
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionCredit,
		// No Settings → overdraft disabled.
	}

	primary := mmodel.BalanceOperation{
		Balance: source,
		Alias:   "0#@alice#default",
		Amount: mtransaction.Amount{
			Value:     decimal.NewFromInt(100),
			Operation: libConstants.DEBIT,
			Direction: constant.DirectionCredit,
		},
	}

	loader := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
		t.Fatalf("loader must not be invoked when AllowOverdraft is false")
		return nil, nil
	}

	enriched, _, err := enrichOverdraftOperations(context.Background(), orgID, ledgerID,
		[]mmodel.BalanceOperation{primary}, nil, loader)
	require.NoError(t, err)
	assert.Equal(t, []mmodel.BalanceOperation{primary}, enriched)
}

// TestEnrichOverdraftOperations_LoaderError propagates infra failures so the
// handler can roll back idempotency and the redis queue. Dropping the error
// silently would leave the transaction mid-enrichment without observable
// failure in the API response.
func TestEnrichOverdraftOperations_LoaderError(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	source := overdraftEnabledBalance(t, "@alice", decimal.NewFromInt(50), "100")

	primary := mmodel.BalanceOperation{
		Balance: source,
		Alias:   "0#@alice#default",
		Amount: mtransaction.Amount{
			Value:     decimal.NewFromInt(100),
			Operation: libConstants.DEBIT,
			Direction: constant.DirectionCredit,
		},
	}

	sentinel := errors.New("redis connection refused")
	loader := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
		return nil, sentinel
	}

	_, _, err := enrichOverdraftOperations(context.Background(), orgID, ledgerID,
		[]mmodel.BalanceOperation{primary}, nil, loader)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "loader errors must be wrapped transparently")
}

// TestEnrichOverdraftOperations_MissingCompanionIsNoisyButNonFatal documents
// the defensive fallback: if the companion balance was not auto-created (e.g.
// legacy data or a failed Create call during a settings PATCH), the primary
// op still flows through and the Lua script's internal split keeps the
// default balance correct. The absence of the companion op is logged but
// does NOT block the transaction.
func TestEnrichOverdraftOperations_MissingCompanionIsNoisyButNonFatal(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	source := overdraftEnabledBalance(t, "@alice", decimal.NewFromInt(50), "100")

	primary := mmodel.BalanceOperation{
		Balance: source,
		Alias:   "0#@alice#default",
		Amount: mtransaction.Amount{
			Value:     decimal.NewFromInt(100),
			Operation: libConstants.DEBIT,
			Direction: constant.DirectionCredit,
		},
	}

	loader := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
		// Return an empty list — no companion found.
		return []*mmodel.Balance{}, nil
	}

	enriched, _, err := enrichOverdraftOperations(context.Background(), orgID, ledgerID,
		[]mmodel.BalanceOperation{primary}, nil, loader)
	require.NoError(t, err, "missing companion must not abort the transaction")
	assert.Equal(t, []mmodel.BalanceOperation{primary}, enriched,
		"no companion op appended when the companion balance is missing")
}

// TestRejectInternalScopeBalances_BlocksDirectTargeting locks in the CREATE-
// path scope guard: when the user builds a transaction that references a
// balance with BalanceScope=internal (e.g. "@account#overdraft"), the handler
// must surface the canonical 0168 error instead of letting the transaction
// enter the Lua atomic path (where it would be indistinguishable from a
// plain insufficient-funds failure).
//
// This test mirrors the state-transition guard in
// services/command/create_balance_transaction_operations_async.go and
// ensures the two call sites stay in lock-step.
func TestRejectInternalScopeBalances_BlocksDirectTargeting(t *testing.T) {
	internal := &mmodel.Balance{
		Alias: "@alice",
		Key:   constant.OverdraftBalanceKey,
		Settings: &mmodel.BalanceSettings{
			BalanceScope: mmodel.BalanceScopeInternal,
		},
	}

	regular := &mmodel.Balance{
		Alias: "@bob",
		Key:   constant.DefaultBalanceKey,
		Settings: &mmodel.BalanceSettings{
			BalanceScope: mmodel.BalanceScopeTransactional,
		},
	}

	err := rejectInternalScopeBalances(context.Background(), []*mmodel.Balance{regular, internal})
	require.Error(t, err, "any internal-scope balance in the slice must abort the flow")

	// The rejection MUST carry the canonical 0168 code so the HTTP layer
	// returns 422 and downstream observability reports the right category.
	assert.Contains(t, err.Error(), constant.ErrDirectOperationOnInternalBalance.Error(),
		"error must surface as 0168; got %v", err)
}

// TestEnrichOverdraftOperations_DestinationRefundSplit verifies the credit-
// repayment path: when a credit destination has OverdraftUsed > 0, the
// enrichment appends a CREDIT op on the companion overdraft balance for
// min(amount, overdraftUsed). The primary op stays at the full credit
// amount — the Lua atomic script reconciles the split by decrementing
// OverdraftUsed and growing Available only by the remainder.
func TestEnrichOverdraftOperations_DestinationRefundSplit(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Destination balance carries outstanding overdraft. The refund detector
	// keys off OverdraftUsed > 0 and the operation being a plain CREDIT on
	// a direction=credit balance.
	destination := &mmodel.Balance{
		ID:             uuid.New().String(),
		AccountID:      uuid.New().String(),
		Alias:          "@alice",
		Key:            constant.DefaultBalanceKey,
		AssetCode:      "BRL",
		Available:      decimal.Zero,
		OnHold:         decimal.Zero,
		Version:        2,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionCredit,
		// 50 outstanding — partial repayment will cap at this value.
		OverdraftUsed: decimal.NewFromInt(50),
	}
	companion := companionOverdraftBalance("@alice")
	// Simulate the companion balance holding the matching liability.
	companion.Available = decimal.NewFromInt(50)

	primary := mmodel.BalanceOperation{
		Balance: destination,
		Alias:   "0#@alice#default",
		Amount: mtransaction.Amount{
			Asset:           "BRL",
			Value:           decimal.NewFromInt(80),
			Operation:       libConstants.CREDIT,
			TransactionType: libConstants.CREATED,
			Direction:       constant.DirectionCredit,
		},
		InternalKey: utils.BalanceInternalKey(orgID, ledgerID, "@alice#default"),
	}

	var loaderCalls [][]string

	loader := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
		loaderCalls = append(loaderCalls, append([]string(nil), aliases...))
		return []*mmodel.Balance{companion}, nil
	}

	validate := &mtransaction.Responses{
		To: map[string]mtransaction.Amount{
			"0#@alice#default": primary.Amount,
		},
		Destinations: []string{"@alice#default"},
		Aliases:      []string{"@alice#default"},
	}

	enriched, _, err := enrichOverdraftOperations(context.Background(), orgID, ledgerID,
		[]mmodel.BalanceOperation{primary}, validate, loader)
	require.NoError(t, err)

	require.Len(t, loaderCalls, 1, "loader must be called once for the refund's companion")
	assert.Equal(t, []string{"@alice#overdraft"}, loaderCalls[0])

	require.Len(t, enriched, 2)
	assert.Equal(t, primary, enriched[0], "primary credit op must flow through untouched")

	companionOp := enriched[1]
	assert.Equal(t, libConstants.CREDIT, companionOp.Amount.Operation,
		"companion op must be a CREDIT so direction=debit semantics shrink the liability")
	assert.Equal(t, constant.DirectionCredit, companionOp.Amount.Direction,
		"companion direction must be credit (repayment semantics) for rubric resolution")
	assert.True(t, companionOp.Amount.Value.Equal(decimal.NewFromInt(50)),
		"companion repay amount must equal min(80, 50) = 50, got %s", companionOp.Amount.Value)

	// Validate must carry the companion in validate.To so ValidateBalancesRules
	// keeps its len(balances) == len(From)+len(To) invariant.
	companionKey := "0#@alice#overdraft"
	entry, ok := validate.To[companionKey]
	require.True(t, ok, "companion alias must be registered in validate.To")
	assert.True(t, entry.Value.Equal(decimal.NewFromInt(50)))
	assert.Equal(t, libConstants.CREDIT, entry.Operation)
	assert.Contains(t, validate.Destinations, "@alice#overdraft")
}

// TestEnrichOverdraftOperations_RefundCappedAtOverdraftUsed pins the upper
// bound: when the incoming credit is smaller than OverdraftUsed, the
// companion op amount equals the full credit (the whole credit repays
// overdraft; Available on default stays at zero). When it is larger, the
// companion op is capped at OverdraftUsed and the primary op handles the
// remainder via the Lua split.
func TestEnrichOverdraftOperations_RefundCappedAtOverdraftUsed(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	destination := &mmodel.Balance{
		ID:             uuid.New().String(),
		AccountID:      uuid.New().String(),
		Alias:          "@alice",
		Key:            constant.DefaultBalanceKey,
		AssetCode:      "BRL",
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.NewFromInt(500),
		AllowSending:   true,
		AllowReceiving: true,
	}
	companion := companionOverdraftBalance("@alice")

	primary := mmodel.BalanceOperation{
		Balance: destination,
		Alias:   "0#@alice#default",
		Amount: mtransaction.Amount{
			Value:     decimal.NewFromInt(1), // smaller than 500 overdraft
			Operation: libConstants.CREDIT,
			Direction: constant.DirectionCredit,
		},
	}

	loader := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
		return []*mmodel.Balance{companion}, nil
	}

	enriched, _, err := enrichOverdraftOperations(context.Background(), orgID, ledgerID,
		[]mmodel.BalanceOperation{primary}, nil, loader)
	require.NoError(t, err)
	require.Len(t, enriched, 2)

	assert.True(t, enriched[1].Amount.Value.Equal(decimal.NewFromInt(1)),
		"refund cap must be min(credit, overdraft) — credit 1 against overdraft 500 yields 1, got %s",
		enriched[1].Amount.Value)
}

// TestRejectInternalScopeBalances_AllowsTransactionalBalances is the happy
// path: every balance is transactional, so the guard must be a no-op and let
// the handler continue.
func TestRejectInternalScopeBalances_AllowsTransactionalBalances(t *testing.T) {
	transactional := []*mmodel.Balance{
		{Alias: "@alice", Key: constant.DefaultBalanceKey,
			Settings: &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeTransactional}},
		{Alias: "@bob", Key: constant.DefaultBalanceKey},
		// nil balance entries can appear after failed lookups — the guard
		// must tolerate them without panicking or false-positiving.
		nil,
	}

	err := rejectInternalScopeBalances(context.Background(), transactional)
	require.NoError(t, err, "transactional balances and nil entries must pass through")
}

// TestEnrichOverdraftOperations_DebitCompanionInheritsRouteID locks in the
// route-propagation contract for the debit-split (overdraft) enrichment path:
// when the primary source op carries a RouteID (either directly on its
// FromTo entry or via validate.OperationRoutesFrom), the companion FromTo
// entry MUST inherit the same RouteID. This mirrors the hold/commit/cancel
// pattern where companion operations reuse the direct op's routeId — the
// action determines which AccountingEntry rubric is resolved, not a
// different RouteID.
//
// Failure mode this test prevents: with route validation enabled,
// ValidateAccountingRules iterates companion ops in validate.From and looks
// up validate.OperationRoutesFrom[companionAlias]. Without propagation this
// returns "" and the validator rejects the transaction with 0117
// (Accounting Route Not Found).
func TestEnrichOverdraftOperations_DebitCompanionInheritsRouteID(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	source := overdraftEnabledBalance(t, "@alice", decimal.NewFromInt(50), "100")
	companion := companionOverdraftBalance("@alice")

	primary := mmodel.BalanceOperation{
		Balance: source,
		Alias:   "0#@alice#default",
		Amount: mtransaction.Amount{
			Asset:     "BRL",
			Value:     decimal.NewFromInt(100),
			Operation: libConstants.DEBIT,
			Direction: constant.DirectionCredit,
		},
		InternalKey: utils.BalanceInternalKey(orgID, ledgerID, "@alice#default"),
	}

	routeID := uuid.NewString()

	loader := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
		return []*mmodel.Balance{companion}, nil
	}

	// validate mirrors what ValidateSendSourceAndDistribute would produce
	// when the user supplies routeId on from[0].routeId — the concat key
	// "0#@alice#default" maps to the routeID string.
	validate := &mtransaction.Responses{
		From: map[string]mtransaction.Amount{
			"0#@alice#default": primary.Amount,
		},
		Sources: []string{"@alice#default"},
		Aliases: []string{"@alice#default"},
		OperationRoutesFrom: map[string]string{
			"0#@alice#default": routeID,
		},
	}

	_, companionFromTos, err := enrichOverdraftOperations(context.Background(), orgID, ledgerID,
		[]mmodel.BalanceOperation{primary}, validate, loader)
	require.NoError(t, err)
	require.Len(t, companionFromTos, 1, "one companion FromTo per debit split")

	ft := companionFromTos[0]
	require.NotNil(t, ft.RouteID, "companion FromTo must inherit the primary's RouteID so ValidateAccountingRules can resolve it")
	assert.Equal(t, routeID, *ft.RouteID,
		"companion routeID must EQUAL the primary's routeID — same route, action determines rubric (hold/commit/cancel pattern)")

	// Mirror check: validate.OperationRoutesFrom must now carry an entry for
	// the companion alias so validateAccountRules sees a non-empty routeID
	// when it iterates companion operations.
	gotFrom, ok := validate.OperationRoutesFrom["0#@alice#overdraft"]
	require.True(t, ok, "companion alias must be registered in validate.OperationRoutesFrom so route validation finds it")
	assert.Equal(t, routeID, gotFrom,
		"OperationRoutesFrom entry for the companion must point at the primary's routeID")
}

// TestEnrichOverdraftOperations_RefundCompanionInheritsRouteID mirrors the
// debit test for the credit-repayment (refund) path: when the primary
// destination op carries a RouteID, the refund companion FromTo entry MUST
// inherit the same RouteID and it MUST appear in validate.OperationRoutesTo.
func TestEnrichOverdraftOperations_RefundCompanionInheritsRouteID(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	destination := &mmodel.Balance{
		ID:             uuid.New().String(),
		AccountID:      uuid.New().String(),
		Alias:          "@alice",
		Key:            constant.DefaultBalanceKey,
		AssetCode:      "BRL",
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.NewFromInt(50),
		AllowSending:   true,
		AllowReceiving: true,
	}
	companion := companionOverdraftBalance("@alice")

	primary := mmodel.BalanceOperation{
		Balance: destination,
		Alias:   "0#@alice#default",
		Amount: mtransaction.Amount{
			Asset:     "BRL",
			Value:     decimal.NewFromInt(80),
			Operation: libConstants.CREDIT,
			Direction: constant.DirectionCredit,
		},
		InternalKey: utils.BalanceInternalKey(orgID, ledgerID, "@alice#default"),
	}

	routeID := uuid.NewString()

	loader := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
		return []*mmodel.Balance{companion}, nil
	}

	validate := &mtransaction.Responses{
		To: map[string]mtransaction.Amount{
			"0#@alice#default": primary.Amount,
		},
		Destinations: []string{"@alice#default"},
		Aliases:      []string{"@alice#default"},
		OperationRoutesTo: map[string]string{
			"0#@alice#default": routeID,
		},
	}

	_, companionFromTos, err := enrichOverdraftOperations(context.Background(), orgID, ledgerID,
		[]mmodel.BalanceOperation{primary}, validate, loader)
	require.NoError(t, err)
	require.Len(t, companionFromTos, 1, "one companion FromTo per refund split")

	ft := companionFromTos[0]
	require.NotNil(t, ft.RouteID, "refund companion FromTo must inherit the primary's RouteID")
	assert.Equal(t, routeID, *ft.RouteID)

	gotTo, ok := validate.OperationRoutesTo["0#@alice#overdraft"]
	require.True(t, ok, "refund companion alias must be registered in validate.OperationRoutesTo")
	assert.Equal(t, routeID, gotTo,
		"OperationRoutesTo entry for the refund companion must point at the primary's routeID")
}

// TestEnrichOverdraftOperations_CompanionRouteIDNilWhenPrimaryHasNone is the
// backward-compat guard: when route validation is disabled or the primary
// op has no routeId (legacy transaction shape), the companion FromTo must
// also carry a nil RouteID so downstream flows that do not use route
// validation keep working unchanged.
func TestEnrichOverdraftOperations_CompanionRouteIDNilWhenPrimaryHasNone(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	source := overdraftEnabledBalance(t, "@alice", decimal.NewFromInt(50), "100")
	companion := companionOverdraftBalance("@alice")

	primary := mmodel.BalanceOperation{
		Balance: source,
		Alias:   "0#@alice#default",
		Amount: mtransaction.Amount{
			Value:     decimal.NewFromInt(100),
			Operation: libConstants.DEBIT,
			Direction: constant.DirectionCredit,
		},
	}

	loader := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
		return []*mmodel.Balance{companion}, nil
	}

	// No OperationRoutesFrom entry — simulates route validation disabled or
	// legacy transaction shape without routeId.
	validate := &mtransaction.Responses{
		From: map[string]mtransaction.Amount{
			"0#@alice#default": primary.Amount,
		},
		Sources: []string{"@alice#default"},
		Aliases: []string{"@alice#default"},
	}

	_, companionFromTos, err := enrichOverdraftOperations(context.Background(), orgID, ledgerID,
		[]mmodel.BalanceOperation{primary}, validate, loader)
	require.NoError(t, err)
	require.Len(t, companionFromTos, 1)

	assert.Nil(t, companionFromTos[0].RouteID,
		"companion RouteID must be nil when the primary has no routeId so legacy flows stay unchanged")
}
