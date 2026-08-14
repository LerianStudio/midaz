// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// strPtr returns a pointer to s for building scoped Package fixtures.
func strPtr(s string) *string { return &s }

// uuidPtr returns a pointer to id for building scoped Package fixtures.
func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

// TestFindPackageToCalculateFee_Scoping locks the segment- and combined-scope
// semantics fixed in the fee-scoping cluster: scope is an AND of route, segment,
// and amount, and the route-filter short-circuit must not skip an unverified
// segment constraint.
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
			name: "combined route+segment requires BOTH: route matches but segment nil -> no early return, combo dropped",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: strPtr(routeA), SegmentID: uuidPtr(segA), MinimumAmount: min0, MaximumAmount: max},
				{ID: uuid.New(), MinimumAmount: min0, MaximumAmount: max}, // unscoped, but nil-route so dropped by route filter
			},
			route:     routeA,
			segmentID: nil,
			wantNil:   true,
		},
		{
			name: "route-only package still matches on route alone (segment nil)",
			packages: []*pack.Package{
				{ID: uuid.New(), TransactionRoute: strPtr(routeA), MinimumAmount: min0, MaximumAmount: max},
				{ID: uuid.New(), MinimumAmount: min0, MaximumAmount: max}, // unscoped, dropped by route filter
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

			assert.NotNil(t, got)

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
