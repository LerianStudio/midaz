// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const crossLedgerFeeBridgeAlias = "@external/BRL"

var crossLedgerFeeOrganizationID = uuid.MustParse("0199c100-0000-7000-8000-000000000001")

var crossLedgerFeeLedgers = map[string]uuid.UUID{
	"A": uuid.MustParse("0199c100-0000-7000-8000-00000000000a"),
	"B": uuid.MustParse("0199c100-0000-7000-8000-00000000000b"),
	"N": uuid.MustParse("0199c100-0000-7000-8000-00000000000c"),
}

// crossLedgerFeeLeg is one client leg of a cross-ledger request and the ledger
// that owns its account.
type crossLedgerFeeLeg struct {
	alias  string
	amount string
	ledger string
}

// crossLedgerFeeShape is a cross-ledger request and the bridge each decomposed
// part must mark as non-payer, in part order (first-seen debit ledgers, then
// destination-only ledgers). A net-zero part has no bridge and marks nothing.
type crossLedgerFeeShape struct {
	name    string
	total   string
	from    []crossLedgerFeeLeg
	to      []crossLedgerFeeLeg
	ledgers []string
	bridges [][]model.NonPayerLeg
}

func crossLedgerFeeShapes() []crossLedgerFeeShape {
	return []crossLedgerFeeShape{
		{
			name:    "the receiving ledger has only the bridge as source",
			total:   "100",
			from:    []crossLedgerFeeLeg{{"@alice", "100", "A"}},
			to:      []crossLedgerFeeLeg{{"@bob", "100", "B"}},
			ledgers: []string{"A", "B"},
			bridges: [][]model.NonPayerLeg{{{IsFrom: false, Index: 0}}, {{IsFrom: true, Index: 0}}},
		},
		{
			name:    "the receiving ledger also sends",
			total:   "100",
			from:    []crossLedgerFeeLeg{{"@alice", "70", "A"}, {"@bob-out", "30", "B"}},
			to:      []crossLedgerFeeLeg{{"@bob", "100", "B"}},
			ledgers: []string{"A", "B"},
			bridges: [][]model.NonPayerLeg{{{IsFrom: false, Index: 0}}, {{IsFrom: true, Index: 1}}},
		},
		{
			name:    "the sending ledger also receives",
			total:   "100",
			from:    []crossLedgerFeeLeg{{"@alice", "100", "A"}},
			to:      []crossLedgerFeeLeg{{"@alice-back", "30", "A"}, {"@bob", "70", "B"}},
			ledgers: []string{"A", "B"},
			bridges: [][]model.NonPayerLeg{{{IsFrom: false, Index: 1}}, {{IsFrom: true, Index: 0}}},
		},
		{
			name:    "a client external source sits beside the bridge",
			total:   "110",
			from:    []crossLedgerFeeLeg{{"@alice", "100", "A"}, {crossLedgerFeeBridgeAlias, "10", "B"}},
			to:      []crossLedgerFeeLeg{{"@bob", "110", "B"}},
			ledgers: []string{"A", "B"},
			bridges: [][]model.NonPayerLeg{{{IsFrom: false, Index: 0}}, {{IsFrom: true, Index: 1}}},
		},
		{
			name:    "a net-zero participant crosses nothing",
			total:   "120",
			from:    []crossLedgerFeeLeg{{"@alice", "100", "A"}, {"@nina", "20", "N"}},
			to:      []crossLedgerFeeLeg{{"@nina-back", "20", "N"}, {"@bob", "100", "B"}},
			ledgers: []string{"A", "N", "B"},
			bridges: [][]model.NonPayerLeg{{{IsFrom: false, Index: 0}}, nil, {{IsFrom: true, Index: 0}}},
		},
	}
}

func (shape crossLedgerFeeShape) request() (mtransaction.Transaction, CrossLedgerTransactionScopes) {
	scopes := CrossLedgerTransactionScopes{}
	legs := func(in []crossLedgerFeeLeg, isFrom bool, scopeOf *[]CrossLedgerLegScope) []mtransaction.FromTo {
		out := make([]mtransaction.FromTo, len(in))
		for index, leg := range in {
			out[index] = crossLedgerAmountLeg(leg.alias, leg.amount, isFrom)
			*scopeOf = append(*scopeOf, CrossLedgerLegScope{OrganizationID: crossLedgerFeeOrganizationID, LedgerID: crossLedgerFeeLedgers[leg.ledger]})
		}

		return out
	}

	from := legs(shape.from, true, &scopes.Debits)
	to := legs(shape.to, false, &scopes.Credits)

	return crossLedgerTestTransaction(shape.total, from, to), scopes
}

