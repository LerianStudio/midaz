//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

type integrationFeeDebt struct {
	ID                  string                   `json:"id"`
	CreditRef           string                   `json:"creditRef"`
	Remaining           string                   `json:"remaining"`
	Opened              string                   `json:"opened"`
	OriginTransactionID string                   `json:"originTransactionId"`
	Seq                 string                   `json:"seq"`
	AssetCode           string                   `json:"assetCode"`
	DebitRoute          *accounting.FeeDebtRoute `json:"debitRoute,omitempty"`
	CreditRoute         *accounting.FeeDebtRoute `json:"creditRoute,omitempty"`
}

type integrationFeeDebtList struct {
	V       int                  `json:"v"`
	NextSeq string               `json:"nextSeq"`
	Items   []integrationFeeDebt `json:"items"`
}

func feeDebt(origin uuid.UUID, debitRef, creditRef, remaining, opened string, seq int) integrationFeeDebt {
	return integrationFeeDebt{
		ID: origin.String() + ":" + debitRef, CreditRef: creditRef, Remaining: remaining, Opened: opened,
		OriginTransactionID: origin.String(), Seq: strconv.Itoa(seq), AssetCode: "USD",
	}
}

// debtRoute is a route whose code and description derive from its id.
func debtRoute(id string) *accounting.FeeDebtRoute {
	return &accounting.FeeDebtRoute{ID: id, Code: "code-" + id, Description: "rubric " + id}
}

// routed gives a seeded debt the routes of its two sides.
func routed(debt integrationFeeDebt, debit, credit string) integrationFeeDebt {
	debt.DebitRoute, debt.CreditRoute = debtRoute(debit), debtRoute(credit)
	return debt
}

func feePosting(ref, balanceRef string, kind accounting.PostingType, amount string) accounting.Posting {
	return accounting.Posting{
		Ref: ref, BalanceRef: balanceRef, Type: kind, Amount: decimal.RequireFromString(amount),
		DrawPolicy: accounting.DrawForbidden, OverdraftAmount: decimal.Zero,
	}
}

// addPoolBalance adds an internal credit-direction balance, on its own account,
// that the execution's cache does not hold.
func (f *integrationFixture) addPoolBalance(alias string, available int64, shape func(*accounting.BalanceSnapshot)) {
	balance := f.input.Execution.Balances[0]
	balance.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(alias+":balance"))
	balance.AccountID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(alias+":account"))
	balance.Alias, balance.Key, balance.BalanceRef = alias, "default", alias+"#default"
	balance.Available, balance.AllowOverdraft = decimal.NewFromInt(available), false
	if shape != nil {
		shape(&balance)
	}
	f.input.Execution.Balances = append(f.input.Execution.Balances, balance)
	f.resolved.Balances[balance.BalanceRef] = testResolvedBalanceKeys(f.prefix + "balance:{transactions}:" +
		f.input.Execution.OrganizationID.String() + ":" + f.input.Execution.LedgerID.String() + ":" + balance.BalanceRef)
}

// declareFeeDebts declares the fee-debt lists of the fixture's transaction.
func (f *integrationFixture) declareFeeDebts(t *testing.T, refs ...string) {
	t.Helper()

	if f.resolved.FeeDebts == nil {
		f.resolved.FeeDebts = make(map[string]string)
	}
	for _, ref := range refs {
		f.input.Execution.Transactions[0].FeeDebtRefs = append(f.input.Execution.Transactions[0].FeeDebtRefs, ref)
		key := f.feeDebtKey(ref)
		f.resolved.FeeDebts[scopedBalanceRef(f.input.Execution.OrganizationID, f.input.Execution.LedgerID, ref)] = key
		t.Cleanup(func() { require.NoError(t, f.client.Del(context.Background(), key).Err()) })
	}
}

func (f *integrationFixture) feeDebtKey(ref string) string {
	return f.prefix + utils.FeeDebtInternalKey(f.input.Execution.OrganizationID, f.input.Execution.LedgerID, ref)
}

