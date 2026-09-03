// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// End-to-end coverage of the overdraft enrichment seam as the handlers reach it,
// replaying the real create/commit step order:
// ApplyDefaultBalanceKeys -> MutateConcatAliases -> ValidateSendSourceAndDistribute
// -> PropagateRouteValidation -> BuildBalanceOperations -> EnrichOverdraftOperations
// -> ValidateAccountingRules over a ToCache()-built route cache. Exercising the
// whole chain is what catches a companion that enriches but fails route
// validation (or the reverse) — the unit tests around EnrichOverdraftOperations
// alone cannot see the interaction.
package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	redisadapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// overdraftRouteFlowOptions describes one replay of the create/commit chain.
type overdraftRouteFlowOptions struct {
	balances  []*mmodel.Balance
	companion *mmodel.Balance

	transactionStatus string
	// pending mirrors the request body's `pending` flag, which is what makes
	// DetermineOperation emit the two-phase operations (hold / commit / cancel).
	pending bool
	// actionOverride replaces the status-derived accounting action, as the revert
	// path does at the uc.
	actionOverride string
	// bidirectional builds one route pair carrying every action rubric, which is
	// what the two-phase actions (hold/commit/cancel) need.
	bidirectional bool
	// omitOverdraftCredit drops Overdraft.Credit from the route pair so the
	// repayment companion has no rubric to resolve.
	omitOverdraftCredit bool
}

// overdraftRouteFlowResult carries what each assertion needs: the enriched
// operations, the companion FromTo entries destined for BuildOperations, the
// mutated validate response, and the route-validation verdict.
type overdraftRouteFlowResult struct {
	balanceOps       []mmodel.BalanceOperation
	companionFromTos []mtransaction.FromTo
	validate         *mtransaction.Responses
	err              error
}

// companionOps returns the enriched operations that landed on the overdraft
// companion balance.
func (r overdraftRouteFlowResult) companionOps() []mmodel.BalanceOperation {
	out := make([]mmodel.BalanceOperation, 0, 1)

	for _, op := range r.balanceOps {
		if op.Balance != nil && op.Balance.Key == constant.OverdraftBalanceKey {
			out = append(out, op)
		}
	}

	return out
}

func overdraftFlowRoutePair(t *testing.T, srcRouteID, dstRouteID uuid.UUID, opts overdraftRouteFlowOptions) []byte {
	t.Helper()

	overdraft := func() *mmodel.AccountingEntry {
		entry := &mmodel.AccountingEntry{
			Debit: &mmodel.AccountingRubric{Code: "9001", Description: "Overdraft usage"},
		}

		if !opts.omitOverdraftCredit {
			entry.Credit = &mmodel.AccountingRubric{Code: "9002", Description: "Overdraft repayment"}
		}

		return entry
	}

	var tr *mmodel.TransactionRoute

	if opts.bidirectional {
		full := func() *mmodel.AccountingEntries {
			return &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash out"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Cash in"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "3001", Description: "Hold debit"},
					Credit: &mmodel.AccountingRubric{Code: "3002", Description: "Hold credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "4001", Description: "Commit debit"},
					Credit: &mmodel.AccountingRubric{Code: "4002", Description: "Commit credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "5001", Description: "Cancel debit"},
					Credit: &mmodel.AccountingRubric{Code: "5002", Description: "Cancel credit"},
				},
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "6001", Description: "Revert debit"},
					Credit: &mmodel.AccountingRubric{Code: "6002", Description: "Revert credit"},
				},
				Overdraft: overdraft(),
			}
		}

		tr = &mmodel.TransactionRoute{
			OperationRoutes: []mmodel.OperationRoute{
				{ID: srcRouteID, OperationType: "bidirectional", AccountingEntries: full()},
				{ID: dstRouteID, OperationType: "bidirectional", AccountingEntries: full()},
			},
		}
	} else {
		tr = &mmodel.TransactionRoute{
			OperationRoutes: []mmodel.OperationRoute{
				{
					ID:            srcRouteID,
					OperationType: "source",
					AccountingEntries: &mmodel.AccountingEntries{
						Direct: &mmodel.AccountingEntry{
							Debit: &mmodel.AccountingRubric{Code: "1001", Description: "Cash out"},
						},
						Overdraft: overdraft(),
					},
				},
				{
					ID:            dstRouteID,
					OperationType: "destination",
					AccountingEntries: &mmodel.AccountingEntries{
						Direct: &mmodel.AccountingEntry{
							Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Cash in"},
						},
						Overdraft: overdraft(),
					},
				},
			},
		}
	}

	raw, err := tr.ToCache().ToMsgpack()
	require.NoError(t, err)

	return raw
}

