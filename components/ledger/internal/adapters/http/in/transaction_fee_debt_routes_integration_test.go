// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/fee_debt"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	feemodel "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// feeDebtRoutes is a harness whose 50 fee, owed by @debt-payer to @debt-fee, is deferrable
// and routed fee-from/fee-to; @debt-funder credits the payer under its own routes.
type feeDebtRoutes struct {
	*feeHarness
	app                                *fiber.App
	origin, credit                     uuid.UUID
	payer, receiver, funder, from, to  uuid.UUID
	originRoute, creditRoute, feeRoute uuid.UUID
}

func newFeeDebtRoutes(t *testing.T, validated bool) *feeDebtRoutes {
	t.Helper()

	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)
	h.queryUC.EngineWriteBehindCodec = command.EngineWriteBehindEvidenceCodec{}

	debts, err := fee_debt.NewRepository(&feesmongo.MongoConnection{Database: "test_db", DB: h.mongoContainer.Client}, nil)
	require.NoError(t, err)

	h.commandUC.FeeDebts = debts
	h.commandUC.AppliedTransactionCompleter = command.NewTransactionCompletionService(h.completionStore, h.metaRepo).WithFeeDebtRecorder(debts)

	s := &feeDebtRoutes{feeHarness: h, app: h.newV2App()}
	s.origin = postgrestestutil.CreateTestTransactionRouteSimple(t, h.db, h.orgID, h.ledgerID, "fee origin")
	s.credit = postgrestestutil.CreateTestTransactionRouteSimple(t, h.db, h.orgID, h.ledgerID, "fee debt credit")
	s.payer, s.receiver = h.seedBidirectionalRoute(t, "payer", s.origin), h.seedBidirectionalRoute(t, "receiver", s.origin)
	s.from, s.to = h.seedBidirectionalRoute(t, "fee from", s.origin), h.seedBidirectionalRoute(t, "fee to", s.origin)
	s.funder = h.seedBidirectionalRoute(t, "funder", s.credit)
	postgrestestutil.CreateTestOperationTransactionRouteLink(t, h.db, s.payer, s.credit)

	if validated {
		postgrestestutil.SetLedgerSettings(t, h.db, h.ledgerID, map[string]any{"accounting": map[string]any{"validateRoutes": true}})
	}

	h.seedBalance(t, "@debt-payer", "BRL", decimal.NewFromInt(100), "deposit")
	h.seedBalance(t, "@debt-receiver", "BRL", decimal.Zero, "deposit")
	h.seedBalance(t, "@debt-fee", "BRL", decimal.Zero, "deposit")
	h.seedBalance(t, "@debt-funder", "BRL", decimal.NewFromInt(1000), "deposit")

	// The package goes through the create body and service, so the flag reaches the engine
	// only through the package's own JSON, Mongo document and read path.
	var input feemodel.CreatePackageInput
	require.NoError(t, json.Unmarshal(fmt.Appendf(nil, `{"feeGroupLabel":"fee_debt_routes","minimumAmount":"50","maximumAmount":"1000000000","enable":true,
		"fees":{"deferrable_fee":{"feeLabel":"deferrable_fee","priority":1,"referenceAmount":"originalAmount","isDeductibleFrom":false,"deferrable":true,
		"creditAccount":"@debt-fee","routeFrom":%q,"routeTo":%q,"calculationModel":{"applicationRule":"flatFee","calculations":[{"type":"flat","value":"50"}]}}}}`,
		s.from, s.to), &input))
	require.NoError(t, input.ValidateFees())
	_, err = h.feeUC.CreatePackage(h.ctx(), &input, h.orgID, h.ledgerID, uuid.Nil)
	require.NoError(t, err)

	return s
}

// open posts the 80 transfer whose 50 fee the payer (100) pays 20 of, owing 30.
func (s *feeDebtRoutes) open(t *testing.T) uuid.UUID {
	t.Helper()

	return s.transfer(t, s.origin, "@debt-payer", s.payer, "@debt-receiver", s.receiver, "80")
}

// settle posts a 10 credit to the payer, which settles 10 of its debt.
func (s *feeDebtRoutes) settle(t *testing.T) uuid.UUID {
	t.Helper()

	return s.transfer(t, s.credit, "@debt-funder", s.funder, "@debt-payer", s.payer, "10")
}

