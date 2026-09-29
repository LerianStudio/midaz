// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/fee_debt"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	feesservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// feeDebtPropertyFees are the flat deferrable fees of the two packages, each paid to
// its own fee account, so oldest-first settlement crosses fee accounts.
var feeDebtPropertyFees = [2]int64{30, 45}

const (
	feeDebtPropertyOps        = 100
	feeDebtPropertyPayerStart = 100
)

const (
	opOriginate    = "originate"
	opCredit       = "credit"
	opRevertOrigin = "revert-origin"
	opRevertCredit = "revert-credit"
	opRevertAgain  = "revert-again"
)

// feeDebtShrunk are minimal sequences a seed once failed on, replayed before the seeds.
var feeDebtShrunk = map[string][]feeDebtOp{
	// An origin's revert refunds 20 settled into it while an older reopened debt still
	// owes 35: the refund must settle 20 of it, as every credit to the payer does.
	"refund-settles-an-older-debt": {
		{kind: opOriginate, payer: 0, pkg: 1, amount: 100},
		{kind: opCredit, payer: 0, amount: 45},
		{kind: opCredit, payer: 0, amount: 10},
		{kind: opOriginate, payer: 0, pkg: 0, amount: 10},
		{kind: opCredit, payer: 0, amount: 20},
		{kind: opRevertCredit, target: 0},
		{kind: opRevertOrigin, target: 1},
	},
}

// feeDebtOp is one step of a sequence. target indexes the run's origins, credits or
// reverted transactions, so a logged sequence replays exactly.
type feeDebtOp struct {
	kind               string
	payer, pkg, target int
	amount             int64
}

func (op feeDebtOp) String() string {
	switch op.kind {
	case opOriginate:
		return fmt.Sprintf("%s payer=%d pkg=%d principal=%d", op.kind, op.payer, op.pkg, op.amount)
	case opCredit:
		return fmt.Sprintf("%s payer=%d amount=%d", op.kind, op.payer, op.amount)
	default:
		return fmt.Sprintf("%s #%d", op.kind, op.target)
	}
}

type feeDebt struct {
	id                                  string
	origin                              *feeDebtOrigin
	opened, settled, canceled, reopened int64
}

func (d *feeDebt) remaining() int64 { return d.opened - d.settled - d.canceled + d.reopened }

type feeDebtOrigin struct {
	tx              uuid.UUID
	payer, pkg      int
	principal, paid int64
	debt            *feeDebt
	reverted        bool
}

type feeDebtSettlement struct {
	debt   *feeDebt
	amount int64
}

type feeDebtCredit struct {
	tx       uuid.UUID
	payer    int
	amount   int64
	settled  []feeDebtSettlement
	reverted bool
}

// feeDebtRun is one seeded sequence over its own accounts and packages, with the
// model the product rules predict for them.
type feeDebtRun struct {
	t                *testing.T
	h                *feeHarness
	debts            *fee_debt.Repository
	txApp, reads     *fiber.App
	tag              string
	payers, fees     [2]string
	receiver, funder string
	balances         map[string]int64
	origins          []*feeDebtOrigin
	credits          []*feeDebtCredit
	reverted         []uuid.UUID
	trace            []string
}

// TestIntegration_Property_FeeDebt_LedgerMatchesTheProductRules runs seeded fee-debt sequences
// on the real engine, completer and projection, holding balances, debts and double entry to the
// product model after every step. FEE_DEBT_PROPERTY_SEEDS sets the seed count; -run .../seed=N replays one.
func TestIntegration_Property_FeeDebt_LedgerMatchesTheProductRules(t *testing.T) {
	h, debts, reads := newFeeDebtPropertyHarness(t)
	txApp := h.newV2App()

	seeds := 5
	if raw := os.Getenv("FEE_DEBT_PROPERTY_SEEDS"); raw != "" {
		var err error
		seeds, err = strconv.Atoi(raw)
		require.NoError(t, err, "FEE_DEBT_PROPERTY_SEEDS")
	}

	shrunk := 0
	for name, ops := range feeDebtShrunk {
		shrunk++
		t.Run("shrunk="+name, func(t *testing.T) {
			r := newFeeDebtRun(t, h, debts, txApp, reads, -shrunk)
			for _, op := range ops {
				r.apply(op)
			}

			r.checkReadPath()
		})
	}

	for seed := 1; seed <= seeds; seed++ {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			r := newFeeDebtRun(t, h, debts, txApp, reads, seed)
			rng := rand.New(rand.NewPCG(uint64(seed), 0))

			for range feeDebtPropertyOps {
				r.apply(r.draw(rng))
			}

			r.checkReadPath()
		})
	}
}

