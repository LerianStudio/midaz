// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/v2/metrics"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// ---- test helpers -----------------------------------------------------------

// exceptionEnrichBalance builds an mmodel.Balance carrying the block/status flags
// the enrichment reads to decide whether a side would-be-deny.
func exceptionEnrichBalance(accountID, alias, key string, blocked, allowSend, allowRecv bool) *mmodel.Balance {
	return &mmodel.Balance{
		ID:             uuid.New().String(),
		AccountID:      accountID,
		Alias:          alias,
		Key:            key,
		AssetCode:      "BRL",
		Available:      decimal.Zero,
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   allowSend,
		AllowReceiving: allowRecv,
		AccountBlocked: blocked,
		Direction:      constant.DirectionCredit,
	}
}

// fromValidate builds a single-source validate keyed exactly as the real funnel:
// From is keyed by the concat form ("0#@alias#key"), Aliases/Sources carry the
// bare alias-key form ("@alias#key").
func fromValidate(alias, key string) (*mtransaction.Responses, string) {
	ak := mtransaction.AliasKey(alias, key)
	concat := mtransaction.ConcatAlias(0, ak)

	return &mtransaction.Responses{
		Asset:   "BRL",
		From:    map[string]mtransaction.Amount{concat: {Asset: "BRL", Value: decimal.NewFromInt(100), Operation: constant.DEBIT}},
		Aliases: []string{ak},
		Sources: []string{ak},
	}, concat
}

// toValidate builds a single-destination validate (credit side).
func toValidate(alias, key string) (*mtransaction.Responses, string) {
	ak := mtransaction.AliasKey(alias, key)
	concat := mtransaction.ConcatAlias(0, ak)

	return &mtransaction.Responses{
		Asset:        "BRL",
		To:           map[string]mtransaction.Amount{concat: {Asset: "BRL", Value: decimal.NewFromInt(100), Operation: constant.CREDIT}},
		Aliases:      []string{ak},
		Destinations: []string{ak},
	}, concat
}

func exc(id string, codes []string, balanceKey *string, effAt, expAt *time.Time) *mmodel.AccountException {
	return &mmodel.AccountException{
		ID:                   id,
		OperationalTypeCodes: codes,
		BalanceKey:           balanceKey,
		EffectiveAt:          effAt,
		ExpiresAt:            expAt,
	}
}

// countingLoader returns a loader closure and a pointer to its invocation count.
func countingLoader(exceptions []*mmodel.AccountException, loaderErr error) (activeExceptionsLoader, *int) {
	calls := 0

	return func(_ context.Context, _, _, _ uuid.UUID) ([]*mmodel.AccountException, error) {
		calls++
		return exceptions, loaderErr
	}, &calls
}

// ---- matchAccountException: validity + scope boundaries (deterministic now) --