func (f *integrationFixture) seedFeeDebts(t *testing.T, ref string, nextSeq int, items ...integrationFeeDebt) string {
	t.Helper()

	raw, err := json.Marshal(integrationFeeDebtList{V: 1, NextSeq: strconv.Itoa(nextSeq), Items: append([]integrationFeeDebt{}, items...)})
	require.NoError(t, err)
	require.NoError(t, f.client.Set(context.Background(), f.feeDebtKey(ref), raw, 0).Err())

	return string(raw)
}

func (f *integrationFixture) storedFeeDebts(t *testing.T, ref string) integrationFeeDebtList {
	t.Helper()

	raw, err := f.client.Get(context.Background(), f.feeDebtKey(ref)).Result()
	require.NoError(t, err)

	var list integrationFeeDebtList
	require.NoError(t, json.Unmarshal([]byte(raw), &list))

	return list
}

// revert turns the fixture's transaction into a revert of parent.
func (f *integrationFixture) revert(parent uuid.UUID) {
	f.input.CompletionPlans[0].Payload = json.RawMessage(`{"action":"revert","parentTransactionId":"` + parent.String() + `"}`)
}

// execute runs the prepared execution and decodes it exactly as the adapter does.
func (f *integrationFixture) execute(t *testing.T) (string, *accounting.ExecutionResult) {
	t.Helper()

	raw, err := f.run(t)
	require.NoError(t, err)
	result, err := DecodeResult([]byte(raw), f.input.Execution)
	require.NoError(t, err)

	return raw, result
}

// requireReplay proves the stored receipt replays byte-identically and writes nothing.
func (f *integrationFixture) requireReplay(t *testing.T, raw string) {
	t.Helper()

	before := f.capture(t)
	replay, err := f.run(t)
	require.NoError(t, err)
	require.Equal(t, raw, replay)
	require.Equal(t, before, f.capture(t))
}

func (f *integrationFixture) requireUnchanged(t *testing.T, code string, fragments ...string) {
	t.Helper()

	before := f.capture(t)
	_, err := f.run(t)
	require.ErrorContains(t, err, `"code":"`+code+`"`)
	for _, fragment := range fragments {
		require.ErrorContains(t, err, fragment)
	}
	require.Equal(t, before, f.capture(t))
}

func movementLines(result *accounting.ExecutionResult) []string {
	lines := make([]string, 0, len(result.Movements))
	for _, m := range result.Movements {
		lines = append(lines, fmt.Sprintf("%s %s:%s %s %s %s", m.PostingRef, m.Role, m.Ref[strings.LastIndexByte(m.Ref, ':')+1:], m.BalanceRef, m.Type, m.Amount))
	}

	return lines
}

func changeLines(result *accounting.ExecutionResult) []string {
	lines := make([]string, 0, len(result.FeeDebt))
	for _, c := range result.FeeDebt {
		line := fmt.Sprintf("%s %q %s %s->%s %s/%s seq %d %s", c.Kind, c.PostingRef, c.DebtID, c.DebtorRef, c.CreditRef, c.Amount, c.Opened, c.Seq, c.AssetCode)
		if c.DebitRoute != nil || c.CreditRoute != nil {
			line += " routes " + routeLine(c.DebitRoute) + "/" + routeLine(c.CreditRoute)
		}
		lines = append(lines, line)
	}

	return lines
}

func routeLine(route *accounting.FeeDebtRoute) string {
	if route == nil {
		return "-"
	}

	return route.ID + "|" + route.Code + "|" + route.Description
}

func finalAvailable(result *accounting.ExecutionResult) map[string]string {
	final := make(map[string]string, len(result.Final))
	for _, balance := range result.Final {
		final[balance.BalanceRef] = balance.Available.String()
	}

	return final
}

// newDeferralFixture charges a fee of 100 to @source, which holds 30, for @fees.
func newDeferralFixture(t *testing.T, client redis.UniversalClient) *integrationFixture {
	t.Helper()

	f := newIntegrationFixture(t, client)
	f.input.Execution.Balances[0].Available = decimal.NewFromInt(30)
	f.addPoolBalance("@fees", 0, nil)
	f.declareFeeDebts(t, "@source#default")
	debit := feePosting("fee-debit", "@source#default", accounting.PostingDebit, "100")
	debit.DeferShortfall, debit.DrawPolicy = true, accounting.DrawAllowed
	credit := feePosting("fee-credit", "@fees#default", accounting.PostingCredit, "100")
	credit.FundedByRef = "fee-debit"
	f.input.Execution.Transactions[0].Postings = []accounting.Posting{debit, credit}

	return f
}

