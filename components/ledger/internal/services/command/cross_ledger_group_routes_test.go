// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redisadapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// groupRouteFlow runs cross-ledger group preparation against the production
// route validation (a query use case over a mocked ledger-settings repository
// and route cache), for a group of two ledgers A and B of one organization.
type groupRouteFlow struct {
	organizationID     uuid.UUID
	ledgerA, ledgerB   uuid.UUID
	transactionRouteID uuid.UUID
	source             uuid.UUID
	destination        uuid.UUID
	bridge             uuid.UUID
	reader             *groupRouteFlowReader
	uc                 *UseCase
}

// groupRouteFlowReader answers ledger settings with each ledger's route
// validation.
type groupRouteFlowReader struct {
	*atomicTransactionBatchSettingsReader
	*groupRouteValidation
	routeValidation map[uuid.UUID]bool
}

// groupRouteValidation answers route validation with the production query use
// case and records which validation each part and phase went through.
type groupRouteValidation struct {
	routes            *query.UseCase
	partActions       []string
	phases            []string
	singleLedgerCalls int
}

func (reader *groupRouteFlowReader) GetParsedLedgerSettings(_ context.Context, _, ledgerID uuid.UUID) (mmodel.LedgerSettings, error) {
	settings := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
	settings.Accounting.ValidateRoutes = reader.routeValidation[ledgerID]

	return settings, nil
}

func (reader *groupRouteFlowReader) ValidateAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error) {
	return reader.validateSingleTransaction(ctx, organizationID, ledgerID, operations, validate, action)
}

func (reader *groupRouteValidation) GetOrCreateTransactionRouteCache(ctx context.Context, organizationID, transactionRouteID uuid.UUID) (mmodel.TransactionRouteCache, error) {
	return reader.routes.GetOrCreateTransactionRouteCache(ctx, organizationID, transactionRouteID)
}

func (reader *groupRouteValidation) validateSingleTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error) {
	reader.singleLedgerCalls++

	return reader.routes.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, action)
}

func (reader *groupRouteValidation) ValidateGroupPartAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error) {
	reader.partActions = append(reader.partActions, action)

	return reader.routes.ValidateGroupPartAccountingRules(ctx, organizationID, ledgerID, operations, validate, action)
}

func (reader *groupRouteValidation) ValidateGroupAccountingRoutes(ctx context.Context, organizationID uuid.UUID, transactionRoute string, uses []mmodel.AccountingRouteUse, action string) error {
	reader.phases = append(reader.phases, action)

	return reader.routes.ValidateGroupAccountingRoutes(ctx, organizationID, transactionRoute, uses, action)
}

func newGroupRouteFlow(t *testing.T, routeValidation map[string]bool, operationRoutes func(flow *groupRouteFlow) []mmodel.OperationRoute) *groupRouteFlow {
	t.Helper()

	flow := &groupRouteFlow{
		organizationID:     uuid.MustParse("0199b600-0000-7000-8000-000000000001"),
		ledgerA:            uuid.MustParse("0199b600-0000-7000-8000-00000000000a"),
		ledgerB:            uuid.MustParse("0199b600-0000-7000-8000-00000000000b"),
		transactionRouteID: uuid.MustParse("0199b600-0000-7000-8000-000000000002"),
		source:             uuid.MustParse("0199b600-0000-7000-8000-000000000003"),
		destination:        uuid.MustParse("0199b600-0000-7000-8000-000000000004"),
		bridge:             uuid.MustParse("0199b600-0000-7000-8000-000000000005"),
	}

	validation := map[uuid.UUID]bool{flow.ledgerA: routeValidation["A"], flow.ledgerB: routeValidation["B"]}

	balance := func(ledgerID uuid.UUID, id, alias string) *mmodel.Balance {
		return atomicTransactionBatchTestBalance(flow.organizationID, ledgerID, id, alias, "BRL")
	}

	flow.reader = &groupRouteFlowReader{
		atomicTransactionBatchSettingsReader: &atomicTransactionBatchSettingsReader{balances: []*mmodel.Balance{
			balance(flow.ledgerA, "0199b600-0000-7000-8000-0000000000a1", "@alice"),
			balance(flow.ledgerA, "0199b600-0000-7000-8000-0000000000a2", "@external/BRL"),
			balance(flow.ledgerB, "0199b600-0000-7000-8000-0000000000b1", "@bob"),
			balance(flow.ledgerB, "0199b600-0000-7000-8000-0000000000b2", "@external/BRL"),
		}},
		groupRouteValidation: &groupRouteValidation{
			routes: groupRouteQuery(t, flow.organizationID, flow.transactionRouteID, validation, operationRoutes(flow)),
		},
		routeValidation: validation,
	}

	flow.uc = &UseCase{
		TransactionReader: flow.reader,
		UUIDv7Generator:   func() (uuid.UUID, error) { return uuid.NewV7() },
		Clock:             func() time.Time { return time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC) },
	}

	return flow
}

