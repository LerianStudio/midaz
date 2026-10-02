// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	feesservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

const crossLedgerBridgeAlias = "@external/USD"

// withFees prices the fixture's /v2 transactions with the real fee use case over
// the fixture's MongoDB, and returns the package repository the scenarios seed.
//
// The fixture's accounts exist only as balance rows, so the fee resolver finds
// no account behind an alias: no segment and no segment waiver, which is how the
// fee engine treats an account it cannot resolve.
func (fixture *atomicBatchHTTPIntegrationFixture) withFees(t *testing.T) pack.Repository {
	t.Helper()

	accounts, ok := fixture.infra.handler.Query.AccountRepo.(*account.MockRepository)
	require.True(t, ok, "the fixture's account repository must be the absent-account mock")
	accounts.EXPECT().
		FindAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, services.ErrDatabaseItemNotFound).
		AnyTimes()

	packageRepo, err := pack.NewPackageMongoDBRepository(&feesmongo.MongoConnection{
		ConnectionStringSource: fixture.infra.mongoContainer.URI,
		Database:               "test_db",
		MaxPoolSize:            1,
		DB:                     fixture.infra.mongoContainer.Client,
	}, &libLog.GoLogger{})
	require.NoError(t, err)

	resolver, err := feesservices.NewQueryResolver(fixture.infra.handler.Query)
	require.NoError(t, err)

	feeUseCase, err := feesservices.NewUseCase(packageRepo, resolver)
	require.NoError(t, err)

	fixture.infra.handler.Command.FeeApplier = feeUseCase

	return packageRepo
}

// crossLedgerFeeLedgers are two cross-ledger enabled ledgers, A and B, of the
// fixture's organization. Each holds a funded client, an empty receiver, an
// empty fee account and an empty external bridge account.
type crossLedgerFeeLedgers struct {
	ledgerA, ledgerB     uuid.UUID
	clientA, clientB     string
	receiverA, receiverB string
	feeA, feeB           string
}

func (fixture *atomicBatchHTTPIntegrationFixture) crossLedgerFeeLedgers(t *testing.T, name string) crossLedgerFeeLedgers {
	t.Helper()

	ledgers := crossLedgerFeeLedgers{
		ledgerA:   fixture.newLedger(t),
		ledgerB:   fixture.newLedger(t),
		clientA:   "@" + name + "-client-a",
		clientB:   "@" + name + "-client-b",
		receiverA: "@" + name + "-receiver-a",
		receiverB: "@" + name + "-receiver-b",
		feeA:      "@" + name + "-fee-a",
		feeB:      "@" + name + "-fee-b",
	}

	fixture.setCrossLedgerEnabled(t, ledgers.ledgerA, true)
	fixture.setCrossLedgerEnabled(t, ledgers.ledgerB, true)

	for _, ledger := range []struct {
		id                    uuid.UUID
		client, receiver, fee string
	}{
		{ledgers.ledgerA, ledgers.clientA, ledgers.receiverA, ledgers.feeA},
		{ledgers.ledgerB, ledgers.clientB, ledgers.receiverB, ledgers.feeB},
	} {
		fixture.seedFeeBalance(t, ledger.id, ledger.client, "deposit", 100)
		fixture.seedFeeBalance(t, ledger.id, ledger.receiver, "deposit", 0)
		fixture.seedFeeBalance(t, ledger.id, ledger.fee, "deposit", 0)
		fixture.seedFeeBalance(t, ledger.id, crossLedgerBridgeAlias, "external", 0)
	}

	return ledgers
}

func (fixture *atomicBatchHTTPIntegrationFixture) seedFeeBalance(t *testing.T, ledgerID uuid.UUID, alias, accountType string, available int64) {
	t.Helper()

	params := postgrestestutil.DefaultBalanceParams()
	params.Alias = alias
	params.AssetCode = "USD"
	params.AccountType = accountType
	params.Available = decimal.NewFromInt(available)
	postgrestestutil.CreateTestBalance(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, uuid.New(), params)
}