// runOverdraftRouteFlow replays the uc chain for one transaction shape and
// returns everything the assertions need.
func runOverdraftRouteFlow(t *testing.T, opts overdraftRouteFlowOptions) overdraftRouteFlowResult {
	t.Helper()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	txRouteID, srcRouteID, dstRouteID := uuid.New(), uuid.New(), uuid.New()

	txRoute := txRouteID.String()
	srcRoute := srcRouteID.String()
	dstRoute := dstRouteID.String()

	body := mtransaction.Transaction{
		RouteID: &txRoute,
		Pending: opts.pending,
		Send: mtransaction.Send{
			Asset: "BRL",
			Value: decimal.NewFromInt(100),
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{{
					AccountAlias: "@alice",
					Amount:       &mtransaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
					IsFrom:       true,
					RouteID:      &srcRoute,
				}},
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{{
					AccountAlias: "@bob",
					Amount:       &mtransaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
					RouteID:      &dstRoute,
				}},
			},
		},
	}

	mtransaction.ApplyDefaultBalanceKeys(body.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(body.Send.Distribute.To)
	mtransaction.MutateConcatAliases(body.Send.Source.From)
	mtransaction.MutateConcatAliases(body.Send.Distribute.To)

	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, body, opts.transactionStatus)
	require.NoError(t, err)

	mtransaction.PropagateRouteValidation(ctx, validate, opts.transactionStatus)

	balanceOps := BuildBalanceOperations(ctx, organizationID, ledgerID, validate, opts.balances)
	require.NotEmpty(t, balanceOps)

	loader := func(_ context.Context, _, _ uuid.UUID, _ []string) ([]*mmodel.Balance, error) {
		if opts.companion == nil {
			return nil, nil
		}

		return []*mmodel.Balance{opts.companion}, nil
	}

	balanceOps, companionFromTos, err := EnrichOverdraftOperations(ctx, organizationID, ledgerID, balanceOps, validate, loader)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := redisadapter.NewMockRedisRepository(ctrl)
	mockLedger := ledger.NewMockRepository(ctrl)

	mockLedger.EXPECT().
		GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]any{"accounting": map[string]any{"validateRoutes": true}}, nil).
		AnyTimes()

	cacheKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, txRouteID)
	mockRedis.EXPECT().GetBytes(gomock.Any(), cacheKey).
		Return(overdraftFlowRoutePair(t, srcRouteID, dstRouteID, opts), nil).AnyTimes()

	uc := &query.UseCase{
		TransactionRedisRepo: mockRedis,
		LedgerRepo:           mockLedger,
	}

	action := mtransaction.StatusToAction(opts.transactionStatus)
	if opts.actionOverride != "" {
		action = opts.actionOverride
	}

	_, validationErr := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, balanceOps, validate, action)

	return overdraftRouteFlowResult{
		balanceOps:       balanceOps,
		companionFromTos: companionFromTos,
		validate:         validate,
		err:              validationErr,
	}
}

func overdraftFlowSourceBalances(t *testing.T) []*mmodel.Balance {
	t.Helper()

	return []*mmodel.Balance{
		{
			ID: uuid.New().String(), Alias: "@alice", Key: constant.DefaultBalanceKey, AssetCode: "BRL",
			Available: decimal.NewFromInt(40), Direction: constant.DirectionCredit,
			AccountType: "deposit",
			Settings:    &mmodel.BalanceSettings{AllowOverdraft: true},
		},
		{
			ID: uuid.New().String(), Alias: "@bob", Key: constant.DefaultBalanceKey, AssetCode: "BRL",
			Available: decimal.Zero, Direction: constant.DirectionCredit,
			AccountType: "deposit",
		},
	}
}

