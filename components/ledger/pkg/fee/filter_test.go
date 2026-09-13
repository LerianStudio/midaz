// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// strPtr returns a pointer to s for building scoped Package fixtures.
func strPtr(s string) *string { return &s }

// uuidPtr returns a pointer to id for building scoped Package fixtures.
func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

// TestFindPackageToCalculateFee_Scoping locks the segment- and combined-scope
// semantics fixed in the fee-scoping cluster: scope is an AND of route, segment,
// and amount, and no package is selected on a constraint that was not checked.
func TestFindPackageToCalculateFee_Scoping(t *testing.T) {
	t.Parallel()

	routeA := "ROUTE-A"
	segA := uuid.New()
	segB := uuid.New()
	min0 := decimal.Zero
	max := decimal.NewFromInt(1_000_000)
	amount := decimal.NewFromInt(100)

	tests := []struct {
		name          string
		packages      []*pack.Package
		route         string
		segmentID     *uuid.UUID
		wantNil       bool
		wantSegment   *uuid.UUID // asserted on the selected package when wantNil is false
		wantRoute     *string
		expectedError bool
	}{
		{
			name: "unscoped single package still selected (regression guard)",
			packages: []*pack.Package{
				{ID: uuid.New(), MinimumAmount: min0, MaximumAmount: max},
			},
			route:       "",
			segmentID:   nil,
			wantNil:     false,
			wantSegment: nil,
			wantRoute:   nil,
		},
		{
			name: "segment-scoped single package selected when segment matches",
			packages: []*pack.Package{
				{ID: uuid.New(), SegmentID: uuidPtr(segA), MinimumAmount: min0, MaximumAmount: max},
			},
			route:       "",
			segmentID:   uuidPtr(segA),
			wantNil:     false,
			wantSegment: uuidPtr(segA),
		},
		{
			name: "segment-scoped single package NOT selected for nil transaction segment",
			packages: []*pack.Package{
				{ID: uuid.New(), SegmentID: uuidPtr(segA), MinimumAmount: min0, MaximumAmount: max},
			},
			route:     "",
			segmentID: nil,
			wantNil:   true,
		},
		{
			name: "segment-scoped single package NOT selected for a different segment",
			packages: []*pack.Package{
				{ID: uuid.New(), SegmentID: uuidPtr(segA), MinimumAmount: min0, MaximumAmount: max},
			},
			route:     "",
			segmentID: uuidPtr(segB),
			wantNil:   true,
		},
		{
			name: "combined route+segment requires BOTH: matches when both match",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: strPtr(routeA), SegmentID: uuidPtr(segA), MinimumAmount: min0, MaximumAmount: max},
				{ID: uuid.New(), MinimumAmount: min0, MaximumAmount: max}, // unscoped coexisting
			},
			route:       routeA,
			segmentID:   uuidPtr(segA),
			wantNil:     false,
			wantSegment: uuidPtr(segA),
			wantRoute:   strPtr(routeA),
		},
		{
			// The combo package is still dropped for the segment the transaction
			// does not carry, and the unscoped package coexisting with it is now
			// charged instead of nothing being charged at all.
			name: "combined route+segment requires BOTH: route matches but segment nil -> no early return, combo dropped",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: strPtr(routeA), SegmentID: uuidPtr(segA), MinimumAmount: min0, MaximumAmount: max},
				{ID: uuid.New(), MinimumAmount: min0, MaximumAmount: max}, // unscoped: carries no constraint, so it survives any route
			},
			route:       routeA,
			segmentID:   nil,
			wantNil:     false,
			wantSegment: nil,
			wantRoute:   nil,
		},
		{
			name: "route-only package still matches on route alone (segment nil)",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: strPtr(routeA), MinimumAmount: min0, MaximumAmount: max},
				{ID: uuid.New(), MinimumAmount: min0, MaximumAmount: max}, // unscoped, dropped by the specificity tiebreak
			},
			route:       routeA,
			segmentID:   nil,
			wantNil:     false,
			wantSegment: nil,
			wantRoute:   strPtr(routeA),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := FindPackageToCalculateFee(tc.packages, tc.route, tc.segmentID, amount)

			if tc.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tc.wantNil {
				assert.Nil(t, got)
				return
			}

			// require, not assert: the assertions below dereference the selected
			// package, so a regression that selects nothing would panic the whole
			// test binary and hide every other case in this package.
			require.NotNil(t, got)

			if tc.wantSegment == nil {
				assert.Nil(t, got.SegmentID)
			} else {
				assert.NotNil(t, got.SegmentID)
				assert.Equal(t, *tc.wantSegment, *got.SegmentID)
			}

			if tc.wantRoute == nil {
				assert.Nil(t, got.TransactionRoute)
			} else {
				assert.NotNil(t, got.TransactionRoute)
				assert.Equal(t, *tc.wantRoute, *got.TransactionRoute)
			}
		})
	}
}

