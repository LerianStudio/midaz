// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	feesservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
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
	resolver, err := feesservices.NewQueryResolver(h.queryUC)
	require.NoError(t, err)
	h.feeUC, err = feesservices.NewUseCase(deferrablePackages{h.packageRepo}, resolver)
	require.NoError(t, err)
	h.commandUC.FeeApplier = h.feeUC

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

	fee := flatFee("deferrable_fee", "@debt-fee", "50", false)
	fee.routeFrom, fee.routeTo = routeString(s.from), routeString(s.to)
	h.seedPackage(t, packageSpec{label: "fee_debt_routes", minAmount: decimal.NewFromInt(50), fees: []feeSpec{fee}})

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

// TestFeeDebtSettlesInsideAnAtomicBatch credits the indebted payer from an atomic batch
// item: the collect's creditor is no leg of the item, and the batch still settles the debt.
func TestFeeDebtSettlesInsideAnAtomicBatch(t *testing.T) {
	s := newFeeDebtRoutes(t, false)
	s.enableAtomicBatches(t)

	s.open(t)
	credit := s.v2RoutedBody("fee debt batch credit", "BRL", "10", s.credit,
		[]string{s.v2RoutedLeg("@debt-funder", "10", s.funder)}, []string{s.v2RoutedLeg("@debt-payer", "10", s.payer)})
	created := s.post(t, s.app, s.v2CreatePath("batch"), `{"transactions":[{"action":"direct","order":1,`+strings.TrimPrefix(credit, "{")+`]}`,
		map[string]string{"X-Idempotency": uuid.NewString()})
	require.Equalf(t, 201, created.status, "batch: %s", string(created.rawBody))
	s.balances(t, "0", "80", "30", "990")
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