// overdraftFlowDestinationBalances puts the outstanding overdraft on the
// destination (@bob), so an incoming credit repays it.
func overdraftFlowDestinationBalances(t *testing.T, sourceOnHold decimal.Decimal) []*mmodel.Balance {
	t.Helper()

	return []*mmodel.Balance{
		{
			ID: uuid.New().String(), Alias: "@alice", Key: constant.DefaultBalanceKey, AssetCode: "BRL",
			Available: decimal.NewFromInt(1000), OnHold: sourceOnHold,
			Direction: constant.DirectionCredit, AccountType: "deposit",
		},
		{
			ID: uuid.New().String(), Alias: "@bob", Key: constant.DefaultBalanceKey, AssetCode: "BRL",
			Available: decimal.Zero, OverdraftUsed: decimal.NewFromInt(50),
			Direction: constant.DirectionCredit, AccountType: "deposit",
			Settings: &mmodel.BalanceSettings{AllowOverdraft: true},
		},
	}
}

func overdraftFlowCompanion(alias string, available decimal.Decimal) *mmodel.Balance {
	return &mmodel.Balance{
		ID: uuid.New().String(), Alias: alias, Key: constant.OverdraftBalanceKey, AssetCode: "BRL",
		Available: available, Direction: constant.DirectionDebit,
		AccountType: "deposit",
		Settings:    &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal},
	}
}

// TestOverdraftRouteFlow_SourceDrawOnDirectCreate is the control: a direct
// create drawing overdraft at the source enriches a companion debit that the
// source route's overdraft.debit rubric covers.
func TestOverdraftRouteFlow_SourceDrawOnDirectCreate(t *testing.T) {
	result := runOverdraftRouteFlow(t, overdraftRouteFlowOptions{
		balances:          overdraftFlowSourceBalances(t),
		companion:         overdraftFlowCompanion("@alice", decimal.Zero),
		transactionStatus: constant.CREATED,
	})

	require.NoError(t, result.err, "source overdraft draw with the overdraft rubric configured must pass")
	require.Len(t, result.companionOps(), 1, "the source draw must enrich exactly one companion debit")
	assert.Equal(t, libConstants.DEBIT, result.companionOps()[0].Amount.Operation)
}

// TestOverdraftRouteFlow_DestinationRepaymentOnDirectCreate covers the mirror
// case: a direct create crediting an overdrafted destination enriches the
// repayment companion, validated against the destination route's
// overdraft.credit rubric.
func TestOverdraftRouteFlow_DestinationRepaymentOnDirectCreate(t *testing.T) {
	result := runOverdraftRouteFlow(t, overdraftRouteFlowOptions{
		balances:          overdraftFlowDestinationBalances(t, decimal.Zero),
		companion:         overdraftFlowCompanion("@bob", decimal.NewFromInt(50)),
		transactionStatus: constant.CREATED,
	})

	require.NoError(t, result.err, "destination overdraft repayment with the overdraft rubric configured must pass")

	companions := result.companionOps()
	require.Len(t, companions, 1)
	assert.Equal(t, libConstants.CREDIT, companions[0].Amount.Operation)
	assert.True(t, companions[0].Amount.Value.Equal(decimal.NewFromInt(50)),
		"repayment is capped at the outstanding overdraft; got %s", companions[0].Amount.Value)
}

// TestOverdraftRouteFlow_PendingCreateDrawsNoOverdraftOnSource locks the product
// rule through the uc chain: a HOLD never draws overdraft, so a pending
// create whose source hold exceeds Available enriches no draw companion — on
// either route version, since this is a product rule and not a version rule.
//
// The chain stops at ValidateAccountingRules, so the rejection itself is not
// observable here; that a hold exceeding Available is refused with 0018 and moves
// nothing is locked at the atomic script in
// TestIntegration_Overdraft_PendingHoldRejectsOverdraftDraw.
func TestOverdraftRouteFlow_PendingCreateDrawsNoOverdraftOnSource(t *testing.T) {
	result := runOverdraftRouteFlow(t, overdraftRouteFlowOptions{
		balances:          overdraftFlowSourceBalances(t),
		companion:         overdraftFlowCompanion("@alice", decimal.Zero),
		transactionStatus: constant.PENDING,
		pending:           true,
		bidirectional:     true,
	})

	require.NoError(t, result.err)
	assert.Empty(t, result.companionOps(),
		"a hold must not draw overdraft, so no companion debit may be enriched on a pending create")
	assert.Empty(t, result.companionFromTos)
	assert.NotContains(t, result.validate.Sources, "@alice#"+constant.OverdraftBalanceKey)
}