func (s *feeDebtRoutes) transfer(t *testing.T, route uuid.UUID, from string, fromRoute uuid.UUID, to string, toRoute uuid.UUID, amount string) uuid.UUID {
	t.Helper()

	body := s.v2RoutedBody("fee debt routes", "BRL", amount, route,
		[]string{s.v2RoutedLeg(from, amount, fromRoute)}, []string{s.v2RoutedLeg(to, amount, toRoute)})
	created := s.createV2Direct(t, s.app, body, map[string]string{"X-Idempotency": uuid.NewString()})
	require.Equalf(t, 201, created.status, "create: %s", string(created.rawBody))

	return mustTxID(t, created)
}

func (s *feeDebtRoutes) revert(t *testing.T, parent uuid.UUID) uuid.UUID {
	t.Helper()

	reverted := s.post(t, s.app, s.v2StatePath(parent, "revert"), "", nil)
	require.Equalf(t, 201, reverted.status, "revert: %s", string(reverted.rawBody))

	return mustTxID(t, reverted)
}

// balances asserts the payer, receiver, fee account and funder, in that order.
func (s *feeDebtRoutes) balances(t *testing.T, want ...string) {
	t.Helper()

	for i, alias := range []string{"@debt-payer", "@debt-receiver", "@debt-fee", "@debt-funder"} {
		assertLiveBalance(t, s.feeHarness, alias, "default", want[i])
	}
}

// rows reads a transaction's operation rows as "TYPE direction alias amount routeId code".
func (s *feeDebtRoutes) rows(t *testing.T, txID uuid.UUID) []string {
	t.Helper()

	result, err := s.db.Query(`SELECT type, direction, account_alias, amount, route_id, COALESCE(route_code, '') FROM operation WHERE transaction_id = $1`, txID)
	require.NoError(t, err)

	defer func() { _ = result.Close() }()

	var views []string

	for result.Next() {
		var (
			kind, direction, alias, code string
			amount                       decimal.Decimal
			route                        *string
		)

		require.NoError(t, result.Scan(&kind, &direction, &alias, &amount, &route, &code))
		require.NotNil(t, route, "%s %s %s has no route", kind, direction, alias)
		views = append(views, kind+" "+direction+" "+alias+" "+amount.String()+" "+*route+" "+code)
	}

	require.NoError(t, result.Err())

	return views
}

// row is the view of an expected row booked to code, empty on a ledger that resolves
// no rubrics.
func row(kind, direction, alias, amount string, route uuid.UUID, code string) string {
	return kind + " " + direction + " " + alias + " " + amount + " " + route.String() + " " + code
}

// rubric is the code the fixture's entries give route for the side of direction.
func rubric(route uuid.UUID, direction string) string {
	return route.String() + "-" + map[string]string{"debit": "D", "credit": "C"}[direction]
}

