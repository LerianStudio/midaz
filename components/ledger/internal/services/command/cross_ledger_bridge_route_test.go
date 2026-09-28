// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// bridgeRouteReader serves per-ledger settings and one transaction route cache.
type bridgeRouteReader struct {
	atomicTransactionBatchSettingsReader
	cache      mmodel.TransactionRouteCache
	cacheCalls []uuid.UUID
}

func (reader *bridgeRouteReader) GetOrCreateTransactionRouteCache(_ context.Context, _, transactionRouteID uuid.UUID) (mmodel.TransactionRouteCache, error) {
	reader.cacheCalls = append(reader.cacheCalls, transactionRouteID)

	return reader.cache, nil
}

type bridgeRouteFixture struct {
	organizationID     uuid.UUID
	ledgerA, ledgerB   atomicTransactionBatchLedgerRef
	ledgerC            atomicTransactionBatchLedgerRef
	transactionRouteID uuid.UUID
	bridgeRouteID      string
	clientRouteID      string
}

func newBridgeRouteFixture() bridgeRouteFixture {
	organizationID := uuid.MustParse("0199a400-0000-7000-8000-000000000001")

	return bridgeRouteFixture{
		organizationID:     organizationID,
		ledgerA:            atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199a400-0000-7000-8000-000000000002")},
		ledgerB:            atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199a400-0000-7000-8000-000000000003")},
		ledgerC:            atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199a400-0000-7000-8000-000000000004")},
		transactionRouteID: uuid.MustParse("0199a400-0000-7000-8000-000000000005"),
		bridgeRouteID:      "0199a400-0000-7000-8000-000000000006",
		clientRouteID:      "0199a400-0000-7000-8000-000000000007",
	}
}

func bridgeRouteCache(bridgeRouteIDs ...string) mmodel.TransactionRouteCache {
	bridges := make(map[string]mmodel.OperationRouteCache, len(bridgeRouteIDs))
	for _, id := range bridgeRouteIDs {
		bridges[id] = mmodel.OperationRouteCache{
			OperationType: constant.OperationRouteTypeBidirectional,
			AccountingEntries: &mmodel.AccountingEntries{CrossLedger: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "1900", Description: "Arriving"},
				Credit: &mmodel.AccountingRubric{Code: "2900", Description: "Leaving"},
			}},
		}
	}

	cache := mmodel.TransactionRouteCache{Actions: map[string]mmodel.ActionRouteCache{
		constant.ActionDirect: {Source: map[string]mmodel.OperationRouteCache{
			"0199a400-0000-7000-8000-000000000007": {OperationType: constant.OperationRouteTypeSource},
		}},
	}}
	if len(bridges) > 0 {
		cache.Actions[constant.ActionCrossLedger] = mmodel.ActionRouteCache{Bidirectional: bridges}
	}

	return cache
}

func (f bridgeRouteFixture) reader(cache mmodel.TransactionRouteCache, validating ...atomicTransactionBatchLedgerRef) *bridgeRouteReader {
	settingsByRef := map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{}
	for _, ref := range []atomicTransactionBatchLedgerRef{f.ledgerA, f.ledgerB, f.ledgerC} {
		settingsByRef[ref] = mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
	}

	for _, ref := range validating {
		settings := settingsByRef[ref]
		settings.Accounting.ValidateRoutes = true
		settingsByRef[ref] = settings
	}

	return &bridgeRouteReader{
		atomicTransactionBatchSettingsReader: atomicTransactionBatchSettingsReader{settingsByRef: settingsByRef},
		cache:                                cache,
	}
}

func (f bridgeRouteFixture) routedTransaction(total string, from, to []mtransaction.FromTo) mtransaction.Transaction {
	tx := crossLedgerTestTransaction(total, from, to)
	routeID := f.transactionRouteID.String()
	tx.RouteID = &routeID

	return tx
}

func (f bridgeRouteFixture) clientLeg(alias, amount string, isFrom bool) mtransaction.FromTo {
	leg := crossLedgerAmountLeg(alias, amount, isFrom)
	routeID := f.clientRouteID
	leg.RouteID = &routeID

	return leg
}

func routeOf(leg mtransaction.FromTo) string {
	if leg.RouteID == nil {
		return ""
	}

	return *leg.RouteID
}