func (shape crossLedgerFeeShape) decompose(t *testing.T) (CreateCrossLedgerTransactionV2Input, []decomposedCrossLedgerPart) {
	t.Helper()

	request, scopes := shape.request()
	in := CreateCrossLedgerTransactionV2Input{Transaction: request, Scopes: scopes}

	parts, err := decomposeCrossLedgerTransaction(request, internalCrossLedgerScopes(scopes))
	require.NoError(t, err)
	require.Len(t, parts, len(shape.ledgers))

	for index, ledger := range shape.ledgers {
		require.Equal(t, crossLedgerFeeLedgers[ledger], parts[index].ledgerRef.ledgerID, "part %d ledger", index)
	}

	return in, parts
}

// roundTrippedIntent is the intent a commit reads back from the group row.
func (shape crossLedgerFeeShape) roundTrippedIntent(t *testing.T, parts []decomposedCrossLedgerPart) CrossLedgerGroupIntent {
	t.Helper()

	intent, err := buildCrossLedgerGroupIntent("BRL", parts)
	require.NoError(t, err)

	raw, err := encodeCrossLedgerGroupIntent(intent)
	require.NoError(t, err)

	decoded, err := decodeCrossLedgerGroupIntent(raw)
	require.NoError(t, err)

	return *decoded
}

func crossLedgerFeeUseCase() (*UseCase, *fakeFeeApplier) {
	applier := &fakeFeeApplier{}
	reader := &atomicTransactionBatchSettingsReader{
		settings: mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}},
	}

	return &UseCase{
		TransactionReader: reader,
		FeeApplier:        applier,
		UUIDv7Generator:   func() (uuid.UUID, error) { return uuid.NewV7() },
		Clock:             func() time.Time { return time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC) },
	}, applier
}

// priceBatchItems freezes the batch and runs the per-item preparation, which
// ends at the fee seam, returning what each item handed the fee engine.
func priceBatchItems(t *testing.T, uc *UseCase, applier *fakeFeeApplier, in CreateAtomicTransactionBatchV2Input) []model.FeeCalculate {
	t.Helper()

	run, err := uc.initializeAtomicTransactionBatchV2(context.Background(), in)
	require.NoError(t, err)

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
	ctx, span := tracer.Start(context.Background(), "test.price_cross_ledger_items")
	t.Cleanup(func() { span.End() })

	for index := range run.items {
		require.NoError(t, uc.prepareAtomicTransactionBatchItem(ctx, span, logger, &run.items[index]))
	}

	require.Len(t, applier.received, len(run.items))

	return applier.received
}

// assertBridgeMarked checks the calculation marks exactly the expected legs and
// that each marked leg is the bridge alias at that position.
func assertBridgeMarked(t *testing.T, calculation model.FeeCalculate, want []model.NonPayerLeg) {
	t.Helper()

	if want == nil {
		assert.Empty(t, calculation.NonPayerLegs, "a part without a bridge marks no leg")
		return
	}

	require.Equal(t, want, calculation.NonPayerLegs)

	for _, leg := range calculation.NonPayerLegs {
		legs := calculation.Transaction.Send.Distribute.To
		if leg.IsFrom {
			legs = calculation.Transaction.Send.Source.From
		}

		require.Less(t, leg.Index, len(legs))
		assert.Equal(t, crossLedgerFeeBridgeAlias, legs[leg.Index].AccountAlias, "the marked leg must be the bridge")
	}
}

func TestCrossLedgerDirect_EveryPartMarksItsBridgeAsNonPayer(t *testing.T) {
	for _, shape := range crossLedgerFeeShapes() {
		t.Run(shape.name, func(t *testing.T) {
			uc, applier := crossLedgerFeeUseCase()
			in, parts := shape.decompose(t)

			calculations := priceBatchItems(t, uc, applier, buildCrossLedgerAtomicBatchInput(in, uuid.New(), parts))

			require.Len(t, calculations, len(shape.bridges))
			for index := range calculations {
				assert.Equal(t, crossLedgerFeeLedgers[shape.ledgers[index]], calculations[index].LedgerID)
				assertBridgeMarked(t, calculations[index], shape.bridges[index])
			}
		})
	}
}