func TestMatchAccountException_ValidityAndScope(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	hourBefore := now.Add(-time.Hour)
	hourAfter := now.Add(time.Hour)
	exact := now

	tests := []struct {
		name       string
		exceptions []*mmodel.AccountException
		code       string
		balanceKey string
		wantID     string // "" => no match
	}{
		{"exact code match, no bounds", []*mmodel.AccountException{exc("E", []string{"PIX_IN"}, nil, nil, nil)}, "PIX_IN", "default", "E"},
		{"code not covered", []*mmodel.AccountException{exc("E", []string{"TED_OUT"}, nil, nil, nil)}, "PIX_IN", "default", ""},
		{"balance-key scope covers", []*mmodel.AccountException{exc("E", []string{"PIX_IN"}, ptr("default"), nil, nil)}, "PIX_IN", "default", "E"},
		{"balance-key scope does not cover", []*mmodel.AccountException{exc("E", []string{"PIX_IN"}, ptr("asset-freeze"), nil, nil)}, "PIX_IN", "default", ""},
		{"validity within window", []*mmodel.AccountException{exc("E", []string{"PIX_IN"}, nil, &hourBefore, &hourAfter)}, "PIX_IN", "default", "E"},
		{"validity expired", []*mmodel.AccountException{exc("E", []string{"PIX_IN"}, nil, &hourBefore, &hourBefore)}, "PIX_IN", "default", ""},
		{"validity future", []*mmodel.AccountException{exc("E", []string{"PIX_IN"}, nil, &hourAfter, nil)}, "PIX_IN", "default", ""},
		{"validity indeterminate", []*mmodel.AccountException{exc("E", []string{"PIX_IN"}, nil, nil, nil)}, "PIX_IN", "default", "E"},
		{"expiry exact boundary is exclusive", []*mmodel.AccountException{exc("E", []string{"PIX_IN"}, nil, nil, &exact)}, "PIX_IN", "default", ""},
		{"effective exact boundary is inclusive", []*mmodel.AccountException{exc("E", []string{"PIX_IN"}, nil, &exact, nil)}, "PIX_IN", "default", "E"},
		{"first-match wins (loader ASC order)", []*mmodel.AccountException{
			exc("E1", []string{"PIX_IN"}, nil, nil, nil),
			exc("E2", []string{"PIX_IN"}, nil, nil, nil),
		}, "PIX_IN", "default", "E1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := matchAccountException(tt.exceptions, tt.code, tt.balanceKey, now)

			if tt.wantID == "" {
				assert.Nil(t, got, "expected no match")
				return
			}

			require.NotNil(t, got, "expected a match")
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

// ---- enrichAccountExceptionGrants: full matrix -------------------------------

func TestEnrichAccountExceptionGrants(t *testing.T) {
	t.Parallel()

	org := uuid.New()
	ledger := uuid.New()
	acc := uuid.New().String()

	tests := []struct {
		name            string
		side            string // "from" | "to"
		code            string
		blocked         bool
		allowSend       bool
		allowRecv       bool
		balanceKey      string
		exceptions      []*mmodel.AccountException
		loaderErr       error
		wantAppliedID   string // "" => nil
		wantGranted     bool
		wantLoaderCalls int
	}{
		{
			name: "block on debit transpassed", side: "from", code: "PIX_IN",
			blocked: true, allowSend: true, allowRecv: true, balanceKey: "default",
			exceptions:    []*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, nil, nil, nil)},
			wantAppliedID: "E1", wantGranted: true, wantLoaderCalls: 1,
		},
		{
			name: "block on credit transpassed", side: "to", code: "PIX_IN",
			blocked: true, allowSend: true, allowRecv: true, balanceKey: "default",
			exceptions:    []*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, nil, nil, nil)},
			wantAppliedID: "E1", wantGranted: true, wantLoaderCalls: 1,
		},
		{
			name: "0024 status restriction transpassed", side: "from", code: "PIX_IN",
			blocked: false, allowSend: false, allowRecv: true, balanceKey: "default",
			exceptions:    []*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, nil, nil, nil)},
			wantAppliedID: "E1", wantGranted: true, wantLoaderCalls: 1,
		},
		{
			name: "type not covered rejects (narrow exception)", side: "from", code: "PIX_IN",
			blocked: true, allowSend: true, allowRecv: true, balanceKey: "default",
			exceptions:    []*mmodel.AccountException{exc("E1", []string{"TED_OUT"}, nil, nil, nil)},
			wantAppliedID: "", wantGranted: false, wantLoaderCalls: 1,
		},
		{
			name: "balance-key scope covers", side: "from", code: "PIX_IN",
			blocked: true, allowSend: true, allowRecv: true, balanceKey: "default",
			exceptions:    []*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, ptr("default"), nil, nil)},
			wantAppliedID: "E1", wantGranted: true, wantLoaderCalls: 1,
		},
		{
			name: "balance-key scope does not cover", side: "from", code: "PIX_IN",
			blocked: true, allowSend: true, allowRecv: true, balanceKey: "default",
			exceptions:    []*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, ptr("asset-freeze"), nil, nil)},
			wantAppliedID: "", wantGranted: false, wantLoaderCalls: 1,
		},
		{
			name: "store error => no grant, no abort", side: "from", code: "PIX_IN",
			blocked: true, allowSend: true, allowRecv: true, balanceKey: "default",
			exceptions:    nil,
			loaderErr:     errors.New("redis down"),
			wantAppliedID: "", wantGranted: false, wantLoaderCalls: 1,
		},
		{
			name: "no operational type code => zero I/O", side: "from", code: "",
			blocked: true, allowSend: true, allowRecv: true, balanceKey: "default",
			exceptions:    []*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, nil, nil, nil)},
			wantAppliedID: "", wantGranted: false, wantLoaderCalls: 0,
		},
		{
			name: "no would-be-deny side => zero I/O", side: "from", code: "PIX_IN",
			blocked: false, allowSend: true, allowRecv: true, balanceKey: "default",
			exceptions:    []*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, nil, nil, nil)},
			wantAppliedID: "", wantGranted: false, wantLoaderCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				validate *mtransaction.Responses
				key      string
			)

			if tt.side == "from" {
				validate, key = fromValidate("@acc", tt.balanceKey)
			} else {
				validate, key = toValidate("@acc", tt.balanceKey)
			}

			bal := exceptionEnrichBalance(acc, "@acc", tt.balanceKey, tt.blocked, tt.allowSend, tt.allowRecv)

			loader, calls := countingLoader(tt.exceptions, tt.loaderErr)

			applied := enrichAccountExceptionGrants(context.Background(), loader, nil, org, ledger, tt.code, validate, []*mmodel.Balance{bal})

			assert.Equal(t, tt.wantLoaderCalls, *calls, "loader invocation count")

			if tt.wantAppliedID == "" {
				assert.Nil(t, applied, "expected nil appliedExceptionID")
			} else {
				require.NotNil(t, applied)
				assert.Equal(t, tt.wantAppliedID, *applied)
			}

			var got mtransaction.Amount
			if tt.side == "from" {
				got = validate.From[key]
			} else {
				got = validate.To[key]
			}

			assert.Equal(t, tt.wantGranted, got.BlockBypassGranted, "grant flag on matched side")

			if tt.wantGranted {
				assert.Equal(t, tt.wantAppliedID, got.GrantedExceptionID)
			} else {
				assert.Empty(t, got.GrantedExceptionID)
			}
		})
	}
}