func (fixture *atomicBatchHTTPIntegrationFixture) leg(ledgerID uuid.UUID, alias string, amount int64) TransactionV2LegRequest {
	return TransactionV2LegRequest{
		Alias:          alias,
		OrganizationID: fixture.infra.orgID.String(),
		LedgerID:       ledgerID.String(),
		Amount:         decimal.NewFromInt(amount).String(),
	}
}

// transferAToB moves 100 USD from A's client to B's receiver.
func (fixture *atomicBatchHTTPIntegrationFixture) transferAToB(ledgers crossLedgerFeeLedgers, description string) CreateTransactionV2Request {
	return CreateTransactionV2Request{
		Description: description,
		Asset:       "USD",
		Amount:      "100",
		Debits:      []TransactionV2LegRequest{fixture.leg(ledgers.ledgerA, ledgers.clientA, 100)},
		Credits:     []TransactionV2LegRequest{fixture.leg(ledgers.ledgerB, ledgers.receiverB, 100)},
	}
}

func (fixture *atomicBatchHTTPIntegrationFixture) postFeeGroup(t *testing.T, action string, request CreateTransactionV2Request, key string) CreateTransactionV2Response {
	t.Helper()

	raw, err := json.Marshal(request)
	require.NoError(t, err)

	response := postTransaction(t, fixture.app, v2CreateURL(action), string(raw), key)

	return decodeCrossLedgerGroup(t, response.StatusCode, drainBody(t, response), http.StatusCreated)
}

// requireBridges asserts the bridge balance of each ledger. A bridge never pays a
// fee, so A's bridge is credited exactly what B's bridge is debited.
func (fixture *atomicBatchHTTPIntegrationFixture) requireBridges(t *testing.T, ledgers crossLedgerFeeLedgers, crossed int64) {
	t.Helper()

	requireCachedAvailable(t, fixture, ledgers.ledgerA, crossLedgerBridgeAlias, crossed)
	requireCachedAvailable(t, fixture, ledgers.ledgerB, crossLedgerBridgeAlias, -crossed)
}

func memberMetadata(t *testing.T, result CreateTransactionV2Response, ledgerID uuid.UUID) map[string]any {
	t.Helper()

	for _, member := range result.Transactions {
		if member.LedgerID == ledgerID.String() {
			return member.Metadata
		}
	}

	require.Failf(t, "group member not found", "no member in ledger %s", ledgerID)

	return nil
}

// requireBridgeExemption asserts the part recorded the package and the
// cross_ledger_bridge skip, and no charge.
func requireBridgeExemption(t *testing.T, metadata map[string]any, packageID uuid.UUID) {
	t.Helper()

	raw, ok := metadata["feeExemption"].(string)
	require.Truef(t, ok, "feeExemption must be recorded; metadata: %v", metadata)

	var exemption struct {
		Exempt bool   `json:"exempt"`
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &exemption))
	require.True(t, exemption.Exempt)
	require.Equal(t, "cross_ledger_bridge", exemption.Reason)
	require.Equal(t, packageID.String(), metadata["packageAppliedID"])
	require.NotContains(t, metadata, "feeApplied")
}

// requireFeeAccountUntouched asserts no movement reached the fee account: it
// still holds its seeded zero and, never having moved, has no live balance.
func (fixture *atomicBatchHTTPIntegrationFixture) requireFeeAccountUntouched(t *testing.T, ledgerID uuid.UUID, alias string) {
	t.Helper()

	cached := getBalanceFromRedis(t, context.Background(), fixture.infra.redisRepo, fixture.infra.orgID, ledgerID, alias, "default")
	if cached != nil {
		require.Truef(t, cached.Available.IsZero(), "fee account %s received %s", alias, cached.Available.String())
	}

	var available decimal.Decimal
	require.NoError(t, fixture.infra.pgContainer.DB.QueryRow(
		`SELECT available FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND alias = $3 AND key = 'default'`,
		fixture.infra.orgID, ledgerID, alias,
	).Scan(&available))
	requireDecimalEqual(t, decimal.Zero, available, "fee account %s", alias)
}