func newFeeDebtPropertyHarness(t *testing.T) (*feeHarness, *fee_debt.Repository, *fiber.App) {
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

	reads := fiber.New()
	apiV2 := reads.Group("/v2")
	hAPI := openapi.New(reads, apiV2, openapi.Config{Title: "fee-debt-property", Version: "test", Servers: []string{"/v2"}})
	http.InstallLedgerSchemaNamer(hAPI)
	RegisterFeeDebtV2RoutesToApp(apiV2, hAPI, &authMiddleware.AuthClient{Enabled: false},
		&FeeDebtHandler{Service: &feesservices.FeeDebtService{Repo: debts}}, nil)

	return h, debts, reads
}

// newFeeDebtRun seeds the run's accounts and its two packages, which only an origin
// naming the run's fee tier selects: credits and reverts carry no fee.
func newFeeDebtRun(t *testing.T, h *feeHarness, debts *fee_debt.Repository, txApp, reads *fiber.App, seed int) *feeDebtRun {
	// Runs share nothing but the ledger. Dropping the last run's Redis state keeps the harness's
	// 128MB Valkey from filling: past it the RDB snapshot fails and Valkey refuses every write.
	require.NoError(t, h.redisContainer.Client.FlushDB(t.Context()).Err())

	tag := "s" + strconv.Itoa(seed)
	r := &feeDebtRun{
		t: t, h: h, debts: debts, txApp: txApp, reads: reads, tag: tag,
		payers:   [2]string{"@" + tag + "-payer0", "@" + tag + "-payer1"},
		fees:     [2]string{"@" + tag + "-fee0", "@" + tag + "-fee1"},
		receiver: "@" + tag + "-receiver", funder: "@" + tag + "-funder",
		balances: map[string]int64{},
	}

	for alias, start := range map[string]int64{
		r.payers[0]: feeDebtPropertyPayerStart, r.payers[1]: feeDebtPropertyPayerStart,
		r.fees[0]: 0, r.fees[1]: 0, r.receiver: 0, r.funder: 1_000_000,
	} {
		h.seedBalance(t, alias, "BRL", decimal.NewFromInt(start), "deposit")
		r.balances[alias] = start
	}

	for pkg, fee := range feeDebtPropertyFees {
		h.seedPackage(t, packageSpec{
			label: r.tier(pkg), metadataSelector: map[string]string{"feeTier": r.tier(pkg)},
			fees: []feeSpec{flatFee(r.tier(pkg)+"_fee", r.fees[pkg], strconv.FormatInt(fee, 10), false)},
		})
	}

	return r
}

func (r *feeDebtRun) tier(pkg int) string { return r.tag + "-" + strconv.Itoa(pkg) }

// description names the step, so no two transactions share a reversal: a revert is
// keyed on its reversal's hash, which carries no origin id (revert_transaction.go).
func (r *feeDebtRun) description() string {
	return "fee debt property " + r.tag + " step " + strconv.Itoa(len(r.trace))
}

// draw picks an operation the model can run; a draw whose kind has no candidate
// draws again, and a credit always has one.
func (r *feeDebtRun) draw(rng *rand.Rand) feeDebtOp {
	for {
		payer := rng.IntN(2)

		switch n := rng.IntN(100); {
		case n < 35:
			if available := r.balances[r.payers[payer]]; available > 0 {
				return feeDebtOp{kind: opOriginate, payer: payer, pkg: rng.IntN(2), amount: 1 + rng.Int64N(available)}
			}
		case n < 70:
			return feeDebtOp{kind: opCredit, payer: payer, amount: 1 + rng.Int64N(60)}
		case n < 82:
			if open := r.candidates(len(r.origins), func(i int) bool { return !r.origins[i].reverted }); len(open) > 0 {
				return feeDebtOp{kind: opRevertOrigin, target: open[rng.IntN(len(open))]}
			}
		case n < 94:
			settling := r.candidates(len(r.credits), func(i int) bool { return !r.credits[i].reverted && len(r.credits[i].settled) > 0 })
			if len(settling) > 0 {
				return feeDebtOp{kind: opRevertCredit, target: settling[rng.IntN(len(settling))]}
			}
		default:
			if len(r.reverted) > 0 {
				return feeDebtOp{kind: opRevertAgain, target: rng.IntN(len(r.reverted))}
			}
		}
	}
}