// TestEnrichAccountExceptionGrants_MatchByBalanceID proves a From map keyed by
// the balance ID (rather than the alias-key concat form) still matches, and that
// a nil balance in the slice is skipped safely.
func TestEnrichAccountExceptionGrants_MatchByBalanceID(t *testing.T) {
	t.Parallel()

	org := uuid.New()
	ledger := uuid.New()
	acc := uuid.New().String()

	bal := exceptionEnrichBalance(acc, "@acc", "default", true, true, true)

	validate := &mtransaction.Responses{
		Asset:   "BRL",
		From:    map[string]mtransaction.Amount{bal.ID: {Asset: "BRL", Value: decimal.NewFromInt(100), Operation: constant.DEBIT}},
		Aliases: []string{bal.ID},
		Sources: []string{bal.ID},
	}

	loader, calls := countingLoader([]*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, nil, nil, nil)}, nil)

	applied := enrichAccountExceptionGrants(context.Background(), loader, nil, org, ledger, "PIX_IN", validate, []*mmodel.Balance{nil, bal})

	require.NotNil(t, applied)
	assert.Equal(t, "E1", *applied)
	assert.Equal(t, 1, *calls)
	assert.True(t, validate.From[bal.ID].BlockBypassGranted)
}

// TestEnrichAccountExceptionGrants_UnparseableAccountID proves a balance whose
// AccountID is not a UUID fails closed (no grant, no panic) — the loader is never
// reached with an invalid id.
func TestEnrichAccountExceptionGrants_UnparseableAccountID(t *testing.T) {
	t.Parallel()

	org := uuid.New()
	ledger := uuid.New()

	validate, key := fromValidate("@acc", "default")
	bal := exceptionEnrichBalance("not-a-uuid", "@acc", "default", true, true, true)

	loader, calls := countingLoader([]*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, nil, nil, nil)}, nil)

	applied := enrichAccountExceptionGrants(context.Background(), loader, nil, org, ledger, "PIX_IN", validate, []*mmodel.Balance{bal})

	assert.Nil(t, applied, "unparseable account id must fail closed")
	assert.False(t, validate.From[key].BlockBypassGranted)
	assert.Equal(t, 0, *calls, "loader must not be called with an invalid account id")
}