// groupRouteQuery is the production route validation over a mocked ledger
// settings repository (route validation per ledger) and a route cache holding
// the transaction route made of operationRoutes.
func groupRouteQuery(t *testing.T, organizationID, transactionRouteID uuid.UUID, validation map[uuid.UUID]bool, operationRoutes []mmodel.OperationRoute) *query.UseCase {
	t.Helper()

	ctrl := gomock.NewController(t)

	ledgers := ledger.NewMockRepository(ctrl)
	ledgers.EXPECT().GetSettings(gomock.Any(), organizationID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, ledgerID uuid.UUID) (map[string]any, error) {
			return map[string]any{"accounting": map[string]any{"validateRoutes": validation[ledgerID]}}, nil
		}).AnyTimes()

	cache, err := (&mmodel.TransactionRoute{ID: transactionRouteID, OperationRoutes: operationRoutes}).ToCache().ToMsgpack()
	require.NoError(t, err)

	cacheRepo := redisadapter.NewMockRedisRepository(ctrl)
	cacheRepo.EXPECT().GetBytes(gomock.Any(), utils.AccountingRoutesInternalKey(organizationID, transactionRouteID)).
		Return(cache, nil).AnyTimes()

	return &query.UseCase{TransactionRedisRepo: cacheRepo, LedgerRepo: ledgers}
}

var (
	groupRouteBridgeDebit  = &mmodel.AccountingRubric{Code: "X-debit", Description: "Arriving from another ledger"}
	groupRouteBridgeCredit = &mmodel.AccountingRubric{Code: "X-credit", Description: "Leaving to another ledger"}
)

func groupRouteRubric(code string) *mmodel.AccountingRubric {
	return &mmodel.AccountingRubric{Code: code, Description: code}
}

func (flow *groupRouteFlow) sourceRoute() mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: flow.source, OperationType: constant.OperationRouteTypeSource, AccountingEntries: &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{Debit: groupRouteRubric("S-direct")},
		Hold:   &mmodel.AccountingEntry{Debit: groupRouteRubric("S-hold"), Credit: groupRouteRubric("S-hold")},
		Commit: &mmodel.AccountingEntry{Debit: groupRouteRubric("S-commit")},
	}}
}

func (flow *groupRouteFlow) destinationRoute(entries *mmodel.AccountingEntries) mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: flow.destination, OperationType: constant.OperationRouteTypeDestination, AccountingEntries: entries}
}

func (flow *groupRouteFlow) directAndCommitDestination() mmodel.OperationRoute {
	return flow.destinationRoute(&mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{Credit: groupRouteRubric("D-direct")},
		Commit: &mmodel.AccountingEntry{Credit: groupRouteRubric("D-commit")},
	})
}

func (flow *groupRouteFlow) commitOnlyDestination() mmodel.OperationRoute {
	return flow.destinationRoute(&mmodel.AccountingEntries{Commit: &mmodel.AccountingEntry{Credit: groupRouteRubric("D-commit")}})
}

func (flow *groupRouteFlow) directOnlyDestination() mmodel.OperationRoute {
	return flow.destinationRoute(&mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{Credit: groupRouteRubric("D-direct")}})
}

func (flow *groupRouteFlow) bridgeRoute() mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: flow.bridge, OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &mmodel.AccountingEntries{
		CrossLedger: &mmodel.AccountingEntry{Debit: groupRouteBridgeDebit, Credit: groupRouteBridgeCredit},
	}}
}