func (r *feeDebtRun) candidates(n int, eligible func(int) bool) []int {
	var out []int

	for i := range n {
		if eligible(i) {
			out = append(out, i)
		}
	}

	return out
}

func (r *feeDebtRun) apply(op feeDebtOp) {
	r.trace = append(r.trace, op.String())

	switch op.kind {
	case opOriginate:
		r.originate(op)
	case opCredit:
		r.credit(op)
	case opRevertOrigin:
		r.revertOrigin(r.origins[op.target])
	case opRevertCredit:
		r.revertCredit(r.credits[op.target])
	case opRevertAgain:
		r.refuseRevert(r.reverted[op.target], "0087")
	}

	r.check()
}

// originate transfers principal to the receiver under a deferrable fee: the fee takes
// what the principal leaves of the payer's available, and the rest opens a debt.
func (r *feeDebtRun) originate(op feeDebtOp) {
	payer, charge := r.payers[op.payer], feeDebtPropertyFees[op.pkg]
	paid := min(r.balances[payer]-op.amount, charge)
	amount := strconv.FormatInt(op.amount, 10)
	body := r.h.v2WithMetadata(r.h.v2Body(r.description(), "BRL", amount,
		[]string{r.h.v2Leg(payer, amount)}, []string{r.h.v2Leg(r.receiver, amount)}), `{"feeTier":"`+r.tier(op.pkg)+`"}`)

	origin := &feeDebtOrigin{tx: r.create(body), payer: op.payer, pkg: op.pkg, principal: op.amount, paid: paid}
	r.move(payer, r.receiver, op.amount)
	r.move(payer, r.fees[op.pkg], paid)

	if paid < charge {
		origin.debt = &feeDebt{origin: origin, opened: charge - paid, id: r.liveDebtID(op.payer, origin.tx)}
	}

	r.origins = append(r.origins, origin)
}

// credit funds the payer, whose open debts it settles oldest first.
func (r *feeDebtRun) credit(op feeDebtOp) {
	payer, amount := r.payers[op.payer], strconv.FormatInt(op.amount, 10)
	body := r.h.v2Body(r.description(), "BRL", amount, []string{r.h.v2Leg(r.funder, amount)}, []string{r.h.v2Leg(payer, amount)})

	credit := &feeDebtCredit{tx: r.create(body), payer: op.payer, amount: op.amount}
	r.move(r.funder, payer, op.amount)
	credit.settled = r.settle(op.payer, op.amount)
	r.credits = append(r.credits, credit)
}

// revertOrigin cancels the origin's open debt and refunds its whole fee: the part paid
// at origin and every part settled since. Everything the revert credits the payer
// settles the payer's other debts.
func (r *feeDebtRun) revertOrigin(origin *feeDebtOrigin) {
	r.mustRevert(origin.tx)

	payer, fee := r.payers[origin.payer], r.fees[origin.pkg]
	refund := int64(0)

	if debt := origin.debt; debt != nil {
		refund = debt.opened - debt.remaining()
		debt.canceled += debt.remaining()
	}

	r.move(r.receiver, payer, origin.principal)
	r.move(fee, payer, origin.paid+refund)
	origin.reverted = true
	r.settle(origin.payer, origin.principal+origin.paid+refund)
}

// revertCredit gives the credit back to the funder. Each debt it settled whose origin
// still stands reopens, and its fee account returns that part; a part settled into a
// reverted origin was already refunded to the payer, so the payer returns it.
func (r *feeDebtRun) revertCredit(credit *feeDebtCredit) {
	payer, owed := r.payers[credit.payer], credit.amount

	for _, s := range credit.settled {
		if !s.debt.origin.reverted {
			owed -= s.amount
		}
	}

	if r.balances[payer] < owed {
		r.refuseRevert(credit.tx, "0018")

		return
	}

	r.mustRevert(credit.tx)
	r.move(payer, r.funder, owed)

	for _, s := range credit.settled {
		if !s.debt.origin.reverted {
			r.move(r.fees[s.debt.origin.pkg], r.funder, s.amount)
			s.debt.reopened += s.amount
		}
	}

	credit.reverted = true
}