// TestEnrichAccountExceptionGrants_DedupePerAccount proves the loader is called
// ONCE per distinct AccountID even when two would-be-deny sides share the account.
func TestEnrichAccountExceptionGrants_DedupePerAccount(t *testing.T) {
	t.Parallel()

	org := uuid.New()
	ledger := uuid.New()
	acc := uuid.New().String()

	akDefault := mtransaction.AliasKey("@acc", "default")
	akOther := mtransaction.AliasKey("@acc", "other")
	keyDefault := mtransaction.ConcatAlias(0, akDefault)
	keyOther := mtransaction.ConcatAlias(1, akOther)

	validate := &mtransaction.Responses{
		Asset: "BRL",
		From: map[string]mtransaction.Amount{
			keyDefault: {Asset: "BRL", Value: decimal.NewFromInt(100), Operation: constant.DEBIT},
			keyOther:   {Asset: "BRL", Value: decimal.NewFromInt(100), Operation: constant.DEBIT},
		},
		Aliases: []string{akDefault, akOther},
		Sources: []string{akDefault, akOther},
	}

	balances := []*mmodel.Balance{
		exceptionEnrichBalance(acc, "@acc", "default", true, true, true),
		exceptionEnrichBalance(acc, "@acc", "other", true, true, true),
	}

	loader, calls := countingLoader([]*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, nil, nil, nil)}, nil)

	applied := enrichAccountExceptionGrants(context.Background(), loader, nil, org, ledger, "PIX_IN", validate, balances)

	require.NotNil(t, applied)
	assert.Equal(t, 1, *calls, "loader must be called once per distinct AccountID")
	assert.True(t, validate.From[keyDefault].BlockBypassGranted)
	assert.True(t, validate.From[keyOther].BlockBypassGranted)
}

// TestEnrichAccountExceptionGrants_BothSidesBlocked proves two blocked sides on
// distinct accounts both get grants and appliedExceptionID is the FIRST in the
// deterministic sources->destinations order.
func TestEnrichAccountExceptionGrants_BothSidesBlocked(t *testing.T) {
	t.Parallel()

	org := uuid.New()
	ledger := uuid.New()
	srcAcc := uuid.New().String()
	dstAcc := uuid.New().String()

	akSrc := mtransaction.AliasKey("@src", "default")
	akDst := mtransaction.AliasKey("@dst", "default")
	keySrc := mtransaction.ConcatAlias(0, akSrc)
	keyDst := mtransaction.ConcatAlias(1, akDst)

	validate := &mtransaction.Responses{
		Asset:        "BRL",
		From:         map[string]mtransaction.Amount{keySrc: {Asset: "BRL", Value: decimal.NewFromInt(100), Operation: constant.DEBIT}},
		To:           map[string]mtransaction.Amount{keyDst: {Asset: "BRL", Value: decimal.NewFromInt(100), Operation: constant.CREDIT}},
		Aliases:      []string{akSrc, akDst}, // sources first
		Sources:      []string{akSrc},
		Destinations: []string{akDst},
	}

	balances := []*mmodel.Balance{
		exceptionEnrichBalance(srcAcc, "@src", "default", true, true, true),
		exceptionEnrichBalance(dstAcc, "@dst", "default", true, true, true),
	}

	loaderBySrc := func(_ context.Context, _, _, account uuid.UUID) ([]*mmodel.AccountException, error) {
		if account.String() == srcAcc {
			return []*mmodel.AccountException{exc("SRC", []string{"PIX_IN"}, nil, nil, nil)}, nil
		}
		return []*mmodel.AccountException{exc("DST", []string{"PIX_IN"}, nil, nil, nil)}, nil
	}

	applied := enrichAccountExceptionGrants(context.Background(), loaderBySrc, nil, org, ledger, "PIX_IN", validate, balances)

	require.NotNil(t, applied)
	assert.Equal(t, "SRC", *applied, "appliedExceptionID must be the first grant (source side)")
	assert.Equal(t, "SRC", validate.From[keySrc].GrantedExceptionID)
	assert.Equal(t, "DST", validate.To[keyDst].GrantedExceptionID)
	assert.True(t, validate.From[keySrc].BlockBypassGranted)
	assert.True(t, validate.To[keyDst].BlockBypassGranted)
}

