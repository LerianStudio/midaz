// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	libZap "github.com/LerianStudio/lib-observability/zap"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingResolver is a feeshared.MidazResolver test double that records, per
// alias, how many times GetAccountByAlias was invoked. It lets the cache tests
// assert that a distinct alias is resolved at most once per CalculateFee call.
type countingResolver struct {
	// segmentByAlias maps an account alias to the segment ID the resolver should
	// report. A missing alias resolves to a nil account (resolved-absent).
	segmentByAlias map[string]*uuid.UUID
	// perAliasCalls counts GetAccountByAlias invocations keyed by alias.
	perAliasCalls map[string]int
}

func newCountingResolver(segmentByAlias map[string]*uuid.UUID) *countingResolver {
	return &countingResolver{
		segmentByAlias: segmentByAlias,
		perAliasCalls:  make(map[string]int),
	}
}

func (r *countingResolver) GetAccountByAlias(
	_ context.Context,
	_, _ uuid.UUID,
	alias string,
) (*feeshared.Account, error) {
	r.perAliasCalls[alias]++

	segID, ok := r.segmentByAlias[alias]
	if !ok {
		return nil, nil
	}

	return &feeshared.Account{
		ID:        alias,
		Alias:     alias,
		SegmentID: segID,
	}, nil
}

func (r *countingResolver) AccountExistsByAlias(_ context.Context, _, _ uuid.UUID, _ string) error {
	return nil
}

func (r *countingResolver) ListAccounts(_ context.Context, _, _ uuid.UUID, _, _ *uuid.UUID) ([]feeshared.Account, error) {
	return nil, nil
}

func (r *countingResolver) CountTransactionsByRoute(_ context.Context, _, _ uuid.UUID, _, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}

func (r *countingResolver) maxPerAliasCalls() int {
	max := 0
	for _, n := range r.perAliasCalls {
		if n > max {
			max = n
		}
	}

	return max
}

var _ feeshared.MidazResolver = (*countingResolver)(nil)