// settle pays the payer's open debts from budget, oldest origin first, full or partial.
func (r *feeDebtRun) settle(payer int, budget int64) []feeDebtSettlement {
	var settled []feeDebtSettlement

	for _, origin := range r.origins {
		debt := origin.debt
		if budget == 0 || origin.payer != payer || debt == nil || debt.remaining() == 0 {
			continue
		}

		amount := min(budget, debt.remaining())
		budget -= amount
		debt.settled += amount
		r.move(r.payers[payer], r.fees[origin.pkg], amount)
		settled = append(settled, feeDebtSettlement{debt: debt, amount: amount})
	}

	return settled
}

func (r *feeDebtRun) move(from, to string, amount int64) {
	r.balances[from] -= amount
	r.balances[to] += amount
}

func (r *feeDebtRun) create(body string) uuid.UUID {
	created := r.h.createV2Direct(r.t, r.txApp, body, map[string]string{"X-Idempotency": uuid.NewString()})
	require.Equalf(r.t, 201, created.status, "create: %s\n%s", created.rawBody, r.sequence())

	return mustTxID(r.t, created)
}

func (r *feeDebtRun) mustRevert(tx uuid.UUID) {
	reverted := r.h.post(r.t, r.txApp, r.h.v2StatePath(tx, "revert"), "", nil)
	require.Equalf(r.t, 201, reverted.status, "revert: %s\n%s", reverted.rawBody, r.sequence())
	require.NotEqualf(r.t, "true", reverted.replayed, "revert of %s answered with another reversal\n%s", tx, r.sequence())
	r.reverted = append(r.reverted, tx)
}

// refuseRevert expects the revert refused with code; check then proves nothing moved.
func (r *feeDebtRun) refuseRevert(tx uuid.UUID, code string) {
	refused := r.h.post(r.t, r.txApp, r.h.v2StatePath(tx, "revert"), "", nil)
	require.Equalf(r.t, code, refused.body["code"], "revert refused with %s: %d %s\n%s", code, refused.status, refused.rawBody, r.sequence())
}

func (r *feeDebtRun) sequence() string {
	return "seed " + r.tag + " sequence:\n  " + strings.Join(r.trace, "\n  ")
}

// check holds the ledger to the model: every balance, the live debt lists, the
// projection of every debt ever opened, and the double entry of every transaction.
func (r *feeDebtRun) check() {
	aliases := append(append([]string{r.receiver, r.funder}, r.payers[:]...), r.fees[:]...)
	refs := make([]string, len(aliases))
	for i, alias := range aliases {
		refs[i] = mtransaction.AliasKey(alias, "default")
	}

	live, err := r.h.queryUC.GetBalances(r.h.ctx(), r.h.orgID, r.h.ledgerID, refs)
	require.NoError(r.t, err)

	want, got := map[string]string{}, map[string]string{}
	for _, alias := range aliases {
		want[alias] = strconv.FormatInt(r.balances[alias], 10)
	}

	for _, balance := range live {
		got[balance.Alias] = balance.Available.String()
		require.Falsef(r.t, balance.Available.IsNegative(), "%s went negative\n%s", balance.Alias, r.sequence())
	}

	require.Equalf(r.t, want, got, "balances\n%s", r.sequence())
	require.Equalf(r.t, r.modelLists(), r.liveLists(), "live debt lists (id=remaining, oldest first)\n%s", r.sequence())
	require.Equalf(r.t, r.modelDebts(), r.projectedDebts(), "projected remaining of every debt\n%s", r.sequence())
	require.Emptyf(r.t, r.unbalancedTransactions(), "transactions whose debits differ from their credits\n%s", r.sequence())
}

// modelLists is each payer's open debts, oldest first, as id=remaining.
func (r *feeDebtRun) modelLists() [2][]string {
	var lists [2][]string

	for _, origin := range r.origins {
		if debt := origin.debt; debt != nil && debt.remaining() > 0 {
			lists[origin.payer] = append(lists[origin.payer], debt.id+"="+strconv.FormatInt(debt.remaining(), 10))
		}
	}

	return lists
}