func TestCrossLedgerHold_OriginsMarkTheirExactBridgeAsNonPayer(t *testing.T) {
	for _, shape := range crossLedgerFeeShapes() {
		t.Run(shape.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := transactiongroup.NewMockRepository(ctrl)
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

			uc, applier := crossLedgerFeeUseCase()
			uc.TransactionGroupRepo = repo

			var captured CreateAtomicTransactionBatchV2Input
			uc.createAtomicTransactionBatchV2 = func(_ context.Context, batch CreateAtomicTransactionBatchV2Input) (*CreateAtomicTransactionBatchV2Result, error) {
				captured = batch
				return &CreateAtomicTransactionBatchV2Result{BatchID: *batch.GroupID, Transactions: []*transaction.Transaction{{ID: uuid.NewString()}}}, nil
			}

			in, parts := shape.decompose(t)
			_, err := uc.CreateCrossLedgerHoldV2(context.Background(), in)
			require.NoError(t, err)

			var wantLedgers []uuid.UUID
			var wantBridges [][]model.NonPayerLeg
			for index := range parts {
				role, err := crossLedgerGroupPartRole(parts[index].transaction, "BRL")
				require.NoError(t, err)

				if role == CrossLedgerGroupRoleOrigin {
					wantLedgers = append(wantLedgers, crossLedgerFeeLedgers[shape.ledgers[index]])
					wantBridges = append(wantBridges, shape.bridges[index])
				}
			}

			calculations := priceBatchItems(t, uc, applier, captured)

			require.Len(t, calculations, len(wantBridges), "only the origins are priced at hold")
			for index := range calculations {
				assert.Equal(t, wantLedgers[index], calculations[index].LedgerID)
				assertBridgeMarked(t, calculations[index], wantBridges[index])
			}
		})
	}
}

func TestCrossLedgerCommit_DestinationsMarkTheBridgeDerivedFromTheIntent(t *testing.T) {
	for _, shape := range crossLedgerFeeShapes() {
		t.Run(shape.name, func(t *testing.T) {
			uc, applier := crossLedgerFeeUseCase()
			_, parts := shape.decompose(t)
			intent := shape.roundTrippedIntent(t, parts)

			var wantLedgers []uuid.UUID
			var wantBridges [][]model.NonPayerLeg
			for index := range intent.Parts {
				if intent.Parts[index].Role == CrossLedgerGroupRoleDestination {
					wantLedgers = append(wantLedgers, crossLedgerFeeLedgers[shape.ledgers[index]])
					wantBridges = append(wantBridges, shape.bridges[index])
				}
			}

			groupID := uuid.New()
			calculations := priceBatchItems(t, uc, applier, CreateAtomicTransactionBatchV2Input{
				Transactions:     crossLedgerGroupDestinationItems(intent),
				GroupID:          &groupID,
				CrossLedgerGroup: true,
			})

			require.Len(t, calculations, len(wantBridges), "only the destinations are priced at commit")
			for index := range calculations {
				assert.Equal(t, wantLedgers[index], calculations[index].LedgerID)
				assertBridgeMarked(t, calculations[index], wantBridges[index])
			}
		})
	}
}

// The commit has no recorded position, so the bridge it derives from the
// persisted intent must be the one the decomposition appended.
func TestCrossLedgerDestinationBridge_MatchesTheDecomposedPosition(t *testing.T) {
	for _, shape := range crossLedgerFeeShapes() {
		t.Run(shape.name, func(t *testing.T) {
			_, parts := shape.decompose(t)
			intent := shape.roundTrippedIntent(t, parts)

			destinations := 0
			for index := range intent.Parts {
				if intent.Parts[index].Role != CrossLedgerGroupRoleDestination {
					continue
				}

				destinations++

				require.NotNil(t, parts[index].bridge)
				assert.Equal(t, parts[index].bridge, crossLedgerDestinationBridge(intent.Parts[index].Transaction, intent.Asset))
			}

			require.Positive(t, destinations)
		})
	}
}

// A net-zero part whose client source is the bridge alias classifies as a
// destination, so the commit marks that client leg although the decomposition
// appended no bridge to it.
func TestCrossLedgerDestinationBridge_NetZeroPartWithAClientExternalSourceMarksThatLeg(t *testing.T) {
	shape := crossLedgerFeeShape{
		total:   "120",
		from:    []crossLedgerFeeLeg{{"@alice", "100", "A"}, {crossLedgerFeeBridgeAlias, "20", "N"}},
		to:      []crossLedgerFeeLeg{{"@nina", "20", "N"}, {"@bob", "100", "B"}},
		ledgers: []string{"A", "N", "B"},
	}

	_, parts := shape.decompose(t)
	intent := shape.roundTrippedIntent(t, parts)

	require.Nil(t, parts[1].bridge, "the net-zero part crosses nothing")
	require.Equal(t, CrossLedgerGroupRoleDestination, intent.Parts[1].Role)
	assert.Equal(t, &crossLedgerBridgePosition{isFrom: true, index: 0},
		crossLedgerDestinationBridge(intent.Parts[1].Transaction, intent.Asset))
}