// buildSegmentFeeScenario assembles a CalculateFee scenario carrying a
// segment:<uuid> waiver and several payer accounts on both the source and
// destination sides. The same alias set is checked across every distribution
// pass (allAccountsExempt source + destination, findMaxAccount, and the two
// loops in calculateProportionalFees), so a uncached run resolves each alias
// multiple times.
func buildSegmentFeeScenario() (*model.FeeCalculate, *pack.Package, *transaction.Responses, uuid.UUID) {
	waivedSeg := uuid.MustParse("dddddddd-0000-0000-0000-000000000001")

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						AccountAlias: "payer-1",
						Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						AccountAlias: "payee-1",
						Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	isDeductible := false
	fee := model.Fee{
		FeeLabel: "PercentFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: feeconstant.AppRulePercentual,
			Calculations: []model.Calculation{{
				Type:  feeconstant.FeeTypePercentage,
				Value: "10",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &isDeductible,
		CreditAccount:    "@fee_account",
	}

	feePackage := &pack.Package{
		ID:             uuid.New(),
		Fees:           map[string]model.Fee{"percent": fee},
		WaivedAccounts: &[]string{"segment:" + waivedSeg.String()},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{
			"payer-1": {Asset: "BRL", Value: decimal.NewFromInt(600)},
			"payer-2": {Asset: "BRL", Value: decimal.NewFromInt(400)},
		},
		To: map[string]transaction.Amount{
			"payee-1": {Asset: "BRL", Value: decimal.NewFromInt(700)},
			"payee-2": {Asset: "BRL", Value: decimal.NewFromInt(300)},
		},
	}

	return feeCalc, feePackage, resp, waivedSeg
}

// runScenario executes one CalculateFee over a fresh copy of the scenario and
// returns the resolver (for call-count assertions) and the resulting fee legs
// (for balance-invariant comparison). When useCache is true the SegmentContext
// carries a non-nil per-call ResolverCache.
func runScenario(t *testing.T, useCache bool) (*countingResolver, *transaction.Responses, *model.FeeCalculate) {
	t.Helper()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc, feePackage, resp, _ := buildSegmentFeeScenario()

	// No payer belongs to the waived segment in this scenario, so every account
	// is non-exempt: the fee applies and every alias is resolved at least once.
	resolver := newCountingResolver(map[string]*uuid.UUID{})

	segCtx := &SegmentContext{
		Ctx:            context.Background(),
		Resolver:       resolver,
		OrganizationID: testOrgID,
		LedgerID:       testLedgerID,
	}
	if useCache {
		segCtx.ResolverCache = make(map[string]*feeshared.Account)
	}

	err := CalculateFee(logger, feeCalc, feePackage, resp, "BRL", segCtx)
	require.NoError(t, err)

	return resolver, resp, feeCalc
}

// TestCalculateFee_SegmentCache_ResolvesEachAliasOnce proves that with a per-call
// ResolverCache, each distinct account alias is resolved AT MOST once across the
// whole CalculateFee call, even though exemption is checked in multiple passes.
func TestCalculateFee_SegmentCache_ResolvesEachAliasOnce(t *testing.T) {
	t.Parallel()

	resolver, _, _ := runScenario(t, true)

	// Every distinct alias must have been resolved at most once.
	assert.LessOrEqual(t, resolver.maxPerAliasCalls(), 1,
		"with the per-call cache, no alias may be resolved more than once; per-alias counts=%v", resolver.perAliasCalls)

	// And the payer aliases (which are non-exempt and therefore resolved) were
	// in fact resolved — the cache must not suppress the first resolution.
	assert.Equal(t, 1, resolver.perAliasCalls["payer-1"], "payer-1 must resolve exactly once")
	assert.Equal(t, 1, resolver.perAliasCalls["payer-2"], "payer-2 must resolve exactly once")
}

// TestCalculateFee_SegmentCache_WithoutCacheResolvesRepeatedly is the control:
// without a cache the SAME alias is resolved multiple times across the passes,
// which is the redundant N+1 the cache eliminates. This pins the regression so a
// future change that silently drops the cache is caught.
func TestCalculateFee_SegmentCache_WithoutCacheResolvesRepeatedly(t *testing.T) {
	t.Parallel()

	resolver, _, _ := runScenario(t, false)

	assert.Greater(t, resolver.maxPerAliasCalls(), 1,
		"without a cache at least one alias must be resolved more than once (the redundancy the cache removes); per-alias counts=%v", resolver.perAliasCalls)
}

// TestCalculateFee_SegmentCache_BalanceUnchanged is the double-entry guard: the
// cache is a pure I/O optimization, so the produced fee legs and the mutated
// Send.Value MUST be byte-for-byte identical with and without the cache.
func TestCalculateFee_SegmentCache_BalanceUnchanged(t *testing.T) {
	t.Parallel()

	_, respCached, feeCalcCached := runScenario(t, true)
	_, respUncached, feeCalcUncached := runScenario(t, false)

	assert.True(t, feeCalcCached.Transaction.Send.Value.Equal(feeCalcUncached.Transaction.Send.Value),
		"Send.Value must be identical with and without cache: cached=%s uncached=%s",
		feeCalcCached.Transaction.Send.Value.String(), feeCalcUncached.Transaction.Send.Value.String())

	assertSameLegs(t, "From", respCached.From, respUncached.From)
	assertSameLegs(t, "To", respCached.To, respUncached.To)
}

// assertSameLegs asserts two fee-leg maps carry the same keys and exactly-equal
// decimal amounts under the same asset (the conservation invariant the ledger
// validator enforces).
func assertSameLegs(t *testing.T, side string, got, want map[string]transaction.Amount) {
	t.Helper()

	assert.Equal(t, len(want), len(got), "%s: leg count must match", side)

	for key, wantAmt := range want {
		gotAmt, ok := got[key]
		if !assert.True(t, ok, "%s: leg %q present in uncached run must exist in cached run", side, key) {
			continue
		}

		assert.Equal(t, wantAmt.Asset, gotAmt.Asset, "%s: leg %q asset must match", side, key)
		assert.True(t, wantAmt.Value.Equal(gotAmt.Value),
			"%s: leg %q value must be exactly equal: want=%s got=%s",
			side, key, wantAmt.Value.String(), gotAmt.Value.String())
	}
}