func TestRouteCrossLedgerBridgeLegs(t *testing.T) {
	t.Parallel()

	f := newBridgeRouteFixture()
	oneToOne := func() (mtransaction.Transaction, crossLedgerTransactionScopes) {
		return f.routedTransaction("100",
				[]mtransaction.FromTo{f.clientLeg("@debit", "100", true)},
				[]mtransaction.FromTo{f.clientLeg("@credit", "100", false)}),
			crossLedgerTransactionScopes{from: []atomicTransactionBatchLedgerRef{f.ledgerA}, to: []atomicTransactionBatchLedgerRef{f.ledgerB}}
	}

	t.Run("every validating part's bridge leg takes the bridge route and client legs keep theirs", func(t *testing.T) {
		t.Parallel()

		tx, scopes := oneToOne()
		parts, err := decomposeCrossLedgerTransaction(tx, scopes)
		require.NoError(t, err)

		reader := f.reader(bridgeRouteCache(f.bridgeRouteID), f.ledgerA, f.ledgerB)
		require.NoError(t, (&UseCase{TransactionReader: reader}).routeCrossLedgerBridgeLegs(context.Background(), tx, parts))

		require.Len(t, parts, 2)
		assert.Equal(t, f.clientRouteID, routeOf(parts[0].transaction.Send.Source.From[0]))
		assert.Equal(t, f.bridgeRouteID, routeOf(parts[0].transaction.Send.Distribute.To[0]), "origin bridge leg")
		assert.Equal(t, f.bridgeRouteID, routeOf(parts[1].transaction.Send.Source.From[0]), "destination bridge leg")
		assert.Equal(t, f.clientRouteID, routeOf(parts[1].transaction.Send.Distribute.To[0]))
		assert.Equal(t, []uuid.UUID{f.transactionRouteID}, reader.cacheCalls, "the transaction route is loaded once")
	})

	t.Run("a part whose ledger does not validate routes keeps an unrouted bridge leg", func(t *testing.T) {
		t.Parallel()

		tx, scopes := oneToOne()
		parts, err := decomposeCrossLedgerTransaction(tx, scopes)
		require.NoError(t, err)

		reader := f.reader(bridgeRouteCache(f.bridgeRouteID), f.ledgerA)
		require.NoError(t, (&UseCase{TransactionReader: reader}).routeCrossLedgerBridgeLegs(context.Background(), tx, parts))

		assert.Equal(t, f.bridgeRouteID, routeOf(parts[0].transaction.Send.Distribute.To[0]))
		assert.Nil(t, parts[1].transaction.Send.Source.From[0].RouteID)
	})

	t.Run("a validating ledger that crosses nothing needs no bridge route", func(t *testing.T) {
		t.Parallel()

		tx := f.routedTransaction("100",
			[]mtransaction.FromTo{f.clientLeg("@a-debit", "50", true), f.clientLeg("@b-debit", "50", true)},
			[]mtransaction.FromTo{f.clientLeg("@a-credit", "50", false), f.clientLeg("@c-credit", "50", false)})
		parts, err := decomposeCrossLedgerTransaction(tx, crossLedgerTransactionScopes{
			from: []atomicTransactionBatchLedgerRef{f.ledgerA, f.ledgerB},
			to:   []atomicTransactionBatchLedgerRef{f.ledgerA, f.ledgerC},
		})
		require.NoError(t, err)
		require.Len(t, parts, 3)
		require.Len(t, parts[0].transaction.Send.Source.From, 1, "the net-zero ledger has no bridge leg")
		require.Len(t, parts[0].transaction.Send.Distribute.To, 1, "the net-zero ledger has no bridge leg")

		reader := f.reader(bridgeRouteCache(), f.ledgerA)
		require.NoError(t, (&UseCase{TransactionReader: reader}).routeCrossLedgerBridgeLegs(context.Background(), tx, parts))

		assert.Empty(t, reader.cacheCalls)
		for _, part := range parts {
			for _, leg := range append(part.transaction.Send.Source.From, part.transaction.Send.Distribute.To...) {
				if leg.AccountAlias == "@external/BRL" {
					assert.Nil(t, leg.RouteID)
				}
			}
		}
	})

	t.Run("a request without a transaction route stamps nothing and reads nothing", func(t *testing.T) {
		t.Parallel()

		tx, scopes := oneToOne()
		tx.RouteID = nil
		parts, err := decomposeCrossLedgerTransaction(tx, scopes)
		require.NoError(t, err)

		reader := f.reader(bridgeRouteCache(f.bridgeRouteID), f.ledgerA, f.ledgerB)
		require.NoError(t, (&UseCase{TransactionReader: reader}).routeCrossLedgerBridgeLegs(context.Background(), tx, parts))

		assert.Zero(t, reader.calls)
		assert.Empty(t, reader.cacheCalls)
		assert.Nil(t, parts[0].transaction.Send.Distribute.To[0].RouteID)
	})

	t.Run("a transaction route without a bridge route is refused for a validating part", func(t *testing.T) {
		t.Parallel()

		tx, scopes := oneToOne()
		parts, err := decomposeCrossLedgerTransaction(tx, scopes)
		require.NoError(t, err)

		err = (&UseCase{TransactionReader: f.reader(bridgeRouteCache(), f.ledgerB)}).routeCrossLedgerBridgeLegs(context.Background(), tx, parts)

		var businessErr pkg.UnprocessableOperationError
		require.ErrorAs(t, err, &businessErr)
		assert.Equal(t, constant.ErrCrossLedgerRouteNotConfigured.Error(), businessErr.Code)
	})

	t.Run("a transaction route with two bridge routes is refused", func(t *testing.T) {
		t.Parallel()

		tx, scopes := oneToOne()
		parts, err := decomposeCrossLedgerTransaction(tx, scopes)
		require.NoError(t, err)

		err = (&UseCase{TransactionReader: f.reader(bridgeRouteCache(f.bridgeRouteID, uuid.NewString()), f.ledgerA)}).
			routeCrossLedgerBridgeLegs(context.Background(), tx, parts)

		var businessErr pkg.UnprocessableOperationError
		require.ErrorAs(t, err, &businessErr)
		assert.Equal(t, constant.ErrInvalidCrossLedgerRoute.Error(), businessErr.Code)
	})

	t.Run("a participant without cross-ledger enabled keeps its own refusal first", func(t *testing.T) {
		t.Parallel()

		tx, scopes := oneToOne()
		parts, err := decomposeCrossLedgerTransaction(tx, scopes)
		require.NoError(t, err)

		reader := f.reader(bridgeRouteCache(), f.ledgerA, f.ledgerB)
		reader.settingsByRef[f.ledgerB] = mmodel.LedgerSettings{}

		err = (&UseCase{TransactionReader: reader}).routeCrossLedgerBridgeLegs(context.Background(), tx, parts)

		var businessErr pkg.UnprocessableOperationError
		require.ErrorAs(t, err, &businessErr)
		assert.Equal(t, constant.ErrCrossLedgerNotEnabled.Error(), businessErr.Code)
	})
}