func TestCrossLedgerDestinationBridge_NoBridgeAliasSourceMarksNothing(t *testing.T) {
	transaction := crossLedgerTestTransaction("10",
		[]mtransaction.FromTo{crossLedgerAmountLeg("@alice", "10", true)},
		[]mtransaction.FromTo{crossLedgerAmountLeg(crossLedgerFeeBridgeAlias, "10", false)})

	assert.Nil(t, crossLedgerDestinationBridge(transaction, "BRL"))
}

// Route assignment, default balance keys and the first validation run between
// the decomposition and the fee seam; none of them may move the bridge leg.
func TestCrossLedgerFeeSeam_TheRoutedBridgeKeepsItsPosition(t *testing.T) {
	routes := func(flow *groupRouteFlow) []mmodel.OperationRoute {
		return []mmodel.OperationRoute{flow.sourceRoute(), flow.directAndCommitDestination(), flow.bridgeRoute()}
	}

	assertRoutedBridge := func(t *testing.T, flow *groupRouteFlow, calculation model.FeeCalculate, want model.NonPayerLeg) {
		t.Helper()

		assertBridgeMarked(t, calculation, []model.NonPayerLeg{want})

		legs := calculation.Transaction.Send.Distribute.To
		if want.IsFrom {
			legs = calculation.Transaction.Send.Source.From
		}

		bridge := legs[want.Index]
		require.NotNil(t, bridge.RouteID, "the bridge carries the crossLedger route")
		assert.Equal(t, flow.bridge.String(), *bridge.RouteID)
		assert.Equal(t, constant.DefaultBalanceKey, bridge.BalanceKey)
	}

	t.Run("direct", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": true}, routes)
		applier := &fakeFeeApplier{}
		flow.uc.FeeApplier = applier

		parts := flow.decompose(t, flow.transfer(&flow.source, &flow.destination))
		_, err := flow.prepareBatch(t, buildCrossLedgerAtomicBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), parts))
		require.NoError(t, err)

		require.Len(t, applier.received, 2)
		assertRoutedBridge(t, flow, applier.received[0], model.NonPayerLeg{IsFrom: false, Index: 0})
		assertRoutedBridge(t, flow, applier.received[1], model.NonPayerLeg{IsFrom: true, Index: 0})
	})

	t.Run("commit", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": true}, routes)
		applier := &fakeFeeApplier{}
		flow.uc.FeeApplier = applier

		parts := flow.decompose(t, flow.transfer(&flow.source, &flow.destination))
		intent, err := buildCrossLedgerGroupIntent("BRL", parts)
		require.NoError(t, err)
		raw, err := encodeCrossLedgerGroupIntent(intent)
		require.NoError(t, err)
		decoded, err := decodeCrossLedgerGroupIntent(raw)
		require.NoError(t, err)

		groupID := uuid.New()
		_, err = flow.prepareBatch(t, CreateAtomicTransactionBatchV2Input{
			Transactions:     crossLedgerGroupDestinationItems(*decoded),
			GroupID:          &groupID,
			CrossLedgerGroup: true,
		})
		require.NoError(t, err)

		require.Len(t, applier.received, 1)
		assertRoutedBridge(t, flow, applier.received[0], model.NonPayerLeg{IsFrom: true, Index: 0})
	})
}

// Outside a cross-ledger group an @external leg is the world boundary and may
// pay a fee.
func TestAtomicBatchFeeSeam_OrdinaryItemsMarkNoNonPayerLeg(t *testing.T) {
	uc, applier := crossLedgerFeeUseCase()
	ledgerID := crossLedgerFeeLedgers["A"]

	calculations := priceBatchItems(t, uc, applier, CreateAtomicTransactionBatchV2Input{
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{
			atomicTransactionBatchItemInput(crossLedgerFeeOrganizationID, ledgerID, crossLedgerFeeBridgeAlias, "@bob"),
			atomicTransactionBatchItemInput(crossLedgerFeeOrganizationID, ledgerID, "@bob", crossLedgerFeeBridgeAlias),
		},
	})

	for index := range calculations {
		assert.Empty(t, calculations[index].NonPayerLegs, "item %d", index)
	}
}

func TestBuildCrossLedgerHoldBatchInput_RefusesADecompositionThatDoesNotAlignWithTheIntent(t *testing.T) {
	_, parts := crossLedgerFeeShapes()[0].decompose(t)
	intent := crossLedgerFeeShapes()[0].roundTrippedIntent(t, parts)

	_, err := buildCrossLedgerHoldBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), intent, parts[:1])

	require.Error(t, err)
}