// TestOverdraftRouteFlow_PendingCreateDefersDestinationRepayment locks the
// deferred-leg rule on the enrichment side: the destination credit of a pending
// create posts nothing until the commit, so no repayment companion may be
// enriched. Mirroring one here would settle a liability the atomic script never
// touches, and the amount would be double-counted when the commit repays for
// real.
func TestOverdraftRouteFlow_PendingCreateDefersDestinationRepayment(t *testing.T) {
	result := runOverdraftRouteFlow(t, overdraftRouteFlowOptions{
		balances:          overdraftFlowDestinationBalances(t, decimal.Zero),
		companion:         overdraftFlowCompanion("@bob", decimal.NewFromInt(50)),
		transactionStatus: constant.PENDING,
		pending:           true,
		bidirectional:     true,
	})

	require.NoError(t, result.err)
	assert.Empty(t, result.companionOps(),
		"a pending create must not enrich a repayment companion for its deferred destination credit")
	assert.Empty(t, result.companionFromTos,
		"no companion operation record may be built for a deferred credit")
	assert.NotContains(t, result.validate.Destinations, "@bob#"+constant.OverdraftBalanceKey)
}

// TestOverdraftRouteFlow_CancelDefersDestinationRepayment is the cancel sibling
// of the pending case: a cancel batch still carries the destination CREDIT, but
// the destination never received the pending credit, so the cancel must enrich no
// repayment companion for it. Only the source restore may repay on a cancel.
func TestOverdraftRouteFlow_CancelDefersDestinationRepayment(t *testing.T) {
	result := runOverdraftRouteFlow(t, overdraftRouteFlowOptions{
		balances:          overdraftFlowDestinationBalances(t, decimal.NewFromInt(100)),
		companion:         overdraftFlowCompanion("@bob", decimal.NewFromInt(50)),
		transactionStatus: constant.CANCELED,
		pending:           true,
		bidirectional:     true,
	})

	require.NoError(t, result.err)
	assert.Empty(t, result.companionOps(),
		"a cancel must not enrich a repayment companion for the destination it never credited")
	assert.Empty(t, result.companionFromTos,
		"no overdraft operation record may be built for a deferred cancel credit")
	assert.NotContains(t, result.validate.Destinations, "@bob#"+constant.OverdraftBalanceKey)
}

// TestOverdraftRouteFlow_CommitRepaysDestinationOverdraft is the commit half of
// the two-phase lifecycle: the destination credit posts on APPROVED, so that is
// where the repayment companion is enriched, registered on the To side, and
// validated against the destination route's overdraft.credit rubric.
func TestOverdraftRouteFlow_CommitRepaysDestinationOverdraft(t *testing.T) {
	result := runOverdraftRouteFlow(t, overdraftRouteFlowOptions{
		balances:          overdraftFlowDestinationBalances(t, decimal.NewFromInt(100)),
		companion:         overdraftFlowCompanion("@bob", decimal.NewFromInt(50)),
		transactionStatus: constant.APPROVED,
		pending:           true,
		bidirectional:     true,
	})

	require.NoError(t, result.err, "commit repayment with the overdraft rubric configured must pass")

	companions := result.companionOps()
	require.Len(t, companions, 1, "the commit must enrich exactly one repayment companion")
	assert.Equal(t, libConstants.CREDIT, companions[0].Amount.Operation)
	assert.Equal(t, constant.DirectionCredit, companions[0].Amount.Direction)
	assert.True(t, companions[0].Amount.Value.Equal(decimal.NewFromInt(50)),
		"repayment is capped at the outstanding overdraft; got %s", companions[0].Amount.Value)

	// The companion must reach validate.To (not From): the repayment rides the
	// destination leg, and validateOverdraftRoutes resolves its route from there.
	entry, ok := result.validate.To["0#@bob#"+constant.OverdraftBalanceKey]
	require.True(t, ok, "the repayment companion must be registered in validate.To; got %v", result.validate.To)
	assert.True(t, entry.Value.Equal(decimal.NewFromInt(50)))

	require.Len(t, result.companionFromTos, 1,
		"the companion must reach BuildOperations so the overdraft leg is persisted")
	assert.False(t, result.companionFromTos[0].IsFrom)
	assert.Equal(t, constant.OverdraftBalanceKey, result.companionFromTos[0].BalanceKey)
}