// transfer is a request moving 10 BRL from @alice in ledger A to @bob in ledger
// B, each leg naming the given operation route (none when nil).
func (flow *groupRouteFlow) transfer(sourceRoute, destinationRoute *uuid.UUID) mtransaction.Transaction {
	routeID := flow.transactionRouteID.String()
	leg := func(alias string, route *uuid.UUID, isFrom bool) mtransaction.FromTo {
		ft := mtransaction.FromTo{AccountAlias: alias, Amount: &mtransaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(10)}, IsFrom: isFrom}
		if route != nil {
			id := route.String()
			ft.RouteID = &id
		}

		return ft
	}

	return mtransaction.Transaction{
		Description: "cross-ledger routed transfer",
		RouteID:     &routeID,
		Send: mtransaction.Send{
			Asset:      "BRL",
			Value:      decimal.NewFromInt(10),
			Source:     mtransaction.Source{From: []mtransaction.FromTo{leg("@alice", sourceRoute, true)}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{leg("@bob", destinationRoute, false)}},
		},
	}
}

// decompose splits the request into its ledger parts and stamps the bridge legs
// of the route-validating ones, as the direct and hold entry points do.
func (flow *groupRouteFlow) decompose(t *testing.T, request mtransaction.Transaction) []decomposedCrossLedgerPart {
	t.Helper()

	a := atomicTransactionBatchLedgerRef{organizationID: flow.organizationID, ledgerID: flow.ledgerA}
	b := atomicTransactionBatchLedgerRef{organizationID: flow.organizationID, ledgerID: flow.ledgerB}

	parts, err := decomposeCrossLedgerTransaction(request, crossLedgerTransactionScopes{
		from: []atomicTransactionBatchLedgerRef{a},
		to:   []atomicTransactionBatchLedgerRef{b},
	})
	require.NoError(t, err)
	require.NoError(t, flow.uc.routeCrossLedgerBridgeLegs(context.Background(), request, parts))

	return parts
}

// prepareBatch freezes and prepares a group batch as CreateAtomicTransactionBatchV2
// does, and returns the run with the error of the first failing step.
func (flow *groupRouteFlow) prepareBatch(t *testing.T, in CreateAtomicTransactionBatchV2Input) (*atomicTransactionBatchRun, error) {
	t.Helper()

	run, err := flow.uc.initializeAtomicTransactionBatchV2(context.Background(), in)
	require.NoError(t, err)

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
	ctx, span := tracer.Start(context.Background(), "test.prepare_cross_ledger_group_routes")
	t.Cleanup(func() { span.End() })

	return run, flow.uc.prepareAtomicTransactionBatchItems(ctx, span, logger, run)
}

func (flow *groupRouteFlow) runBatch(t *testing.T, in CreateAtomicTransactionBatchV2Input) (*atomicTransactionBatchRun, error) {
	t.Helper()

	run, err := flow.prepareBatch(t, in)
	if err != nil {
		return run, err
	}

	return run, flow.uc.validateCrossLedgerBatchGroupRoutes(context.Background(), run)
}

func requireGroupRouteCode(t *testing.T, err error, want error) {
	t.Helper()

	require.Error(t, err)

	var unprocessable pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &unprocessable), "expected a business error, got %T: %v", err, err)
	assert.Equal(t, want.Error(), unprocessable.Code, unprocessable.Message)
}

// projectedRouteCodes maps each projected operation's balance to its route code.
func projectedRouteCodes(item atomicTransactionBatchItemRun) map[string]string {
	codes := make(map[string]string)
	for _, spec := range item.prepared.projection {
		codes[spec.BalanceRef] = spec.RouteCode
	}

	return codes
}