// TestEnrichAccountExceptionGrants_FailClosedEmitsStoreErrorMetric proves the
// fail-closed posture: on a loader error the enrichment emits
// account_exception_evaluations_total{result="store_error"}, grants nothing and
// does NOT abort (returns nil, no error).
func TestEnrichAccountExceptionGrants_FailClosedEmitsStoreErrorMetric(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(mp.Meter("exception-enrich-test"), nil)
	require.NoError(t, err)

	org := uuid.New()
	ledger := uuid.New()
	acc := uuid.New().String()

	validate, key := fromValidate("@acc", "default")
	bal := exceptionEnrichBalance(acc, "@acc", "default", true, true, true)

	loader, calls := countingLoader(nil, errors.New("store unavailable"))

	applied := enrichAccountExceptionGrants(context.Background(), loader, factory, org, ledger, "PIX_IN", validate, []*mmodel.Balance{bal})

	assert.Nil(t, applied, "fail-closed => no applied exception")
	assert.False(t, validate.From[key].BlockBypassGranted, "fail-closed => no grant")
	assert.Equal(t, 1, *calls)

	totals := collectExceptionEvalCounters(t, reader)
	assert.Equal(t, int64(1), totals["ledger/store_error"], "store_error must be counted once")
}

// TestEnrichAccountExceptionGrants_GrantedEmitsMetric proves the granted result
// path emits account_exception_evaluations_total{result="granted"}.
func TestEnrichAccountExceptionGrants_GrantedEmitsMetric(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(mp.Meter("exception-enrich-granted-test"), nil)
	require.NoError(t, err)

	org := uuid.New()
	ledger := uuid.New()
	acc := uuid.New().String()

	validate, _ := fromValidate("@acc", "default")
	bal := exceptionEnrichBalance(acc, "@acc", "default", true, true, true)

	loader, _ := countingLoader([]*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, nil, nil, nil)}, nil)

	applied := enrichAccountExceptionGrants(context.Background(), loader, factory, org, ledger, "PIX_IN", validate, []*mmodel.Balance{bal})
	require.NotNil(t, applied)

	totals := collectExceptionEvalCounters(t, reader)
	assert.Equal(t, int64(1), totals["ledger/granted"], "granted must be counted once")
}

func collectExceptionEvalCounters(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	totals := make(map[string]int64)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "account_exception_evaluations_total" {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				comp, _ := dp.Attributes.Value("component")
				res, _ := dp.Attributes.Value("result")
				totals[comp.AsString()+"/"+res.AsString()] = dp.Value
			}
		}
	}

	return totals
}

// ---- composition: producer (enrichment) -> consumer (validators) ------------