func TestIntegrationFeeDebtDeferralOpensTheShortfall(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newDeferralFixture(t, container.Client)
	tx := f.input.Execution.Transactions[0].ID.String()

	raw, result := f.execute(t)

	require.Equal(t, []string{
		"fee-debit primary:0 @source#default debit 30",
		"fee-credit primary:0 @fees#default credit 30",
	}, movementLines(result))
	require.True(t, result.Movements[0].OverdraftDelta.IsZero(), "a deferrable debit never draws overdraft")
	require.Equal(t, map[string]string{"@source#default": "0", "@fees#default": "30"}, finalAvailable(result))
	require.Equal(t, []string{
		`opened "fee-debit" ` + tx + `:fee-debit @source#default->@fees#default 70/70 seq 1 USD`,
	}, changeLines(result))
	require.Equal(t, f.input.Execution.Transactions[0].ID, result.FeeDebt[0].OriginTransactionID)

	stored, err := f.client.Get(context.Background(), f.feeDebtKey("@source#default")).Result()
	require.NoError(t, err)
	require.Equal(t, `{"items":[{"assetCode":"USD","creditRef":"@fees#default","id":"`+tx+`:fee-debit","opened":"70",`+
		`"originTransactionId":"`+tx+`","remaining":"70","seq":"1"}],"nextSeq":"2","v":1}`, stored)
	ttl, err := f.client.TTL(context.Background(), f.feeDebtKey("@source#default")).Result()
	require.NoError(t, err)
	require.Equal(t, int64(-1), int64(ttl), "a fee-debt list never expires")

	recovery, err := f.client.HGet(context.Background(), f.resolved.Recovery, tx+":"+f.input.Execution.ExecutionID.String()).Result()
	require.NoError(t, err)
	var saved struct {
		Record struct{ Result accounting.ExecutionResult }
	}
	require.NoError(t, json.Unmarshal([]byte(recovery), &saved))
	require.Equal(t, result.FeeDebt, saved.Record.Result.FeeDebt)

	f.requireReplay(t, raw)
}

func TestIntegrationFeeDebtDeferralRepayRouteDenied(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	t.Run("fully deferred fee repays nothing and passes", func(t *testing.T) {
		f := newDeferralFixture(t, container.Client)
		f.input.Execution.Balances[0].Available = decimal.NewFromInt(100)
		f.input.Execution.Balances[1].OverdraftUsed = decimal.NewFromInt(50)
		f.input.Execution.Transactions[0].Postings[1].RepayRouteDenied = true
		f.addPoolBalance("@merchant", 0, nil)
		fees := f.input.Execution.Transactions[0].Postings
		f.input.Execution.Transactions[0].Postings = append([]accounting.Posting{
			feePosting("main-debit", "@source#default", accounting.PostingDebit, "100"),
			feePosting("main-credit", "@merchant#default", accounting.PostingCredit, "100"),
		}, fees...)

		raw, err := f.run(t)
		require.NoError(t, err, "a zero funded credit cannot repay overdraft")
		result, err := DecodeResult([]byte(raw), f.input.Execution)
		require.NoError(t, err)
		require.Len(t, result.FeeDebt, 1)
		require.Equal(t, "100", result.FeeDebt[0].Amount.String())

		for _, final := range result.Final {
			if final.BalanceRef == "@fees#default" {
				require.Equal(t, "50", final.OverdraftUsed.String(), "the fee account keeps its debt")
			}
		}
	})

	t.Run("partly funded fee would repay and refuses", func(t *testing.T) {
		f := newDeferralFixture(t, container.Client)
		f.input.Execution.Balances[1].OverdraftUsed = decimal.NewFromInt(50)
		f.input.Execution.Transactions[0].Postings[1].RepayRouteDenied = true
		before := f.capture(t)

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"overdraft_repay_route_denied"`)
		require.Equal(t, before, f.capture(t))
	})
}

func TestIntegrationFeeDebtDeferralKeepsTheFeeRoutes(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newDeferralFixture(t, container.Client)
	postings := f.input.Execution.Transactions[0].Postings
	postings[0].DebtRoute, postings[1].DebtRoute = debtRoute("r-from"), debtRoute("r-to")
	tx := f.input.Execution.Transactions[0].ID.String()

	raw, result := f.execute(t)

	require.Equal(t, []string{
		`opened "fee-debit" ` + tx + `:fee-debit @source#default->@fees#default 70/70 seq 1 USD routes r-from|code-r-from|rubric r-from/r-to|code-r-to|rubric r-to`,
	}, changeLines(result))
	debt := routed(feeDebt(f.input.Execution.Transactions[0].ID, "fee-debit", "@fees#default", "70", "70", 1), "r-from", "r-to")
	require.Equal(t, integrationFeeDebtList{V: 1, NextSeq: "2", Items: []integrationFeeDebt{debt}}, f.storedFeeDebts(t, "@source#default"))

	f.requireReplay(t, raw)
}