// TestFeeDebtMovementsCarryTheFeeRoutes opens a 30 debt, settles 10, reverts the settling
// credit, settles 10 again and reverts the origin, refunding the 10 paid, on a ledger that
// does not validate routes: every fee-debt row, the take-back included, books under the
// fee's own routes, and every step leaves the balances it must.
func TestFeeDebtMovementsCarryTheFeeRoutes(t *testing.T) {
	s := newFeeDebtRoutes(t, false)

	origin := s.open(t)
	assert.ElementsMatch(t, []string{
		row("DEBIT", "debit", "@debt-payer", "80", s.payer, ""), row("DEBIT", "debit", "@debt-payer", "20", s.from, ""),
		row("CREDIT", "credit", "@debt-receiver", "80", s.receiver, ""), row("CREDIT", "credit", "@debt-fee", "20", s.to, ""),
	}, s.rows(t, origin), "the payer pays 20 of the 50 fee and owes 30")
	s.balances(t, "0", "80", "20", "1000")

	settlement := []string{
		row("DEBIT", "debit", "@debt-funder", "10", s.funder, ""), row("CREDIT", "credit", "@debt-payer", "10", s.payer, ""),
		row("FEE_SETTLEMENT", "debit", "@debt-payer", "10", s.from, ""), row("FEE_SETTLEMENT", "credit", "@debt-fee", "10", s.to, ""),
	}
	settling := s.settle(t)
	assert.ElementsMatch(t, settlement, s.rows(t, settling), "the credit settles 10 under the fee's routes")
	s.balances(t, "0", "80", "30", "990")

	assert.ElementsMatch(t, []string{
		row("DEBIT", "debit", "@debt-fee", "10", s.to, ""), row("CREDIT", "credit", "@debt-funder", "10", s.funder, ""),
	}, s.rows(t, s.revert(t, settling)), "the take-back books under the fee's credit route")
	s.balances(t, "0", "80", "20", "1000")

	assert.ElementsMatch(t, settlement, s.rows(t, s.settle(t)), "the reopened debt settles again")
	s.balances(t, "0", "80", "30", "990")

	assert.ElementsMatch(t, []string{
		row("DEBIT", "debit", "@debt-receiver", "80", s.receiver, ""), row("DEBIT", "debit", "@debt-fee", "20", s.to, ""),
		row("CREDIT", "credit", "@debt-payer", "80", s.payer, ""), row("CREDIT", "credit", "@debt-payer", "20", s.from, ""),
		row("FEE_REFUND", "credit", "@debt-payer", "10", s.from, ""), row("FEE_REFUND", "debit", "@debt-fee", "10", s.to, ""),
	}, s.rows(t, s.revert(t, origin)), "the origin's revert refunds the 10 paid under the fee's routes")
	s.balances(t, "110", "0", "0", "990")
}

// TestFeeDebtMovementsCarryTheFeeRubrics runs on a route-validating ledger, where each
// fee-debt row also carries the rubric of the fee's route: the direct rubric stored at
// opening on a settlement, the revert rubric on a refund.
func TestFeeDebtMovementsCarryTheFeeRubrics(t *testing.T) {
	s := newFeeDebtRoutes(t, true)

	origin := s.open(t)
	assert.ElementsMatch(t, []string{
		row("DEBIT", "debit", "@debt-payer", "80", s.payer, rubric(s.payer, "debit")), row("DEBIT", "debit", "@debt-payer", "20", s.from, rubric(s.from, "debit")),
		row("CREDIT", "credit", "@debt-receiver", "80", s.receiver, rubric(s.receiver, "credit")), row("CREDIT", "credit", "@debt-fee", "20", s.to, rubric(s.to, "credit")),
	}, s.rows(t, origin))

	assert.ElementsMatch(t, []string{
		row("DEBIT", "debit", "@debt-funder", "10", s.funder, rubric(s.funder, "debit")), row("CREDIT", "credit", "@debt-payer", "10", s.payer, rubric(s.payer, "credit")),
		row("FEE_SETTLEMENT", "debit", "@debt-payer", "10", s.from, rubric(s.from, "debit")), row("FEE_SETTLEMENT", "credit", "@debt-fee", "10", s.to, rubric(s.to, "credit")),
	}, s.rows(t, s.settle(t)))
	s.balances(t, "0", "80", "30", "990")

	assert.ElementsMatch(t, []string{
		row("DEBIT", "debit", "@debt-receiver", "80", s.receiver, rubric(s.receiver, "debit")), row("DEBIT", "debit", "@debt-fee", "20", s.to, rubric(s.to, "debit")),
		row("CREDIT", "credit", "@debt-payer", "80", s.payer, rubric(s.payer, "credit")), row("CREDIT", "credit", "@debt-payer", "20", s.from, rubric(s.from, "credit")),
		row("FEE_REFUND", "credit", "@debt-payer", "10", s.from, rubric(s.from, "credit")), row("FEE_REFUND", "debit", "@debt-fee", "10", s.to, rubric(s.to, "debit")),
	}, s.rows(t, s.revert(t, origin)))
	s.balances(t, "110", "0", "0", "990")
}