// TestEnrichAccountExceptionGrants_Composition_TranspassesValidator is the
// money-path E2E-mocked proof: the grant produced onto validate.From is honored
// by ValidateBalancesRules, which stops rejecting the blocked source with 0502.
func TestEnrichAccountExceptionGrants_Composition_TranspassesValidator(t *testing.T) {
	t.Parallel()

	org := uuid.New()
	ledger := uuid.New()
	acc := uuid.New().String()
	ctx := context.Background()

	validate, key := fromValidate("@acc", "default")
	mmBal := exceptionEnrichBalance(acc, "@acc", "default", true /*blocked*/, true, true)

	txBal := &mtransaction.Balance{
		ID:             mmBal.ID,
		AccountID:      acc,
		Alias:          "@acc",
		Key:            "default",
		AssetCode:      "BRL",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		AccountBlocked: true,
		Direction:      constant.DirectionCredit,
	}

	txInput := mtransaction.Transaction{Send: mtransaction.Send{Asset: "BRL"}}

	// Without a grant the validator denies with 0502.
	denyErr := mtransaction.ValidateBalancesRules(ctx, txInput, *validate, []*mtransaction.Balance{txBal})
	require.Error(t, denyErr)
	assert.Contains(t, denyErr.Error(), constant.ErrAccountBlockedTransactionRestriction.Error(),
		"blocked source must deny 0502 before enrichment")

	// Produce the grant, then re-run the validator: it must now pass.
	loader, _ := countingLoader([]*mmodel.AccountException{exc("E1", []string{"PIX_IN"}, nil, nil, nil)}, nil)
	applied := enrichAccountExceptionGrants(ctx, loader, nil, org, ledger, "PIX_IN", validate, []*mmodel.Balance{mmBal})
	require.NotNil(t, applied)
	require.True(t, validate.From[key].BlockBypassGranted)

	passErr := mtransaction.ValidateBalancesRules(ctx, txInput, *validate, []*mtransaction.Balance{txBal})
	require.NoError(t, passErr, "grant must transpass the 0502 gate for the matched side")
}

// ---- call-site wiring (live source) -----------------------------------------

// TestExceptionEnrichmentCallSiteWiring proves the enrichment is called in
// executeCreateTransaction BETWEEN rejectInternalScopeBalances and
// buildBalanceOperations, and that the persisted record literal carries
// AppliedExceptionID.
func TestExceptionEnrichmentCallSiteWiring(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("transaction_create.go")
	require.NoError(t, err)

	body := string(src)

	callIdx := strings.Index(body, "enrichAccountExceptionGrants(")
	require.GreaterOrEqual(t, callIdx, 0, "enrichAccountExceptionGrants must be called in the create flow")

	rejectIdx := strings.Index(body, "rejectInternalScopeBalances(")
	buildIdx := strings.Index(body, "buildBalanceOperations(")
	require.GreaterOrEqual(t, rejectIdx, 0)
	require.GreaterOrEqual(t, buildIdx, 0)

	assert.Greater(t, callIdx, rejectIdx, "enrichment must run AFTER rejectInternalScopeBalances")
	assert.Less(t, callIdx, buildIdx, "enrichment must run BEFORE buildBalanceOperations")

	assert.Contains(t, body, "AppliedExceptionID:", "record literal must persist AppliedExceptionID")
}

// TestExceptionEnrichmentNotReevaluatedOnStateTransition proves ADR-004:
// commit/cancel state handlers never re-run the enrichment loader — the source
// file for state transitions does not reference the enrichment at all.
func TestExceptionEnrichmentNotReevaluatedOnStateTransition(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("transaction_state_handlers.go")
	require.NoError(t, err)

	assert.NotContains(t, string(src), "enrichAccountExceptionGrants",
		"state transitions must inherit the decision, never re-evaluate the loader")
	assert.NotContains(t, string(src), "GetActiveAccountExceptions",
		"state transitions must not query active account exceptions")
}