func TestCrossLedgerGroupIntent_BridgeRouteSurvivesPersistence(t *testing.T) {
	t.Parallel()

	f := newBridgeRouteFixture()
	tx := f.routedTransaction("100",
		[]mtransaction.FromTo{f.clientLeg("@debit", "100", true)},
		[]mtransaction.FromTo{f.clientLeg("@credit", "100", false)})
	parts, err := decomposeCrossLedgerTransaction(tx, crossLedgerTransactionScopes{
		from: []atomicTransactionBatchLedgerRef{f.ledgerA}, to: []atomicTransactionBatchLedgerRef{f.ledgerB},
	})
	require.NoError(t, err)
	require.NoError(t, (&UseCase{TransactionReader: f.reader(bridgeRouteCache(f.bridgeRouteID), f.ledgerA, f.ledgerB)}).
		routeCrossLedgerBridgeLegs(context.Background(), tx, parts))

	intent, err := buildCrossLedgerGroupIntent("BRL", parts)
	require.NoError(t, err)

	raw, err := encodeCrossLedgerGroupIntent(intent)
	require.NoError(t, err)

	decoded, err := decodeCrossLedgerGroupIntent(raw)
	require.NoError(t, err)
	assert.Equal(t, CrossLedgerGroupIntentFormatVersion, decoded.FormatVersion)
	require.Len(t, decoded.Parts, 2)
	assert.Equal(t, CrossLedgerGroupRoleOrigin, decoded.Parts[0].Role)
	assert.Equal(t, f.bridgeRouteID, routeOf(decoded.Parts[0].Transaction.Send.Distribute.To[0]))
	assert.Equal(t, CrossLedgerGroupRoleDestination, decoded.Parts[1].Role)
	assert.Equal(t, f.bridgeRouteID, routeOf(decoded.Parts[1].Transaction.Send.Source.From[0]))
	assert.Equal(t, f.clientRouteID, routeOf(decoded.Parts[1].Transaction.Send.Distribute.To[0]))
}