// TestOverdraftRouteFlow_CommitRejectsMissingOverdraftCreditRubric pins the
// validation half of the commit fix: enriching the repayment companion is what
// puts it in front of validateOverdraftRoutes, so a destination route without an
// overdraft.credit rubric now fails the commit with 0492 instead of silently
// repaying without a general-ledger classification.
func TestOverdraftRouteFlow_CommitRejectsMissingOverdraftCreditRubric(t *testing.T) {
	result := runOverdraftRouteFlow(t, overdraftRouteFlowOptions{
		balances:            overdraftFlowDestinationBalances(t, decimal.NewFromInt(100)),
		companion:           overdraftFlowCompanion("@bob", decimal.NewFromInt(50)),
		transactionStatus:   constant.APPROVED,
		pending:             true,
		bidirectional:       true,
		omitOverdraftCredit: true,
	})

	require.Len(t, result.companionOps(), 1, "the companion must be enriched for the rubric check to reach it")
	require.Error(t, result.err)
	assert.ErrorContains(t, result.err, "0492")
}

// TestOverdraftRouteFlow_CommitDrawsNoOverdraftOnSource locks the commit-side
// guard: at commit the source leg consumes the OnHold reserved at create, so
// even with Available at zero it opens no overdraft and must enrich no companion
// debit.
func TestOverdraftRouteFlow_CommitDrawsNoOverdraftOnSource(t *testing.T) {
	balances := []*mmodel.Balance{
		{
			ID: uuid.New().String(), Alias: "@alice", Key: constant.DefaultBalanceKey, AssetCode: "BRL",
			// Available drained into OnHold by the pending create: the funds are
			// reserved, not overdrawn.
			Available: decimal.Zero, OnHold: decimal.NewFromInt(100),
			Direction: constant.DirectionCredit, AccountType: "deposit",
			Settings: &mmodel.BalanceSettings{AllowOverdraft: true},
		},
		{
			ID: uuid.New().String(), Alias: "@bob", Key: constant.DefaultBalanceKey, AssetCode: "BRL",
			Available: decimal.Zero, Direction: constant.DirectionCredit, AccountType: "deposit",
		},
	}

	result := runOverdraftRouteFlow(t, overdraftRouteFlowOptions{
		balances:          balances,
		companion:         overdraftFlowCompanion("@alice", decimal.Zero),
		transactionStatus: constant.APPROVED,
		pending:           true,
		bidirectional:     true,
	})

	require.NoError(t, result.err)
	assert.Empty(t, result.companionOps(),
		"a commit consuming its own hold must not be read as an overdraft draw")
	assert.Empty(t, result.companionFromTos)
}

// TestOverdraftRouteFlow_RevertRepaysDestinationOverdraft covers the revert
// shape: the account that drew the overdraft becomes the revert's destination
// and repays through the companion, resolved against the revert action.
func TestOverdraftRouteFlow_RevertRepaysDestinationOverdraft(t *testing.T) {
	result := runOverdraftRouteFlow(t, overdraftRouteFlowOptions{
		balances:          overdraftFlowDestinationBalances(t, decimal.Zero),
		companion:         overdraftFlowCompanion("@bob", decimal.NewFromInt(50)),
		transactionStatus: constant.CREATED,
		bidirectional:     true,
		actionOverride:    constant.ActionRevert,
	})

	require.NoError(t, result.err, "revert repaying overdraft at the destination must pass")
	assert.Len(t, result.companionOps(), 1)
}