func TestIntegrationFeeDebtDeferralCap(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	full := make([]integrationFeeDebt, 0, 256)
	for seq := 1; seq <= 256; seq++ {
		full = append(full, feeDebt(uuid.NewSHA1(uuid.NameSpaceOID, []byte{byte(seq), byte(seq >> 8)}), "fee-debit", "@fees#default", "1", "1", seq))
	}

	t.Run("a shortfall on a full list is refused", func(t *testing.T) {
		f := newDeferralFixture(t, container.Client)
		f.seedFeeDebts(t, "@source#default", 257, full...)
		f.requireUnchanged(t, "insufficient_funds", `"postingIndex":0`, `"balanceRef":"@source#default"`)
	})

	t.Run("a funded fee ignores the cap", func(t *testing.T) {
		f := newDeferralFixture(t, container.Client)
		seeded := f.seedFeeDebts(t, "@source#default", 257, full...)
		f.input.Execution.Balances[0].Available = decimal.NewFromInt(100)

		_, result := f.execute(t)
		require.Empty(t, result.FeeDebt)
		stored, err := f.client.Get(context.Background(), f.feeDebtKey("@source#default")).Result()
		require.NoError(t, err)
		require.Equal(t, seeded, stored)
	})
}

// newCollectFixture credits @source 1000 from @dest and collects its debts.
func newCollectFixture(t *testing.T, client redis.UniversalClient, items ...string) *integrationFixture {
	t.Helper()

	f := newIntegrationFixture(t, client)
	f.input.Execution.Balances[0].Available = decimal.Zero
	f.addPoolBalance("@dest", 1000, nil)
	f.addPoolBalance("@fees", 0, nil)
	f.addPoolBalance("@fees2", 0, nil)
	f.declareFeeDebts(t, "@source#default")
	collect := feePosting("inflow:collect", "@source#default", accounting.PostingCollect, "1000")
	collect.Items = items
	f.input.Execution.Transactions[0].Postings = []accounting.Posting{
		feePosting("inflow-debit", "@dest#default", accounting.PostingDebit, "1000"),
		feePosting("inflow", "@source#default", accounting.PostingCredit, "1000"),
		collect,
	}

	return f
}