// TestFeeDebtSettlementRevertsOnARouteValidatingLedger reverts a credit that settled a
// debt on a ledger that validates routes: the take-back, whose route belongs to the fee
// and not to the credit, is not held to the credit's transaction route and books to the
// debit rubric of that route's revert entry, and the debt reopens to settle again.
func TestFeeDebtSettlementRevertsOnARouteValidatingLedger(t *testing.T) {
	s := newFeeDebtRoutes(t, true)

	_, err := s.db.Exec(`UPDATE operation_route SET accounting_entries = jsonb_set(accounting_entries, '{revert}', $1::jsonb) WHERE id = $2`,
		fmt.Sprintf(`{"debit":{"code":"%[1]s-RD","description":"fee to revert"},"credit":{"code":"%[1]s-RC","description":"fee to revert"}}`, s.to), s.to)
	require.NoError(t, err, "give the fee's credit route a revert entry of its own")

	s.open(t)
	settling := s.settle(t)
	s.balances(t, "0", "80", "30", "990")

	assert.ElementsMatch(t, []string{
		row("DEBIT", "debit", "@debt-fee", "10", s.to, s.to.String()+"-RD"), row("CREDIT", "credit", "@debt-funder", "10", s.funder, rubric(s.funder, "credit")),
	}, s.rows(t, s.revert(t, settling)), "the take-back books under the fee's credit route and its revert debit rubric")
	s.balances(t, "0", "80", "20", "1000")

	s.settle(t)
	s.balances(t, "0", "80", "30", "990")
}

// TestFeeDebtSettlementGroupRevertsOnARouteValidatingLedger reverts a cross-ledger
// group whose credit settled a debt on a route-validating ledger: the take-back is held
// to none of the group's routes, the group reverts, and the debt reopens to settle again.
func TestFeeDebtSettlementGroupRevertsOnARouteValidatingLedger(t *testing.T) {
	s := newFeeDebtRoutes(t, true)
	s.enableAtomicBatches(t)
	s.commandUC.TransactionGroupRepo = transactiongroup.NewTransactionGroupPostgreSQLRepository(s.pgConn)

	remote := s.withSecondLedger(t)
	for _, ledgerID := range []uuid.UUID{s.ledgerID, remote.ledgerID} {
		postgrestestutil.SetLedgerSettings(t, s.db, ledgerID, map[string]any{
			"crossLedger": map[string]any{"enabled": true}, "accounting": map[string]any{"validateRoutes": true},
		})
	}

	remote.seedBalance(t, "@debt-remote", "BRL", decimal.NewFromInt(1000), "deposit")
	remote.seedBalance(t, "@external/BRL", "BRL", decimal.Zero, "external")
	s.seedBalance(t, "@external/BRL", "BRL", decimal.Zero, "external")

	group := postgrestestutil.CreateTestTransactionRouteSimple(t, s.db, s.orgID, s.ledgerID, "fee debt group credit")
	source := remote.seedBidirectionalRoute(t, "remote", group)
	postgrestestutil.CreateTestOperationTransactionRouteLink(t, s.db, s.payer, group)
	bridge := postgrestestutil.CreateTestOperationRouteSimple(t, s.db, s.orgID, s.ledgerID, "bridge", "bidirectional")
	_, err := s.db.Exec(`UPDATE operation_route SET accounting_entries=$1::jsonb WHERE id=$2`,
		`{"crossLedger":{"debit":{"code":"X-D","description":"in"},"credit":{"code":"X-C","description":"out"}}}`, bridge)
	require.NoError(t, err, "give the bridge route its crossLedger entry")
	postgrestestutil.CreateTestOperationTransactionRouteLink(t, s.db, bridge, group)

	s.open(t)
	credited := s.createV2Direct(t, s.app, s.v2RoutedBody("fee debt group credit", "BRL", "10", group,
		[]string{remote.v2RoutedLeg("@debt-remote", "10", source)}, []string{s.v2RoutedLeg("@debt-payer", "10", s.payer)}),
		map[string]string{"X-Idempotency": uuid.NewString()})
	settled := decodeCrossLedgerGroup(t, credited.status, credited.rawBody, 201)
	s.balances(t, "0", "80", "30", "1000")

	reverted := s.post(t, s.app, s.v2StatePath(groupMember(t, settled, s.ledgerID), "revert"), "", nil)
	decodeCrossLedgerGroup(t, reverted.status, reverted.rawBody, 201)
	s.balances(t, "0", "80", "20", "1000")
	assertLiveBalance(t, remote, "@debt-remote", "default", "1000")

	s.settle(t)
	s.balances(t, "0", "80", "30", "990")
}

