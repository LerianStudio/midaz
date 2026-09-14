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
// and amount, and a package carrying a segment constraint is never selected
// without that constraint being checked.
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
// Each case derives the selector argument from the canonical route identifier
// the payment carries, the one value the fee service scopes on. What this table
// does NOT guard is the two production call sites that read it: scoping those on
// the deprecated route string instead leaves this whole package green. They are
// guarded one stage out, by TestCalculateFee_RouteScoping and
// TestCalculateFee_DeprecatedRouteStringCarriesNoFeeScope in
// components/ledger/internal/services/fees/calculate-fee_test.go.
//
// A package carrying no segment constraint is charged on a payment whose source
// resolves into a segment, routed and unrouted alike, because a constraint a
// package does not carry constrains nothing. Rows below pin that, because it is
// the fee a client running one unrestricted package is already charged, and
// they pin the rule it generalises to: when several packages match everything
// they carry, the one matching the most constraints is charged, and an equal
// count refuses the payment rather than charging whichever one storage returned
// first.
//
// The rows that carry a behaviour change against origin/develop name what
// origin/develop does on that shape, measured rather than assumed, because a
// comment claiming parity where there is none is how the decision gets reversed
// by accident.
func TestFindPackageToCalculateFee_RouteScoping(t *testing.T) {
	t.Parallel()

	routeID := uuid.NewString()
	segX := uuid.New()
	segY := uuid.New()
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
	unscopedTwin := &pack.Package{ID: uuid.New(), MinimumAmount: min0, MaximumAmount: max}
	segmentScoped := &pack.Package{ID: uuid.New(), SegmentID: uuidPtr(segX), MinimumAmount: min0, MaximumAmount: max}
	otherSegmentScoped := &pack.Package{ID: uuid.New(), SegmentID: uuidPtr(segY), MinimumAmount: min0, MaximumAmount: max}
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
			// A package outside its own amount band is not selected, even
			// when it is the only one the route filter leaves standing. The
			// ledger used to hand that lone survivor back unfiltered and lean
			// on both callers re-checking the band; the band filter now runs
			// on it like any other package, and the callers still re-check.
			// The money was the same either way: the seam row
			// a_package_restricted_to_this_route_is_not_charged_outside_its_own_amount_band
			// in components/ledger/internal/services/fees pins it.
			name:     "a route-scoped package outside its own amount band is not selected",
			packages: []*pack.Package{outOfBand},
			payment:  routed,
			want:     nil,
		},
		{
			// The order the filters run in is money. The amount band must be
			// applied BEFORE the specificity tiebreak: the package restricted to
			// this route matches the route, but the payment falls outside the
			// band its client configured, so it is not a candidate at all and the
			// unrestricted package is charged. Ranked first and filtered after,
			// the out-of-band package would win the tiebreak, be dropped by the
			// band, and the payment would be charged nothing.
			name:     "the unrestricted package is charged when the route-scoped one is out of its own band",
			packages: []*pack.Package{unscoped, outOfBand},
			payment:  routed,
			want:     unscoped,
		},
		{
			// The package carries no segment constraint, so it survives the
			// segment filter on a payment whose source carries a segment and
			// is charged. Charging it is what this repair adds: on
			// origin/develop the same package is charged nothing, because the
			// payment route never reached the route filter and the package was
			// dropped there.
			name:      "a route-scoped package alone is charged on a payment carrying a segment",
			packages:  []*pack.Package{routeScoped},
			payment:   routed,
			segmentID: uuidPtr(segX),
			want:      routeScoped,
		},
		{
			// The money the ledger moves today and this repair must not touch:
			// a client holding one package with no segment constraint is
			// charged on a payment whose source resolves into a segment, on the
			// legacy payment carrying no route identifier at all...
			name:      "a package restricted to nothing is charged on an unrouted payment carrying a segment",
			packages:  []*pack.Package{unscoped},
			payment:   unrouted,
			segmentID: uuidPtr(segX),
			want:      unscoped,
		},
		{
			// ...and on a routed one.
			name:      "a package restricted to nothing is charged on a routed payment carrying a segment",
			packages:  []*pack.Package{unscoped},
			payment:   routed,
			segmentID: uuidPtr(segX),
			want:      unscoped,
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
			// The segment dimension now answers the constraint question the
			// way the route dimension does: a package carrying no segment
			// constraint applies to any segment, so it is no longer dropped
			// here, and the specificity rule picks the package a client
			// restricted to this route over the one they restricted to
			// nothing.
			//
			// origin/develop charges a package on this shape either way it is
			// read. Handed the empty route string its production caller
			// passes, its route filter drops the route-scoped package, leaves
			// the unrestricted one standing alone and hands it back. Handed
			// the payment route instead, it drops the unrestricted one and
			// hands back the route-scoped one. It never charges nothing here,
			// and this branch charged nothing here until this rule landed.
			name:      "the route-scoped package is charged beside an unrestricted one on a segmented payment",
			packages:  []*pack.Package{unscoped, routeScoped},
			payment:   routed,
			segmentID: uuidPtr(segX),
			want:      routeScoped,
		},
		{
			// The segment half of the specificity rule: a package restricted
			// to this payment segment matches one more constraint than a
			// package restricted to nothing, so it is the one charged.
			name:      "the package scoped to this segment wins over the unrestricted one",
			packages:  []*pack.Package{unscoped, segmentScoped},
			payment:   unrouted,
			segmentID: uuidPtr(segX),
			want:      segmentScoped,
		},
		{
			// A package restricted to a DIFFERENT segment carries a
			// constraint the payment does not match, so it is dropped, and
			// the package restricted to nothing is charged. origin/develop
			// charges nothing on this shape: its segment filter drops both.
			name:      "the unrestricted package is charged when the only segment-scoped one belongs elsewhere",
			packages:  []*pack.Package{unscoped, otherSegmentScoped},
			payment:   unrouted,
			segmentID: uuidPtr(segX),
			want:      unscoped,
		},
		{
			// Two packages restricted to nothing are as ambiguous on a
			// segmented payment as they are on an unsegmented one, so the
			// payment is refused rather than charged an arbitrary one of the
			// two. origin/develop charges nothing here instead, because its
			// segment filter drops both: the refusal is a behaviour change
			// this rule brings and it is pinned rather than discovered.
			name:      "two unrestricted packages refuse a segmented payment",
			packages:  []*pack.Package{unscoped, unscopedTwin},
			payment:   unrouted,
			segmentID: uuidPtr(segX),
			wantErr:   true,
		},
		{
			// Specificity counts constraints matched: the package restricted
			// to this route AND this segment matches two, the one restricted
			// to the route alone matches one.
			name:      "the route-and-segment package wins over the route-only one",
			packages:  []*pack.Package{routeScoped, routeAndSegmentScoped},
			payment:   routed,
			segmentID: uuidPtr(segX),
			want:      routeAndSegmentScoped,
		},
		{
			// One constraint each and both matched: nothing separates a
			// package restricted to this route from one restricted to this
			// segment, so the payment is refused rather than charged
			// whichever of the two storage happened to return first. Ranking
			// the route dimension above the segment dimension would be a
			// pricing decision nobody has made.
			name:      "a route-scoped and a segment-scoped package are equally specific and refuse the payment",
			packages:  []*pack.Package{routeScoped, segmentScoped},
			payment:   routed,
			segmentID: uuidPtr(segX),
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Derived from the canonical route identifier the payment carries,
			// the only route value the fee service scopes on, so a change that
			// scopes on anything else fails the routed cases here.
			var routeOfPayment string
			if tc.payment.RouteID != nil {
				routeOfPayment = *tc.payment.RouteID
			}

			got, err := FindPackageToCalculateFee(tc.packages, routeOfPayment, tc.segmentID, amount)

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