func TestIntegrationFeeDebtCollectSettlesOldestFirst(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	o1, o2, o3 := uuid.MustParse("0a4b6d8e-1111-4a1a-8a1a-000000000001"), uuid.MustParse("0a4b6d8e-1111-4a1a-8a1a-000000000002"), uuid.MustParse("0a4b6d8e-1111-4a1a-8a1a-000000000003")

	t.Run("a credit of 1000 settles 200 and leaves 800", func(t *testing.T) {
		d1, d2 := routed(feeDebt(o1, "fee-debit", "@fees#default", "150", "150", 1), "a-from", "a-to"), feeDebt(o2, "fee-debit", "@fees#default", "50", "70", 2)
		f := newCollectFixture(t, container.Client, d1.ID, d2.ID)
		f.seedFeeDebts(t, "@source#default", 3, d1, d2)

		raw, result := f.execute(t)
		require.Equal(t, []string{
			"inflow-debit primary:0 @dest#default debit 1000",
			"inflow primary:0 @source#default credit 1000",
			"inflow:collect fee_debt_debit:0 @source#default debit 150",
			"inflow:collect fee_debt_credit:0 @fees#default credit 150",
			"inflow:collect fee_debt_debit:1 @source#default debit 50",
			"inflow:collect fee_debt_credit:1 @fees#default credit 50",
		}, movementLines(result))
		// @fees is in no other leg: the settlement alone moves it.
		require.Equal(t, map[string]string{"@dest#default": "0", "@source#default": "800", "@fees#default": "200"}, finalAvailable(result))
		require.Equal(t, []string{
			`settled "inflow:collect" ` + d1.ID + ` @source#default->@fees#default 150/150 seq 1 USD routes a-from|code-a-from|rubric a-from/a-to|code-a-to|rubric a-to`,
			`settled "inflow:collect" ` + d2.ID + ` @source#default->@fees#default 50/70 seq 2 USD`,
		}, changeLines(result))
		require.Equal(t, integrationFeeDebtList{V: 1, NextSeq: "3", Items: []integrationFeeDebt{}}, f.storedFeeDebts(t, "@source#default"))

		recovery, err := f.client.HGet(context.Background(), f.resolved.Recovery, f.input.Execution.Transactions[0].ID.String()+":"+f.input.Execution.ExecutionID.String()).Result()
		require.NoError(t, err)
		var saved struct {
			Record struct{ Result accounting.ExecutionResult }
		}
		require.NoError(t, json.Unmarshal([]byte(recovery), &saved))
		require.Equal(t, result.Movements[5].Ref, saved.Record.Result.Movements[5].Ref)
		require.True(t, strings.HasSuffix(saved.Record.Result.Movements[5].Ref, ":inflow:collect:fee_debt_credit:1"))
		require.Equal(t, result.FeeDebt, saved.Record.Result.FeeDebt)

		f.requireReplay(t, raw)
	})

	t.Run("strict fifo across two fee accounts with a partial settlement", func(t *testing.T) {
		d1, d2, d3 := feeDebt(o1, "fee-debit", "@fees#default", "50", "50", 1), feeDebt(o2, "fee-debit", "@fees2#default", "80", "80", 2), feeDebt(o3, "fee-debit", "@fees#default", "40", "40", 3)
		f := newCollectFixture(t, container.Client, d1.ID, d2.ID, d3.ID)
		f.input.Execution.Balances[1].Available = decimal.NewFromInt(100)
		postings := f.input.Execution.Transactions[0].Postings
		postings[0].Amount, postings[1].Amount, postings[2].Amount = decimal.NewFromInt(100), decimal.NewFromInt(100), decimal.NewFromInt(100)
		f.seedFeeDebts(t, "@source#default", 4, d1, d2, d3)

		_, result := f.execute(t)
		require.Equal(t, []string{
			"inflow-debit primary:0 @dest#default debit 100",
			"inflow primary:0 @source#default credit 100",
			"inflow:collect fee_debt_debit:0 @source#default debit 50",
			"inflow:collect fee_debt_credit:0 @fees#default credit 50",
			"inflow:collect fee_debt_debit:1 @source#default debit 50",
			"inflow:collect fee_debt_credit:1 @fees2#default credit 50",
		}, movementLines(result))
		d2.Remaining = "30"
		require.Equal(t, []integrationFeeDebt{d2, d3}, f.storedFeeDebts(t, "@source#default").Items)
	})

	t.Run("it stops at the first live debt outside its items", func(t *testing.T) {
		d1, d2, d3 := feeDebt(o1, "fee-debit", "@fees#default", "10", "10", 1), feeDebt(o2, "fee-debit", "@fees2#default", "20", "20", 2), feeDebt(o3, "fee-debit", "@fees#default", "30", "30", 3)
		f := newCollectFixture(t, container.Client, d1.ID, d3.ID)
		f.seedFeeDebts(t, "@source#default", 4, d1, d2, d3)

		_, result := f.execute(t)
		require.Equal(t, []string{
			"inflow-debit primary:0 @dest#default debit 1000",
			"inflow primary:0 @source#default credit 1000",
			"inflow:collect fee_debt_debit:0 @source#default debit 10",
			"inflow:collect fee_debt_credit:0 @fees#default credit 10",
		}, movementLines(result))
		require.Equal(t, []integrationFeeDebt{d2, d3}, f.storedFeeDebts(t, "@source#default").Items)
	})
}