// requireLegTotal asserts the summed amount of the transaction's operations of
// one type on alias; zero asserts there is none.
func requireLegTotal(t *testing.T, legs []persistedLeg, alias, legType string, want int64) {
	t.Helper()

	requireDecimalEqual(t, decimal.NewFromInt(want), sumAmounts(legsFor(legs, alias, legType)), "%s %s", legType, alias)
}

func TestCrossLedgerFee_BridgeNeverPaysAFee(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	packages := fixture.withFees(t)
	db := fixture.infra.pgContainer.DB
	organization := fixture.infra.orgID

	t.Run("destination ledger non-deductible fee is skipped instead of charging the bridge", func(t *testing.T) {
		ledgers := fixture.crossLedgerFeeLedgers(t, "dest-nd")
		packageID := seedFeePackage(t, packages, organization, ledgers.ledgerB, packageSpec{
			label: "destination non-deductible",
			fees:  []feeSpec{flatFee("inbound", ledgers.feeB, "2", false)},
		})

		result := fixture.postFeeGroup(t, "direct", fixture.transferAToB(ledgers, "inbound non-deductible"), "dest-nd")
		require.Len(t, result.Transactions, 2)

		partB := loadLegs(t, db, groupMember(t, result, ledgers.ledgerB))
		requireLegTotal(t, partB, crossLedgerBridgeAlias, constant.DEBIT, 100)
		requireLegTotal(t, partB, ledgers.feeB, constant.CREDIT, 0)
		requireBalanced(t, partB, "destination part")
		requireBridgeExemption(t, memberMetadata(t, result, ledgers.ledgerB), packageID)

		fixture.requireBridges(t, ledgers, 100)
		requireCachedAvailable(t, fixture, ledgers.ledgerA, ledgers.clientA, 0)
		requireCachedAvailable(t, fixture, ledgers.ledgerB, ledgers.receiverB, 100)
		fixture.requireFeeAccountUntouched(t, ledgers.ledgerB, ledgers.feeB)
	})

	t.Run("destination ledger deductible fee is still deducted from the receiver", func(t *testing.T) {
		ledgers := fixture.crossLedgerFeeLedgers(t, "dest-d")
		packageID := seedFeePackage(t, packages, organization, ledgers.ledgerB, packageSpec{
			label: "destination deductible",
			fees:  []feeSpec{flatFee("inbound", ledgers.feeB, "2", true)},
		})

		result := fixture.postFeeGroup(t, "direct", fixture.transferAToB(ledgers, "inbound deductible"), "dest-d")

		partB := loadLegs(t, db, groupMember(t, result, ledgers.ledgerB))
		requireLegTotal(t, partB, crossLedgerBridgeAlias, constant.DEBIT, 100)
		requireLegTotal(t, partB, ledgers.receiverB, constant.CREDIT, 98)
		requireLegTotal(t, partB, ledgers.feeB, constant.CREDIT, 2)
		requireBalanced(t, partB, "destination part")

		metadata := memberMetadata(t, result, ledgers.ledgerB)
		require.Equal(t, "true", metadata["feeApplied"])
		require.Equal(t, packageID.String(), metadata["packageAppliedID"])
		require.NotContains(t, metadata, "feeExemption")

		fixture.requireBridges(t, ledgers, 100)
		requireCachedAvailable(t, fixture, ledgers.ledgerB, ledgers.receiverB, 98)
		requireCachedAvailable(t, fixture, ledgers.ledgerB, ledgers.feeB, 2)
	})

	t.Run("origin ledger deductible fee is skipped instead of deducting from the bridge", func(t *testing.T) {
		ledgers := fixture.crossLedgerFeeLedgers(t, "orig-d")
		packageID := seedFeePackage(t, packages, organization, ledgers.ledgerA, packageSpec{
			label: "origin deductible",
			fees:  []feeSpec{flatFee("outbound", ledgers.feeA, "2", true)},
		})

		result := fixture.postFeeGroup(t, "direct", fixture.transferAToB(ledgers, "outbound deductible"), "orig-d")

		partA := loadLegs(t, db, groupMember(t, result, ledgers.ledgerA))
		requireLegTotal(t, partA, ledgers.clientA, constant.DEBIT, 100)
		requireLegTotal(t, partA, crossLedgerBridgeAlias, constant.CREDIT, 100)
		requireLegTotal(t, partA, ledgers.feeA, constant.CREDIT, 0)
		requireBalanced(t, partA, "origin part")
		requireBridgeExemption(t, memberMetadata(t, result, ledgers.ledgerA), packageID)

		fixture.requireBridges(t, ledgers, 100)
		fixture.requireFeeAccountUntouched(t, ledgers.ledgerA, ledgers.feeA)
		requireCachedAvailable(t, fixture, ledgers.ledgerB, ledgers.receiverB, 100)
	})

	t.Run("net receiving ledger charges its non-deductible fee only to its client source", func(t *testing.T) {
		ledgers := fixture.crossLedgerFeeLedgers(t, "net-recv")
		seedFeePackage(t, packages, organization, ledgers.ledgerB, packageSpec{
			label: "net receiving non-deductible",
			fees:  []feeSpec{flatFee("mixed", ledgers.feeB, "2", false)},
		})

		// A sends 100 and receives 30; B sends 30 and receives 100. The bridge
		// carries the 70 that crosses from A to B.
		request := CreateTransactionV2Request{
			Description: "net receiving mixed part",
			Asset:       "USD",
			Amount:      "130",
			Debits: []TransactionV2LegRequest{
				fixture.leg(ledgers.ledgerA, ledgers.clientA, 100),
				fixture.leg(ledgers.ledgerB, ledgers.clientB, 30),
			},
			Credits: []TransactionV2LegRequest{
				fixture.leg(ledgers.ledgerB, ledgers.receiverB, 100),
				fixture.leg(ledgers.ledgerA, ledgers.receiverA, 30),
			},
		}
		result := fixture.postFeeGroup(t, "direct", request, "net-recv")

		partB := loadLegs(t, db, groupMember(t, result, ledgers.ledgerB))
		requireLegTotal(t, partB, crossLedgerBridgeAlias, constant.DEBIT, 70)
		requireLegTotal(t, partB, ledgers.clientB, constant.DEBIT, 32)
		requireLegTotal(t, partB, ledgers.feeB, constant.CREDIT, 2)
		requireBalanced(t, partB, "net receiving part")
		require.Equal(t, "true", memberMetadata(t, result, ledgers.ledgerB)["feeApplied"])

		fixture.requireBridges(t, ledgers, 70)
		requireCachedAvailable(t, fixture, ledgers.ledgerA, ledgers.clientA, 0)
		requireCachedAvailable(t, fixture, ledgers.ledgerA, ledgers.receiverA, 30)
		requireCachedAvailable(t, fixture, ledgers.ledgerB, ledgers.clientB, 68)
		requireCachedAvailable(t, fixture, ledgers.ledgerB, ledgers.receiverB, 100)
		requireCachedAvailable(t, fixture, ledgers.ledgerB, ledgers.feeB, 2)
	})

	t.Run("hold and commit never charge either bridge", func(t *testing.T) {
		ledgers := fixture.crossLedgerFeeLedgers(t, "hold-commit")
		originPackage := seedFeePackage(t, packages, organization, ledgers.ledgerA, packageSpec{
			label: "origin deductible",
			fees:  []feeSpec{flatFee("outbound", ledgers.feeA, "2", true)},
		})
		destinationPackage := seedFeePackage(t, packages, organization, ledgers.ledgerB, packageSpec{
			label: "destination non-deductible",
			fees:  []feeSpec{flatFee("inbound", ledgers.feeB, "2", false)},
		})

		held := fixture.postFeeGroup(t, "hold", fixture.transferAToB(ledgers, "held cross-ledger transfer"), "hold-commit")
		require.Len(t, held.Transactions, 1)
		require.Equal(t, constant.PENDING, held.Transactions[0].Status.Code)
		requireBridgeExemption(t, memberMetadata(t, held, ledgers.ledgerA), originPackage)
		requireCachedOnHold(t, fixture, ledgers.ledgerA, ledgers.clientA, 100)
		fixture.requireFeeAccountUntouched(t, ledgers.ledgerA, ledgers.feeA)

		originID := uuid.MustParse(held.Transactions[0].ID)
		response := postTransaction(t, fixture.app, v2CommitURL(organization, ledgers.ledgerA, originID), "", "")
		committed := decodeCrossLedgerGroup(t, response.StatusCode, drainBody(t, response), http.StatusCreated)
		require.Len(t, committed.Transactions, 2)
		requireBridgeExemption(t, memberMetadata(t, committed, ledgers.ledgerB), destinationPackage)

		partA := loadLegs(t, db, originID)
		requireLegTotal(t, partA, crossLedgerBridgeAlias, constant.CREDIT, 100)
		requireLegTotal(t, partA, ledgers.feeA, constant.CREDIT, 0)

		partB := loadLegs(t, db, groupMember(t, committed, ledgers.ledgerB))
		requireLegTotal(t, partB, crossLedgerBridgeAlias, constant.DEBIT, 100)
		requireLegTotal(t, partB, ledgers.feeB, constant.CREDIT, 0)
		requireBalanced(t, partB, "committed destination part")

		fixture.requireBridges(t, ledgers, 100)
		requireCachedOnHold(t, fixture, ledgers.ledgerA, ledgers.clientA, 0)
		requireCachedAvailable(t, fixture, ledgers.ledgerA, ledgers.clientA, 0)
		fixture.requireFeeAccountUntouched(t, ledgers.ledgerA, ledgers.feeA)
		requireCachedAvailable(t, fixture, ledgers.ledgerB, ledgers.receiverB, 100)
		fixture.requireFeeAccountUntouched(t, ledgers.ledgerB, ledgers.feeB)
	})

	t.Run("single-ledger transfer from the external account still pays its fee", func(t *testing.T) {
		ledgerID := fixture.newLedger(t)
		fixture.seedFeeBalance(t, ledgerID, crossLedgerBridgeAlias, "external", 0)
		fixture.seedFeeBalance(t, ledgerID, "@single-receiver", "deposit", 0)
		fixture.seedFeeBalance(t, ledgerID, "@single-fee", "deposit", 0)
		seedFeePackage(t, packages, organization, ledgerID, packageSpec{
			label: "single-ledger non-deductible",
			fees:  []feeSpec{flatFee("deposit", "@single-fee", "2", false)},
		})

		raw, err := json.Marshal(CreateTransactionV2Request{
			Description: "external deposit",
			Asset:       "USD",
			Amount:      "100",
			Debits:      []TransactionV2LegRequest{fixture.leg(ledgerID, crossLedgerBridgeAlias, 100)},
			Credits:     []TransactionV2LegRequest{fixture.leg(ledgerID, "@single-receiver", 100)},
		})
		require.NoError(t, err)

		response := postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), "single-external")
		body := drainBody(t, response)
		require.Equal(t, http.StatusCreated, response.StatusCode, "body: %s", string(body))

		var created CreateTransactionV2Response
		require.NoError(t, json.Unmarshal(body, &created))
		require.Nil(t, created.GroupID)
		require.Equal(t, "true", created.Metadata["feeApplied"])

		legs := loadLegs(t, db, uuid.MustParse(created.ID))
		requireLegTotal(t, legs, crossLedgerBridgeAlias, constant.DEBIT, 102)
		requireLegTotal(t, legs, "@single-fee", constant.CREDIT, 2)
		requireBalanced(t, legs, "single-ledger external deposit")

		requireCachedAvailable(t, fixture, ledgerID, crossLedgerBridgeAlias, -102)
		requireCachedAvailable(t, fixture, ledgerID, "@single-receiver", 100)
		requireCachedAvailable(t, fixture, ledgerID, "@single-fee", 2)
	})
}