func TestCrossLedgerDirectGroup_ValidatesRoutesOverTheWholeGroup(t *testing.T) {
	routes := func(flow *groupRouteFlow) []mmodel.OperationRoute {
		return []mmodel.OperationRoute{flow.sourceRoute(), flow.directAndCommitDestination(), flow.bridgeRoute()}
	}
	flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": true}, routes)

	parts := flow.decompose(t, flow.transfer(&flow.source, &flow.destination))
	run, err := flow.runBatch(t, buildCrossLedgerAtomicBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), parts))
	require.NoError(t, err)

	assert.Equal(t, []string{constant.ActionDirect, constant.ActionDirect}, flow.reader.partActions)
	assert.Equal(t, []string{constant.ActionDirect}, flow.reader.phases)
	assert.Zero(t, flow.reader.singleLedgerCalls, "group parts never take the single-transaction count")

	assert.Equal(t, map[string]string{"@alice#default": "S-direct", "@external/BRL#default": "X-credit"}, projectedRouteCodes(run.items[0]))
	assert.Equal(t, map[string]string{"@external/BRL#default": "X-debit", "@bob#default": "D-direct"}, projectedRouteCodes(run.items[1]))
}

func TestCrossLedgerDirectGroup_RefusesAGroupThatDoesNotCoverTheTemplate(t *testing.T) {
	secondDestination := uuid.MustParse("0199b600-0000-7000-8000-0000000000ee")
	routes := func(flow *groupRouteFlow) []mmodel.OperationRoute {
		unused := flow.directAndCommitDestination()
		unused.ID = secondDestination

		return []mmodel.OperationRoute{flow.sourceRoute(), flow.directAndCommitDestination(), unused, flow.bridgeRoute()}
	}
	flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": true}, routes)

	parts := flow.decompose(t, flow.transfer(&flow.source, &flow.destination))
	_, err := flow.runBatch(t, buildCrossLedgerAtomicBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), parts))

	requireGroupRouteCode(t, err, constant.ErrAccountingRouteCountMismatch)
}

// A part in a ledger that does not validate routes is not checked leg by leg,
// but every client leg of it that names a route counts toward the group's
// coverage of the template; a leg with no route adds nothing.
func TestCrossLedgerDirectGroup_MixedGroupCountsTheNamedRoutesOfEveryPart(t *testing.T) {
	bidirectionalRoutes := func(flow *groupRouteFlow) []mmodel.OperationRoute {
		return []mmodel.OperationRoute{
			{ID: flow.source, OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{Debit: groupRouteRubric("S-direct"), Credit: groupRouteRubric("S-direct")},
			}},
			flow.bridgeRoute(),
		}
	}
	sourceAndDestinationRoutes := func(flow *groupRouteFlow) []mmodel.OperationRoute {
		return []mmodel.OperationRoute{flow.sourceRoute(), flow.directAndCommitDestination(), flow.bridgeRoute()}
	}

	t.Run("an unrouted part adds nothing and is not validated", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": false}, bidirectionalRoutes)

		parts := flow.decompose(t, flow.transfer(&flow.source, nil))
		run, err := flow.runBatch(t, buildCrossLedgerAtomicBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), parts))
		require.NoError(t, err)

		assert.Equal(t, []string{constant.ActionDirect}, flow.reader.partActions, "only the validating part is validated")
		assert.Equal(t, map[string]string{"@external/BRL#default": "", "@bob#default": ""}, projectedRouteCodes(run.items[1]))
	})

	t.Run("the validating part is still checked", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": false}, bidirectionalRoutes)
		foreign := uuid.MustParse("0199b600-0000-7000-8000-0000000000ef")

		parts := flow.decompose(t, flow.transfer(&foreign, nil))
		_, err := flow.runBatch(t, buildCrossLedgerAtomicBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), parts))

		requireGroupRouteCode(t, err, constant.ErrAccountingRouteNotFound)
	})

	t.Run("a named route in the non-validating part completes the template", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": false}, sourceAndDestinationRoutes)

		parts := flow.decompose(t, flow.transfer(&flow.source, &flow.destination))
		run, err := flow.runBatch(t, buildCrossLedgerAtomicBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), parts))
		require.NoError(t, err)

		assert.Equal(t, []string{constant.ActionDirect}, flow.reader.partActions, "only the validating part is validated leg by leg")
		assert.Equal(t, []string{constant.ActionDirect}, flow.reader.phases)
		assert.Equal(t, map[string]string{"@external/BRL#default": "", "@bob#default": ""}, projectedRouteCodes(run.items[1]),
			"a part in a non-validating ledger posts without rubrics")
	})

	t.Run("the same transfer without a route on the non-validating leg", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": false}, sourceAndDestinationRoutes)

		parts := flow.decompose(t, flow.transfer(&flow.source, nil))
		_, err := flow.runBatch(t, buildCrossLedgerAtomicBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), parts))

		requireGroupRouteCode(t, err, constant.ErrAccountingRouteCountMismatch)
	})

	t.Run("a named route of the non-validating part outside the template", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": false}, sourceAndDestinationRoutes)
		foreign := uuid.MustParse("0199b600-0000-7000-8000-0000000000ef")

		parts := flow.decompose(t, flow.transfer(&flow.source, &foreign))
		_, err := flow.runBatch(t, buildCrossLedgerAtomicBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), parts))

		requireGroupRouteCode(t, err, constant.ErrAccountingRouteNotFound)
	})
}