// TestFindPackageToCalculateFee_RouteScoping pins which fee package is charged
// on a payment that carries the canonical transaction route identifier, the
// only route a payment on the fee-charging create path carries. It holds both
// halves of the money outcome: a package a client restricted to one route must
// be charged on that route, and a package a client restricted to nothing must
// go on being charged on every payment, routed ones included.
//
// Each case derives the selector argument from the payment through the same
// accessor the fee service reads, so reverting that accessor to the deprecated
// route string fails the routed cases here. What this table does NOT guard is
// the two production call sites that read it: reverting those to the deprecated
// field leaves this whole package green. They are guarded one stage out, by
// TestCalculateFee_RouteScoping in
// components/ledger/internal/services/fees/calculate-fee_test.go.
func TestFindPackageToCalculateFee_RouteScoping(t *testing.T) {
	t.Parallel()

	routeID := uuid.NewString()
	segX := uuid.New()
	min0 := decimal.Zero
	max := decimal.NewFromInt(1_000_000)
	amount := decimal.NewFromInt(100)

	// The create pipeline that charges fees populates RouteID only, so this is
	// the shape every chargeable payment reaches the selector with.
	routed := transaction.Transaction{RouteID: &routeID}
	unrouted := transaction.Transaction{}

	routeScoped := &pack.Package{ID: uuid.New(), TransactionRoute: &routeID, MinimumAmount: min0, MaximumAmount: max}
	sameRouteTwin := &pack.Package{ID: uuid.New(), TransactionRoute: &routeID, MinimumAmount: min0, MaximumAmount: max}
	unscoped := &pack.Package{ID: uuid.New(), MinimumAmount: min0, MaximumAmount: max}
	routeAndSegmentScoped := &pack.Package{ID: uuid.New(), TransactionRoute: &routeID, SegmentID: uuidPtr(segX), MinimumAmount: min0, MaximumAmount: max}
	// A package a client saved without choosing a route: the create contract
	// accepts the blank string and stores it, and it must go on applying to
	// every payment exactly as a package carrying no route constraint does.
	blankRoute := &pack.Package{ID: uuid.New(), TransactionRoute: strPtr(""), MinimumAmount: min0, MaximumAmount: max}
	// A route-scoped package whose client set an amount band the 100 payment
	// below falls outside of.
	outOfBand := &pack.Package{ID: uuid.New(), TransactionRoute: &routeID,
		MinimumAmount: decimal.NewFromInt(1_000), MaximumAmount: decimal.NewFromInt(5_000)}

	tests := []struct {
		name      string
		packages  []*pack.Package
		payment   transaction.Transaction
		segmentID *uuid.UUID
		want      *pack.Package
		wantErr   bool
	}{
		{
			// The defect: a client restricts a package to one route and is never charged it.
			name:     "route-scoped package is charged on a payment carrying that route",
			packages: []*pack.Package{routeScoped},
			payment:  routed,
			want:     routeScoped,
		},
		{
			// The regression guard: feeding the canonical route in must not stop
			// charging clients who run one flat package on everything.
			name:     "unscoped package is still charged on a routed payment",
			packages: []*pack.Package{unscoped},
			payment:  routed,
			want:     unscoped,
		},
		{
			// The ceiling: a payment with no route at all keeps behaving as today.
			name:     "route-scoped package is not charged on a payment carrying no route",
			packages: []*pack.Package{routeScoped},
			payment:  unrouted,
			want:     nil,
		},
		{
			// The collision rule: the most specific package wins.
			name:     "the route-scoped package wins over the unscoped one on a routed payment",
			packages: []*pack.Package{unscoped, routeScoped},
			payment:  routed,
			want:     routeScoped,
		},
		{
			// The money hole the specificity tiebreak opens if it runs at the route
			// stage instead of on the packages that survived every filter: the
			// route-scoped package is dropped one stage later for a segment this
			// payment does not carry, and the payment would be charged nothing.
			name:     "unscoped package is charged when the route-scoped one demands a segment the payment does not carry",
			packages: []*pack.Package{routeAndSegmentScoped, unscoped},
			payment:  routed,
			want:     unscoped,
		},
		{
			// The create contract accepts a blank route and stores it, so the
			// packages clients already saved without choosing a route carry one.
			// They applied to every payment before route selection was repaired
			// and they must go on applying to every payment after it.
			name:     "a package saved with a blank route is charged on a routed payment",
			packages: []*pack.Package{blankRoute},
			payment:  routed,
			want:     blankRoute,
		},
		{
			// A blank route is the absence of a route constraint, so it cannot
			// win the specificity tiebreak against a package carrying none: the
			// pair is genuinely ambiguous and the payment is refused rather than
			// charged a fee nobody chose.
			name:     "a package saved with a blank route does not outrank a package carrying no route",
			packages: []*pack.Package{blankRoute, unscoped},
			payment:  unrouted,
			wantErr:  true,
		},
		{
			// Every constraint the package carries is checked, the amount band
			// included, even when the package is the only one left standing.
			name:     "a route-scoped package alone is not charged outside its own amount band",
			packages: []*pack.Package{outOfBand},
			payment:  routed,
			want:     nil,
		},
		{
			// Same rule on the segment dimension: a lone survivor of the route
			// filter still faces the segment filter, which keeps a package only
			// when its segment matches the one the payment's source carries.
			name:      "a route-scoped package alone is not charged on a payment carrying a segment",
			packages:  []*pack.Package{routeScoped},
			payment:   routed,
			segmentID: uuidPtr(segX),
			want:      nil,
		},
		{
			// Two packages a client scoped to the same route are equally
			// specific, so the tiebreak cannot separate them: the payment is
			// refused rather than charged an arbitrary one of the two.
			name:     "two packages scoped to the same route refuse the payment",
			packages: []*pack.Package{routeScoped, sameRouteTwin},
			payment:  routed,
			wantErr:  true,
		},
		{
			// The segment dimension keeps the asymmetry this repair deliberately
			// left alone: a payment whose source carries a segment keeps only
			// packages scoped to that same segment, so a ledger holding none is
			// charged nothing. Pinned so a later change to that rule is a
			// decision rather than an accident.
			name:      "no package is charged on a payment carrying a segment none of them scopes to",
			packages:  []*pack.Package{unscoped, routeScoped},
			payment:   routed,
			segmentID: uuidPtr(segX),
			want:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := FindPackageToCalculateFee(tc.packages, tc.payment.EffectiveRouteID(), tc.segmentID, amount)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)

				return
			}

			assert.NoError(t, err)

			if tc.want == nil {
				assert.Nil(t, got)
				return
			}

			assert.NotNil(t, got)
			assert.Same(t, tc.want, got)
		})
	}
}