func (r *feeDebtRun) liveLists() [2][]string {
	var lists [2][]string

	for payer := range r.payers {
		for _, item := range r.liveItems(payer) {
			lists[payer] = append(lists[payer], item.ID+"="+item.Remaining)
		}
	}

	return lists
}

type feeDebtLiveItem struct {
	ID                  string `json:"id"`
	Remaining           string `json:"remaining"`
	OriginTransactionID string `json:"originTransactionId"`
}

func (r *feeDebtRun) liveItems(payer int) []feeDebtLiveItem {
	key := utils.FeeDebtInternalKey(r.h.orgID, r.h.ledgerID, mtransaction.AliasKey(r.payers[payer], "default"))
	values, err := r.h.redisRepo.MGet(r.h.ctx(), []string{key})
	require.NoError(r.t, err)

	var list struct {
		Items []feeDebtLiveItem `json:"items"`
	}

	if raw, found := values[key]; found {
		require.NoError(r.t, json.Unmarshal([]byte(raw), &list))
	}

	return list.Items
}

func (r *feeDebtRun) liveDebtID(payer int, origin uuid.UUID) string {
	for _, item := range r.liveItems(payer) {
		if item.OriginTransactionID == origin.String() {
			return item.ID
		}
	}

	require.Failf(r.t, "no live debt", "origin %s opened no debt on its payer\n%s", origin, r.sequence())

	return ""
}

func (r *feeDebtRun) modelDebts() map[string]string {
	debts := map[string]string{}

	for _, origin := range r.origins {
		if origin.debt != nil {
			debts[origin.debt.id] = strconv.FormatInt(origin.debt.remaining(), 10)
		}
	}

	return debts
}

func (r *feeDebtRun) projectedDebts() map[string]string {
	debts := map[string]string{}

	for id := range r.modelDebts() {
		found, err := r.debts.FindByID(r.h.ctx(), r.h.orgID, r.h.ledgerID, id)
		require.NoError(r.t, err, "projection of %s", id)

		remaining, err := decimal.NewFromString(found.Remaining.String())
		require.NoError(r.t, err)

		debts[id] = remaining.String()
	}

	return debts
}

// unbalancedTransactions lists every transaction of the ledger, of any run, whose
// operations debit a different total than they credit in some asset.
func (r *feeDebtRun) unbalancedTransactions() []string {
	rows, err := r.h.db.Query(`SELECT transaction_id::text || ' ' || asset_code FROM operation
		WHERE organization_id = $1 AND ledger_id = $2 GROUP BY transaction_id, asset_code
		HAVING SUM(amount) FILTER (WHERE direction = 'debit') IS DISTINCT FROM SUM(amount) FILTER (WHERE direction = 'credit')`,
		r.h.orgID, r.h.ledgerID)
	require.NoError(r.t, err)

	defer func() { _ = rows.Close() }()

	var unbalanced []string

	for rows.Next() {
		var view string
		require.NoError(r.t, rows.Scan(&view))
		unbalanced = append(unbalanced, view)
	}

	require.NoError(r.t, rows.Err())

	return unbalanced
}

// checkReadPath holds the fee-debt listing of each payer's open debts, and the total
// it reports owed, to the model.
func (r *feeDebtRun) checkReadPath() {
	for payer, alias := range r.payers {
		status, body := driveFeeV2(r.t, r.reads, "GET", fmt.Sprintf("/v2/organizations/%s/ledgers/%s/fee-debts?account_alias=%s&status=open&limit=100",
			r.h.orgID, r.h.ledgerID, url.QueryEscape(alias)), "")
		require.Equalf(r.t, 200, status, "list: %v", body)

		var listed []string

		items, _ := body["items"].([]any)
		for _, item := range items {
			view, _ := item.(map[string]any)
			listed = append(listed, fmt.Sprintf("%v=%v", view["id"], view["remaining"]))
		}

		total := int64(0)
		for _, origin := range r.origins {
			if origin.payer == payer && origin.debt != nil {
				total += origin.debt.remaining()
			}
		}

		require.Equalf(r.t, r.modelLists()[payer], listed, "listed open debts of %s\n%s", alias, r.sequence())
		require.Equalf(r.t, strconv.FormatInt(total, 10), body["openTotal"], "open total of %s\n%s", alias, r.sequence())
	}
}