// At hold only the origins execute; the destination parts persisted in the
// intent complete the hold template, whose destination side is the commit's.
func TestCrossLedgerHoldGroup_CoversTheHoldTemplateWithTheIntentDestinations(t *testing.T) {
	holdBatchTo := func(t *testing.T, flow *groupRouteFlow, destinationRoute *uuid.UUID) CreateAtomicTransactionBatchV2Input {
		t.Helper()

		parts := flow.decompose(t, flow.transfer(&flow.source, destinationRoute))
		intent, err := buildCrossLedgerGroupIntent("BRL", parts)
		require.NoError(t, err)

		batch, err := buildCrossLedgerHoldBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), intent)
		require.NoError(t, err)

		return batch
	}
	holdBatch := func(t *testing.T, flow *groupRouteFlow) CreateAtomicTransactionBatchV2Input {
		t.Helper()

		return holdBatchTo(t, flow, &flow.destination)
	}

	t.Run("destination route configured for commit", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": true}, func(flow *groupRouteFlow) []mmodel.OperationRoute {
			return []mmodel.OperationRoute{flow.sourceRoute(), flow.commitOnlyDestination(), flow.bridgeRoute()}
		})

		batch := holdBatch(t, flow)
		require.Len(t, batch.Transactions, 1, "only the origin executes at hold")

		_, err := flow.runBatch(t, batch)
		require.NoError(t, err)

		assert.Equal(t, []string{constant.ActionHold}, flow.reader.partActions)
		assert.Equal(t, []string{constant.ActionHold}, flow.reader.phases)
	})

	t.Run("destination route without a commit entry", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": true}, func(flow *groupRouteFlow) []mmodel.OperationRoute {
			return []mmodel.OperationRoute{flow.sourceRoute(), flow.directOnlyDestination(), flow.bridgeRoute()}
		})

		_, err := flow.runBatch(t, holdBatch(t, flow))

		requireGroupRouteCode(t, err, constant.ErrAccountingRouteNotFound)
	})

	t.Run("destination in a non-validating ledger naming its route", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": false}, func(flow *groupRouteFlow) []mmodel.OperationRoute {
			return []mmodel.OperationRoute{flow.sourceRoute(), flow.commitOnlyDestination(), flow.bridgeRoute()}
		})

		_, err := flow.runBatch(t, holdBatch(t, flow))
		require.NoError(t, err)

		assert.Equal(t, []string{constant.ActionHold}, flow.reader.phases)
	})

	t.Run("destination in a non-validating ledger without a route", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": false}, func(flow *groupRouteFlow) []mmodel.OperationRoute {
			return []mmodel.OperationRoute{flow.sourceRoute(), flow.commitOnlyDestination(), flow.bridgeRoute()}
		})

		_, err := flow.runBatch(t, holdBatchTo(t, flow, nil))

		requireGroupRouteCode(t, err, constant.ErrAccountingRouteCountMismatch)
	})
}