func TestIntegrationFeeDebtCollectStopsSoftly(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	d1 := feeDebt(uuid.MustParse("0a4b6d8e-2222-4a1a-8a1a-000000000001"), "fee-debit", "@fees#default", "10", "10", 1)
	d2 := feeDebt(uuid.MustParse("0a4b6d8e-2222-4a1a-8a1a-000000000002"), "fee-debit", "@fees2#default", "20", "20", 2)

	for _, test := range []struct {
		name    string
		shape   func(*integrationFixture)
		settled int
	}{
		{name: "a fee account of debit direction", settled: 1, shape: func(f *integrationFixture) { f.input.Execution.Balances[3].Direction = "debit" }},
		{name: "a blocked fee account", settled: 1, shape: func(f *integrationFixture) { f.input.Execution.Balances[3].Blocked = true }},
		{name: "a fee account that may not receive", settled: 1, shape: func(f *integrationFixture) { f.input.Execution.Balances[3].AllowReceiving = false }},
		{name: "a debtor that may not send", settled: 0, shape: func(f *integrationFixture) { f.input.Execution.Balances[0].AllowSending = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newCollectFixture(t, container.Client, d1.ID, d2.ID)
			seeded := f.seedFeeDebts(t, "@source#default", 3, d1, d2)
			test.shape(f)

			raw, result := f.execute(t)
			require.Equal(t, "inflow primary:0 @source#default credit 1000", movementLines(result)[1], "the credit always succeeds")
			require.Len(t, result.FeeDebt, test.settled)
			if test.settled == 0 {
				require.NotContains(t, raw, `"feeDebt"`)
				stored, err := f.client.Get(context.Background(), f.feeDebtKey("@source#default")).Result()
				require.NoError(t, err)
				require.Equal(t, seeded, stored, "an unsettled list is not rewritten")

				return
			}
			require.Equal(t, "990", finalAvailable(result)["@source#default"])
			require.Equal(t, []integrationFeeDebt{d2}, f.storedFeeDebts(t, "@source#default").Items)
		})
	}
}