func TestCreateCrossLedgerTransactionV2_BridgeRoute(t *testing.T) {
	f := newBridgeRouteFixture()
	input := func() CreateCrossLedgerTransactionV2Input {
		return CreateCrossLedgerTransactionV2Input{
			Transaction: f.routedTransaction("100",
				[]mtransaction.FromTo{f.clientLeg("@debit", "100", true)},
				[]mtransaction.FromTo{f.clientLeg("@credit", "100", false)}),
			Scopes: CrossLedgerTransactionScopes{
				Debits:  []CrossLedgerLegScope{{OrganizationID: f.organizationID, LedgerID: f.ledgerA.ledgerID}},
				Credits: []CrossLedgerLegScope{{OrganizationID: f.organizationID, LedgerID: f.ledgerB.ledgerID}},
			},
		}
	}
	useCase := func(reader TransactionReader, executed *[]CreateAtomicTransactionBatchV2Input) *UseCase {
		return &UseCase{
			TransactionReader: reader,
			UUIDv7Generator:   func() (uuid.UUID, error) { return uuid.MustParse("0199a400-0000-7000-8000-000000000010"), nil },
			createAtomicTransactionBatchV2: func(_ context.Context, batch CreateAtomicTransactionBatchV2Input) (*CreateAtomicTransactionBatchV2Result, error) {
				*executed = append(*executed, batch)
				return &CreateAtomicTransactionBatchV2Result{BatchID: *batch.GroupID}, nil
			},
		}
	}

	t.Run("the executed parts carry the bridge route on their bridge legs", func(t *testing.T) {
		var executed []CreateAtomicTransactionBatchV2Input

		_, err := useCase(f.reader(bridgeRouteCache(f.bridgeRouteID), f.ledgerA, f.ledgerB), &executed).
			CreateCrossLedgerTransactionV2(context.Background(), input())
		require.NoError(t, err)

		require.Len(t, executed, 1)
		require.Len(t, executed[0].Transactions, 2)
		assert.Equal(t, f.bridgeRouteID, routeOf(executed[0].Transactions[0].Transaction.Send.Distribute.To[0]))
		assert.Equal(t, f.bridgeRouteID, routeOf(executed[0].Transactions[1].Transaction.Send.Source.From[0]))
	})

	t.Run("a validating part without a bridge route is refused before anything executes", func(t *testing.T) {
		var executed []CreateAtomicTransactionBatchV2Input

		result, err := useCase(f.reader(bridgeRouteCache(), f.ledgerB), &executed).
			CreateCrossLedgerTransactionV2(context.Background(), input())

		assert.Nil(t, result)
		assert.Empty(t, executed)

		var businessErr pkg.UnprocessableOperationError
		require.ErrorAs(t, err, &businessErr)
		assert.Equal(t, constant.ErrCrossLedgerRouteNotConfigured.Error(), businessErr.Code)
	})
}

func TestCreateCrossLedgerHoldV2_BridgeRoute(t *testing.T) {
	f := newBridgeRouteFixture()
	input := CreateCrossLedgerTransactionV2Input{
		Transaction: f.routedTransaction("10",
			[]mtransaction.FromTo{f.clientLeg("@debit", "10", true)},
			[]mtransaction.FromTo{f.clientLeg("@credit", "10", false)}),
		Scopes: CrossLedgerTransactionScopes{
			Debits:  []CrossLedgerLegScope{{OrganizationID: f.organizationID, LedgerID: f.ledgerA.ledgerID}},
			Credits: []CrossLedgerLegScope{{OrganizationID: f.organizationID, LedgerID: f.ledgerB.ledgerID}},
		},
	}
	useCase := func(t *testing.T, reader TransactionReader) *UseCase {
		// No repository expectation: neither refusal may persist an intent.
		return &UseCase{
			TransactionGroupRepo: transactiongroup.NewMockRepository(gomock.NewController(t)),
			TransactionReader:    reader,
			UUIDv7Generator:      func() (uuid.UUID, error) { return uuid.MustParse("0199a400-0000-7000-8000-000000000011"), nil },
			Clock:                func() time.Time { return time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC) },
		}
	}

	t.Run("a validating part without a bridge route is refused before the intent is persisted", func(t *testing.T) {
		_, err := useCase(t, f.reader(bridgeRouteCache(), f.ledgerA)).CreateCrossLedgerHoldV2(context.Background(), input)

		var businessErr pkg.UnprocessableOperationError
		require.ErrorAs(t, err, &businessErr)
		assert.Equal(t, constant.ErrCrossLedgerRouteNotConfigured.Error(), businessErr.Code)
	})

	t.Run("a configured bridge route persists the intent with the bridge leg routed", func(t *testing.T) {
		uc := useCase(t, f.reader(bridgeRouteCache(f.bridgeRouteID), f.ledgerA))
		persistenceFailed := errors.New("intent persistence stopped by the test")

		var persisted *CrossLedgerGroupIntent

		uc.TransactionGroupRepo.(*transactiongroup.MockRepository).EXPECT().Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, group *transactiongroup.TransactionGroup) error {
				intent, err := decodeCrossLedgerGroupIntent(group.Intent)
				require.NoError(t, err)

				persisted = intent

				return persistenceFailed
			})

		_, err := uc.CreateCrossLedgerHoldV2(context.Background(), input)

		require.ErrorIs(t, err, persistenceFailed)
		require.NotNil(t, persisted)
		require.Equal(t, CrossLedgerGroupRoleOrigin, persisted.Parts[0].Role)

		bridge := persisted.Parts[0].Transaction.Send.Distribute.To[0]
		require.NotNil(t, bridge.RouteID, "the validating origin's bridge leg carries the bridge route")
		assert.Equal(t, f.bridgeRouteID, *bridge.RouteID)
	})
}