// A commit approves the origins and creates the destinations in one execution;
// both belong to the commit phase and are validated against its template.
func TestCrossLedgerCommitGroup_ValidatesTheDestinationsAsCommit(t *testing.T) {
	commitTo := func(t *testing.T, flow *groupRouteFlow, destinationRoute *uuid.UUID) (*atomicTransactionBatchRun, error) {
		t.Helper()

		parts := flow.decompose(t, flow.transfer(&flow.source, destinationRoute))
		intent, err := buildCrossLedgerGroupIntent("BRL", parts)
		require.NoError(t, err)

		origin := intent.Parts[0]
		require.Equal(t, CrossLedgerGroupRoleOrigin, origin.Role)

		input, err := clonePendingTransactionInput(origin.Transaction)
		require.NoError(t, err)

		input.Pending = true
		normalizeTransactionSendLegs(&input)

		validate, err := mtransaction.ValidateSendSourceAndDistribute(context.Background(), input, constant.APPROVED)
		require.NoError(t, err)
		mtransaction.PropagateRouteValidation(context.Background(), validate, constant.APPROVED)

		_, originRoutes, err := flow.uc.prepareCrossLedgerGroupPart(context.Background(), enginePreparationInput{
			organizationID: origin.OrganizationID,
			ledgerID:       origin.LedgerID,
			translation: EngineTranslationInput{
				TransactionID: uuid.New(), Action: constant.ActionCommit, TransactionStatus: constant.APPROVED,
				RouteValidationEnabled: true, TransactionInput: input, Validate: validate,
			},
		}, true)
		if err != nil {
			return nil, err
		}

		destinations, err := flow.prepareBatch(t, CreateAtomicTransactionBatchV2Input{
			Transactions:     crossLedgerGroupDestinationItems(intent),
			GroupID:          cloneUUIDPointer(&flow.transactionRouteID),
			CrossLedgerGroup: true,
		})
		if err != nil {
			return destinations, err
		}

		return destinations, flow.uc.validateCrossLedgerCommitRoutes(context.Background(), []crossLedgerPendingGroupPart{{routes: originRoutes}}, destinations)
	}
	commit := func(t *testing.T, flow *groupRouteFlow) (*atomicTransactionBatchRun, error) {
		t.Helper()

		return commitTo(t, flow, &flow.destination)
	}

	t.Run("destination route configured for commit only", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": true}, func(flow *groupRouteFlow) []mmodel.OperationRoute {
			return []mmodel.OperationRoute{flow.sourceRoute(), flow.commitOnlyDestination(), flow.bridgeRoute()}
		})

		destinations, err := commit(t, flow)
		require.NoError(t, err)

		assert.Equal(t, []string{constant.ActionCommit, constant.ActionCommit}, flow.reader.partActions)
		assert.Equal(t, []string{constant.ActionCommit}, flow.reader.phases)
		require.Len(t, destinations.items, 1)
		assert.Equal(t, constant.ActionDirect, destinations.items[0].action, "the destination still posts as a direct transaction")
		assert.Equal(t, map[string]string{"@external/BRL#default": "X-debit", "@bob#default": "D-commit"}, projectedRouteCodes(destinations.items[0]))
	})

	t.Run("destination route without a commit entry", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": true}, func(flow *groupRouteFlow) []mmodel.OperationRoute {
			return []mmodel.OperationRoute{flow.sourceRoute(), flow.directOnlyDestination(), flow.bridgeRoute()}
		})

		_, err := commit(t, flow)

		requireGroupRouteCode(t, err, constant.ErrAccountingRouteNotFound)
	})

	t.Run("destination in a non-validating ledger naming its route", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": false}, func(flow *groupRouteFlow) []mmodel.OperationRoute {
			return []mmodel.OperationRoute{flow.sourceRoute(), flow.commitOnlyDestination(), flow.bridgeRoute()}
		})

		_, err := commit(t, flow)
		require.NoError(t, err)

		assert.Equal(t, []string{constant.ActionCommit}, flow.reader.partActions, "only the validating origin is validated leg by leg")
		assert.Equal(t, []string{constant.ActionCommit}, flow.reader.phases)
	})

	t.Run("destination in a non-validating ledger without a route", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": false}, func(flow *groupRouteFlow) []mmodel.OperationRoute {
			return []mmodel.OperationRoute{flow.sourceRoute(), flow.commitOnlyDestination(), flow.bridgeRoute()}
		})

		_, err := commitTo(t, flow, nil)

		requireGroupRouteCode(t, err, constant.ErrAccountingRouteCountMismatch)
	})
}