func TestIntegrationFeeDebtRejectsMalformedProtocol(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	parent := uuid.MustParse("0a4b6d8e-3333-4a1a-8a1a-000000000001")
	refund := func(f *integrationFixture, amount string, debtID string) accounting.Posting {
		posting := feePosting("fee-refund:0", "@source#default", accounting.PostingRefund, amount)
		posting.Refunds = []accounting.FeeDebtRefund{{DebtID: debtID, CreditRef: "@fees#default", Opened: decimal.NewFromInt(70), Seq: 1}}
		return posting
	}

	for _, test := range []struct {
		name  string
		shape func(*integrationFixture)
	}{
		{name: "a deferral on a credit", shape: func(f *integrationFixture) { f.input.Execution.Transactions[0].Postings[1].DeferShortfall = true }},
		{name: "a deferral outside a direct transaction", shape: func(f *integrationFixture) {
			f.input.CompletionPlans[0].Payload = json.RawMessage(`{"action":"commit"}`)
		}},
		{name: "a deferral on an undeclared payer", shape: func(f *integrationFixture) {
			f.input.Execution.Transactions[0].FeeDebtRefs, f.resolved.FeeDebts = nil, nil
		}},
		{name: "a funding credit naming no deferral", shape: func(f *integrationFixture) { f.input.Execution.Transactions[0].Postings[1].FundedByRef = "missing" }},
		{name: "a funding credit of another amount", shape: func(f *integrationFixture) {
			f.input.Execution.Transactions[0].Postings[1].Amount = decimal.NewFromInt(99)
		}},
		{name: "an unfunded deferral", shape: func(f *integrationFixture) { f.input.Execution.Transactions[0].Postings[1].FundedByRef = "" }},
		{name: "a funding credit before its deferral", shape: func(f *integrationFixture) {
			postings := f.input.Execution.Transactions[0].Postings
			postings[0], postings[1] = postings[1], postings[0]
		}},
		{name: "two credits funding one deferral", shape: func(f *integrationFixture) {
			postings := &f.input.Execution.Transactions[0].Postings
			second := (*postings)[1]
			second.Ref = "fee-credit-2"
			*postings = append(*postings, second)
		}},
		{name: "a debt route outside a deferrable fee", shape: func(f *integrationFixture) {
			postings := f.input.Execution.Transactions[0].Postings
			postings[0].DeferShortfall, postings[0].DebtRoute, postings[1].FundedByRef = false, debtRoute("r"), ""
		}},
		{name: "a debt route without an id", shape: func(f *integrationFixture) {
			f.input.Execution.Transactions[0].Postings[0].DebtRoute = &accounting.FeeDebtRoute{}
		}},
		{name: "a collect without items", shape: func(f *integrationFixture) {
			f.input.Execution.Transactions[0].Postings = append(f.input.Execution.Transactions[0].Postings, feePosting("collect", "@source#default", accounting.PostingCollect, "1"))
		}},
		{name: "a collect on an undeclared debtor", shape: func(f *integrationFixture) {
			collect := feePosting("collect", "@fees#default", accounting.PostingCollect, "1")
			collect.Items = []string{parent.String() + ":fee-debit"}
			f.input.Execution.Transactions[0].Postings = append(f.input.Execution.Transactions[0].Postings, collect)
		}},
		{name: "a refund outside a revert", shape: func(f *integrationFixture) {
			f.input.Execution.Transactions[0].Postings = []accounting.Posting{refund(f, "70", parent.String()+":fee-debit")}
		}},
		{name: "a refund amount other than its entries", shape: func(f *integrationFixture) {
			f.revert(parent)
			f.input.Execution.Transactions[0].Postings = []accounting.Posting{refund(f, "71", parent.String()+":fee-debit")}
		}},
		{name: "a refund of another parent's debt", shape: func(f *integrationFixture) {
			f.revert(parent)
			f.input.Execution.Transactions[0].Postings = []accounting.Posting{refund(f, "70", "0a4b6d8e-3333-4a1a-8a1a-000000000002:fee-debit")}
		}},
		{name: "a refund expecting more than its debt opened", shape: func(f *integrationFixture) {
			f.revert(parent)
			posting := refund(f, "70", parent.String()+":fee-debit")
			posting.Refunds[0].ExpectedRefund = decimal.NewFromInt(71)
			f.input.Execution.Transactions[0].Postings = []accounting.Posting{posting}
		}},
		{name: "a reopen outside a revert", shape: func(f *integrationFixture) {
			f.input.Execution.Transactions[0].ReopenFeeDebts = []accounting.FeeDebtReopen{{
				DebtID: parent.String() + ":fee-debit", DebtorRef: "@source#default", CreditRef: "@fees#default",
				Amount: decimal.NewFromInt(1), Opened: decimal.NewFromInt(1), Seq: 1,
			}}
		}},
		{name: "a reopen whose fee account no posting debits", shape: func(f *integrationFixture) {
			f.revert(parent)
			f.input.Execution.Transactions[0].Postings = []accounting.Posting{feePosting("rev", "@source#default", accounting.PostingDebit, "1")}
			f.input.Execution.Transactions[0].ReopenFeeDebts = []accounting.FeeDebtReopen{{
				DebtID: "0a4b6d8e-3333-4a1a-8a1a-000000000003:fee-debit", DebtorRef: "@source#default", CreditRef: "@fees#default",
				Amount: decimal.NewFromInt(1), Opened: decimal.NewFromInt(1), Seq: 1,
			}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newDeferralFixture(t, container.Client)
			test.shape(f)
			f.requireUnchanged(t, "invalid_protocol")
		})
	}

	evalKeys := func(t *testing.T, f *integrationFixture, keys []string) error {
		prepared := f.prepared(t)
		return f.client.Eval(context.Background(), integrationEngineLua, keys, string(prepared.Payload),
			strconv.Itoa(f.limits.MaxRequestBytes), strconv.Itoa(f.limits.MaxPreparedBytes),
			strconv.Itoa(f.limits.MaxTransactions), strconv.Itoa(f.limits.MaxPostings), strconv.Itoa(f.limits.MaxBalances)).Err()
	}

	t.Run("a key inventory without the fee-debt key", func(t *testing.T) {
		f := newDeferralFixture(t, container.Client)
		keys := f.prepared(t).Keys
		require.ErrorContains(t, evalKeys(t, f, keys[:len(keys)-1]), `"code":"invalid_protocol"`)
	})

	t.Run("a fee-debt key of another balance", func(t *testing.T) {
		f := newDeferralFixture(t, container.Client)
		keys := f.prepared(t).Keys
		keys[len(keys)-1] = f.feeDebtKey("@fees#default")
		require.ErrorContains(t, evalKeys(t, f, keys), `"code":"invalid_protocol"`)
	})

	t.Run("an empty fee-debt inventory", func(t *testing.T) {
		f := newIntegrationFixture(t, container.Client)
		payload := strings.Replace(string(f.prepared(t).Payload), `"executionId"`, `"feeDebts":[],"executionId"`, 1)
		_, err := f.runRaw(t, payload)
		require.ErrorContains(t, err, `"code":"invalid_protocol"`)
	})

	t.Run("fee-debt changes without a movement", func(t *testing.T) {
		f := newDeferralFixture(t, container.Client)
		f.input.Execution.Balances[0].Available = decimal.Zero
		f.requireUnchanged(t, "fee_debt_conflict")
	})

	for name, raw := range map[string]string{
		"invalid json": `not json`, "a zero nextSeq": `{"items":[],"nextSeq":"0","v":1}`,
		"a route without an id": `{"items":[{"assetCode":"USD","creditRef":"@fees#default","debitRoute":{"code":"","description":"","id":""},` +
			`"id":"0a4b6d8e-3333-4a1a-8a1a-000000000009:fee-debit","opened":"1","originTransactionId":"0a4b6d8e-3333-4a1a-8a1a-000000000009",` +
			`"remaining":"1","seq":"1"}],"nextSeq":"2","v":1}`,
	} {
		t.Run("a stored list with "+name+" is a conflict", func(t *testing.T) {
			f := newDeferralFixture(t, container.Client)
			require.NoError(t, f.client.Set(context.Background(), f.feeDebtKey("@source#default"), raw, 0).Err())
			f.requireUnchanged(t, "fee_debt_conflict")
		})
	}
}

// noFeeDebtResponse is the response the engine gave the base fixture before fee
// debts existed, less its application instant.
const noFeeDebtResponse = `{"final":[{"accountId":"1f3da5d4-1571-4619-938f-1aef51560726","accountType":"deposit","alias":"@source","allowOverdraft":true,"allowReceiving":true,"allowSending":true,"assetCode":"USD","available":"70","balanceRef":"@source#default","balanceScope":"transactional","blocked":false,"direction":"credit","id":"d9e5a2be-9128-43ab-9d3c-0c14e3a8b889","key":"default","ledgerId":"fbe630e6-fde0-4396-bb3b-b5e5da509ae0","onHold":"0","organizationId":"ef73c171-f889-4a9d-a5d3-421ee069d736","overdraftLimit":"1000","overdraftLimitEnabled":false,"overdraftUsed":"0","version":"1"}],"movements":[{"after":{"available":"70","onHold":"0","overdraftUsed":"0","version":"1"},"amount":"30","balanceRef":"@source#default","before":{"available":"100","onHold":"0","overdraftUsed":"0","version":"0"},"overdraftDelta":"0","postingRef":"debit-0","ref":"53d2c279-4d51-4274-8a3d-ee1955ddcaa0:7:debit-0:primary:0","role":"primary","transactionId":"53d2c279-4d51-4274-8a3d-ee1955ddcaa0","type":"debit"}],"protocolVersion":1}`

func TestIntegrationFeeDebtAbsentKeepsTheResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)

	raw, err := f.run(t)
	require.NoError(t, err)
	require.Equal(t, noFeeDebtResponse, regexp.MustCompile(`^\{"appliedAtUnixMicro":[0-9]+,`).ReplaceAllString(raw, "{"))
	recovery, err := f.client.HGet(context.Background(), f.resolved.Recovery, f.input.Execution.Transactions[0].ID.String()+":"+f.input.Execution.ExecutionID.String()).Result()
	require.NoError(t, err)
	require.NotContains(t, recovery, "feeDebt")
}