// enableAtomicBatches wires what the atomic batch create reads beyond a singular create.
func (s *feeDebtRoutes) enableAtomicBatches(t *testing.T) {
	t.Helper()

	batches, ok := s.redisRepo.(command.AtomicTransactionBatchIdempotencyRepository)
	require.True(t, ok, "Redis repository must expose the atomic batch state machine")

	s.commandUC.AtomicTransactionBatchIdempotencyRepo = batches
	s.commandUC.AtomicTransactionBatchProjectionReader = s.queryUC
	s.commandUC.UUIDv7Generator, s.commandUC.Clock = libCommons.GenerateUUIDv7, time.Now
	s.handler.TransactionBatchMaxSize = 50
}

// TestFeeDebtSettlesInsideAnAtomicBatch credits two debtors who owe the same fee account
// from two items of one batch: the shared creditor is no leg of either item, and each
// item collects only its own debtor's debt.
func TestFeeDebtSettlesInsideAnAtomicBatch(t *testing.T) {
	s := newFeeDebtRoutes(t, false)
	s.enableAtomicBatches(t)
	s.seedBalance(t, "@debt-p1", "BRL", decimal.NewFromInt(100), "deposit")
	s.seedBalance(t, "@debt-p2", "BRL", decimal.NewFromInt(100), "deposit")

	// Each 50 fee finds 40 and 45 left after its transfer: p1 owes 10 and p2 owes 5.
	s.transfer(t, s.origin, "@debt-p1", s.payer, "@debt-receiver", s.receiver, "60")
	s.transfer(t, s.origin, "@debt-p2", s.payer, "@debt-receiver", s.receiver, "55")
	assertLiveBalance(t, s.feeHarness, "@debt-fee", "default", "85")

	credit := func(order, debtor, amount string) string {
		body := s.v2RoutedBody("fee debt batch credit", "BRL", amount, s.credit,
			[]string{s.v2RoutedLeg("@debt-funder", amount, s.funder)}, []string{s.v2RoutedLeg(debtor, amount, s.payer)})

		return `{"action":"direct","order":` + order + `,` + strings.TrimPrefix(body, "{")
	}
	created := s.post(t, s.app, s.v2CreatePath("batch"), `{"transactions":[`+credit("1", "@debt-p1", "8")+`,`+credit("2", "@debt-p2", "7")+`]}`,
		map[string]string{"X-Idempotency": uuid.NewString()})
	require.Equalf(t, 201, created.status, "batch: %s", string(created.rawBody))

	for alias, want := range map[string]string{"@debt-p1": "0", "@debt-p2": "2", "@debt-fee": "98", "@debt-funder": "985"} {
		assertLiveBalance(t, s.feeHarness, alias, "default", want)
	}

	for debtor, want := range map[string]string{"@debt-p1#default": "2", "@debt-p2#default": "0"} {
		open, err := s.commandUC.FeeDebts.(*fee_debt.Repository).OpenTotal(s.ctx(), s.orgID, s.ledgerID, debtor)
		require.NoError(t, err)
		assert.Truef(t, decimal.RequireFromString(want).Equal(open), "%s still owes %s, want %s", debtor, open, want)
	}
}

// laggingFeeDebts answers the fee-debt record as it stood before any settlement.
type laggingFeeDebts struct{ command.FeeDebtRecorder }

func (laggingFeeDebts) Settled(context.Context, uuid.UUID, uuid.UUID, []string) (map[string]decimal.Decimal, error) {
	return nil, nil
}

// TestFeeDebtRevertWaitsForTheRecord reverts an origin whose debt's record lags a
// settlement: the client is told to retry, nothing moves, and the retry refunds once
// the record catches up.
func TestFeeDebtRevertWaitsForTheRecord(t *testing.T) {
	s := newFeeDebtRoutes(t, false)

	origin := s.open(t)
	s.settle(t)

	debts := s.commandUC.FeeDebts
	s.commandUC.FeeDebts = laggingFeeDebts{debts}

	refused := s.post(t, s.app, s.v2StatePath(origin, "revert"), "", nil)
	assert.Equal(t, 409, refused.status, "body: %s", string(refused.rawBody))
	assert.Equal(t, "0529", refused.body["code"])
	s.balances(t, "0", "80", "30", "990")

	s.commandUC.FeeDebts = debts
	s.revert(t, origin)
	s.balances(t, "110", "0", "0", "990")
}