// A group revert reverses every part in one batch: the reversals together use
// the revert template, and each bridge leg takes the rubric of the direction it
// now posts.
func TestCrossLedgerRevertGroup_ValidatesTheReversalsAgainstTheRevertTemplate(t *testing.T) {
	revertRoutes := func(extra ...mmodel.OperationRoute) func(flow *groupRouteFlow) []mmodel.OperationRoute {
		return func(flow *groupRouteFlow) []mmodel.OperationRoute {
			bidirectional := func(id uuid.UUID, code string) mmodel.OperationRoute {
				return mmodel.OperationRoute{ID: id, OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &mmodel.AccountingEntries{
					Direct: &mmodel.AccountingEntry{Debit: groupRouteRubric(code + "-direct"), Credit: groupRouteRubric(code + "-direct")},
					Revert: &mmodel.AccountingEntry{Debit: groupRouteRubric(code + "-revert"), Credit: groupRouteRubric(code + "-revert")},
				}}
			}

			return append([]mmodel.OperationRoute{bidirectional(flow.source, "S"), bidirectional(flow.destination, "D"), flow.bridgeRoute()}, extra...)
		}
	}

	revertBatch := func(t *testing.T, flow *groupRouteFlow) CreateAtomicTransactionBatchV2Input {
		t.Helper()

		parts := flow.decompose(t, flow.transfer(&flow.source, &flow.destination))
		reversals := make([]preparedCrossLedgerRevertPart, len(parts))

		for index, part := range parts {
			origin := &transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: part.ledgerRef.organizationID.String(),
				LedgerID:       part.ledgerRef.ledgerID.String(),
				Body:           part.transaction,
			}

			reversals[index] = preparedCrossLedgerRevertPart{origin: origin, reversal: reverseGroupPart(part.transaction)}
		}

		batch, err := buildCrossLedgerRevertBatchInput(RevertTransactionInput{}, uuid.New(), uuid.New(), reversals)
		require.NoError(t, err)

		return batch
	}

	t.Run("the reversals cover the revert template", func(t *testing.T) {
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": true}, revertRoutes())

		run, err := flow.runBatch(t, revertBatch(t, flow))
		require.NoError(t, err)

		assert.Equal(t, []string{constant.ActionRevert, constant.ActionRevert}, flow.reader.partActions)
		assert.Equal(t, []string{constant.ActionRevert}, flow.reader.phases)

		codes := map[uuid.UUID]map[string]string{}
		for index := range run.items {
			codes[run.items[index].ledgerID] = projectedRouteCodes(run.items[index])
		}

		assert.Equal(t, map[string]string{"@external/BRL#default": "X-debit", "@alice#default": "S-revert"}, codes[flow.ledgerA])
		assert.Equal(t, map[string]string{"@bob#default": "D-revert", "@external/BRL#default": "X-credit"}, codes[flow.ledgerB])
	})

	t.Run("a revert route no reversal uses", func(t *testing.T) {
		unused := mmodel.OperationRoute{ID: uuid.MustParse("0199b600-0000-7000-8000-0000000000ed"), OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &mmodel.AccountingEntries{
			Revert: &mmodel.AccountingEntry{Debit: groupRouteRubric("U"), Credit: groupRouteRubric("U")},
		}}
		flow := newGroupRouteFlow(t, map[string]bool{"A": true, "B": true}, revertRoutes(unused))

		_, err := flow.runBatch(t, revertBatch(t, flow))

		requireGroupRouteCode(t, err, constant.ErrAccountingRouteCountMismatch)
	})
}

// reverseGroupPart swaps a part's sides, as the reversal of a posted part does.
func reverseGroupPart(part mtransaction.Transaction) mtransaction.Transaction {
	reversed := part

	from := make([]mtransaction.FromTo, 0, len(part.Send.Distribute.To))
	for _, leg := range part.Send.Distribute.To {
		leg.IsFrom = true
		from = append(from, leg)
	}

	to := make([]mtransaction.FromTo, 0, len(part.Send.Source.From))
	for _, leg := range part.Send.Source.From {
		leg.IsFrom = false
		to = append(to, leg)
	}

	reversed.Send.Source.From = from
	reversed.Send.Distribute.To = to

	return reversed
}

type groupRouteTransitionReader struct {
	*crossLedgerLifecycleReader
	*groupRouteValidation
}

func (reader *groupRouteTransitionReader) ValidateAccountingRules(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error) {
	return reader.validateSingleTransaction(ctx, organizationID, ledgerID, operations, validate, action)
}

// A group commit validates each origin transition as a group part, bridge
// included, and the commit phase over the group. Only the origin ledger
// validates routes here; validating destinations are covered by
// TestCrossLedgerCommitGroup_ValidatesTheDestinationsAsCommit.
func TestTransitionCrossLedgerGroupV2_CommitValidatesTheOriginAsAGroupPart(t *testing.T) {
	transactionRouteID := uuid.MustParse("0199b700-0000-7000-8000-000000000001")
	clientRouteID := uuid.MustParse("0199b700-0000-7000-8000-000000000002")
	bridgeRouteID := uuid.MustParse("0199b700-0000-7000-8000-000000000003")

	clientRoute := mmodel.OperationRoute{ID: clientRouteID, OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &mmodel.AccountingEntries{
		Hold:   &mmodel.AccountingEntry{Debit: groupRouteRubric("C-hold"), Credit: groupRouteRubric("C-hold")},
		Commit: &mmodel.AccountingEntry{Debit: groupRouteRubric("C-commit"), Credit: groupRouteRubric("C-commit")},
	}}

	commit := func(t *testing.T, bridgeEntry *mmodel.AccountingEntry) (*groupRouteValidation, error) {
		t.Helper()

		validating := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
		validating.Accounting.ValidateRoutes = true

		uc, repo, _, target, in, group := newCrossLedgerLifecycleFixtureWith(t, constant.APPROVED, crossLedgerLifecycleSetup{
			route: func(request mtransaction.Transaction) mtransaction.Transaction {
				routeID, clientRoute := transactionRouteID.String(), clientRouteID.String()
				request.RouteID = &routeID
				request.Send.Source.From[0].RouteID = &clientRoute

				return request
			},
			stampParts: func(parts []decomposedCrossLedgerPart) { parts[0].assignBridgeRoute(bridgeRouteID.String()) },
			settingsA:  &validating,
		})

		originLedgerID := uuid.MustParse(target.LedgerID)
		bridge := mmodel.OperationRoute{ID: bridgeRouteID, OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &mmodel.AccountingEntries{CrossLedger: bridgeEntry}}
		validation := &groupRouteValidation{routes: groupRouteQuery(t, group.OrganizationID, transactionRouteID,
			map[uuid.UUID]bool{originLedgerID: true}, []mmodel.OperationRoute{clientRoute, bridge})}

		uc.TransactionReader = &groupRouteTransitionReader{
			crossLedgerLifecycleReader: uc.TransactionReader.(*crossLedgerLifecycleReader),
			groupRouteValidation:       validation,
		}

		// A refused commit releases the pending locks it took.
		uc.TransactionRedisRepo.(*redisadapter.MockRedisRepository).EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		repo.EXPECT().FindByID(gomock.Any(), group.ID).Return(group, nil)
		repo.EXPECT().UpdateStatus(gomock.Any(), group.ID, constant.PENDING, constant.APPROVED).Return(true, nil).MaxTimes(1)

		_, err := uc.transitionCrossLedgerGroupV2(context.Background(), in, target, constant.APPROVED)

		return validation, err
	}

	t.Run("the origin and the commit phase pass", func(t *testing.T) {
		validation, err := commit(t, &mmodel.AccountingEntry{Debit: groupRouteBridgeDebit, Credit: groupRouteBridgeCredit})
		require.NoError(t, err)

		assert.Equal(t, []string{constant.ActionCommit}, validation.partActions, "only the validating origin is validated")
		assert.Equal(t, []string{constant.ActionCommit}, validation.phases)
		assert.Zero(t, validation.singleLedgerCalls)
	})

	t.Run("the origin bridge needs the credit rubric", func(t *testing.T) {
		_, err := commit(t, &mmodel.AccountingEntry{Debit: groupRouteBridgeDebit})

		requireGroupRouteCode(t, err, constant.ErrCrossLedgerRouteNotConfigured)
	})
}
