// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"fmt"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// TestFindPackageToCalculateFee_ShapeCorpus is the money-safety net under fee
// package selection: it enumerates every ledger a client can build out of three
// stored packages and every payment shape that can arrive, and holds the
// selector to three invariants on each one.
//
// The named tables beside it pin the shapes a person thought to write down. This
// one pins the rule those shapes are examples of, so a change that keeps every
// named row green while withdrawing a charge somewhere nobody enumerated goes
// red here instead of in production. The filter ORDER is the concrete case:
// ranking the most specific package before the amount band rather than after it
// keeps every named table in this package green while withdrawing the charge on
// 422 of the 7,974 shapes below, measured against that mutant.
//
// The corpus is generated rather than transcribed: stored packages come in 18
// kinds, route (absent, blank, one route) by segment (absent, one segment, a
// second) by band (payment inside, payment outside), and a ledger is any
// multiset of one to three of them, which is 1,329 ledgers. Each is driven with
// the 6 payment shapes, route (the one route, none) by segment (none, the first,
// the second), for 7,974 shapes in all.
//
// The outcome measured is the selector plus what both fee-service call sites do
// with its answer, which is to re-check the amount band on whatever comes back.
// That re-check is asserted to change nothing: the selector may never hand back
// a package the payment is outside the band of, so the guard the callers keep is
// a second lock on a door already shut rather than the only one.
//
// The invariants, in the order a charge is decided:
//
//  1. No silent nothing. When the ledger holds a package matching every
//     constraint it carries with the payment inside its band, the payment is
//     either charged a package or refused as ambiguous. It is never posted free
//     of charge.
//  2. No charge out of scope. A charged package always has the payment inside
//     its band and matches every constraint it carries.
//  3. The most specific package wins, and a tie refuses. The charged package
//     carries the highest constraint count among the candidates and is the only
//     one carrying it; when the ledger holds candidates and nothing is charged,
//     at least two of them are tied at that count.
func TestFindPackageToCalculateFee_ShapeCorpus(t *testing.T) {
	t.Parallel()

	routeR := uuid.NewString()
	segS := uuid.New()
	segT := uuid.New()
	amount := decimal.NewFromInt(100)

	inBandMin, inBandMax := decimal.Zero, decimal.NewFromInt(1_000_000)
	outBandMin, outBandMax := decimal.NewFromInt(1_000), decimal.NewFromInt(5_000)

	blankRoute := ""

	routes := []struct {
		label string
		value *string
	}{
		{"routeAbsent", nil},
		{"routeBlank", &blankRoute},
		{"routeR", &routeR},
	}

	segments := []struct {
		label string
		value *uuid.UUID
	}{
		{"segmentAbsent", nil},
		{"segmentS", &segS},
		{"segmentT", &segT},
	}

	bands := []struct {
		label  string
		inBand bool
	}{
		{"inBand", true},
		{"outOfBand", false},
	}

	type storedKind struct {
		label   string
		route   *string
		segment *uuid.UUID
		inBand  bool
	}

	var kinds []storedKind

	for _, r := range routes {
		for _, s := range segments {
			for _, b := range bands {
				kinds = append(kinds, storedKind{
					label:   r.label + "/" + s.label + "/" + b.label,
					route:   r.value,
					segment: s.value,
					inBand:  b.inBand,
				})
			}
		}
	}

	require.Len(t, kinds, 18, "the stored-package kinds must be route x segment x band")

	// A ledger is a MULTISET of kinds, built as a non-decreasing index tuple so
	// two packages of the same kind are enumerated once rather than twice: a
	// client holding two identical packages is one ledger, not two.
	var ledgers [][]int

	for i := range kinds {
		ledgers = append(ledgers, []int{i})

		for j := i; j < len(kinds); j++ {
			ledgers = append(ledgers, []int{i, j})

			for k := j; k < len(kinds); k++ {
				ledgers = append(ledgers, []int{i, j, k})
			}
		}
	}

	require.Len(t, ledgers, 1329, "every multiset of one to three of the 18 kinds must be enumerated")

	payments := []struct {
		label   string
		routeID string
		segment *uuid.UUID
	}{
		{"payment routeR/segmentAbsent", routeR, nil},
		{"payment routeR/segmentS", routeR, &segS},
		{"payment routeR/segmentT", routeR, &segT},
		{"payment routeNone/segmentAbsent", "", nil},
		{"payment routeNone/segmentS", "", &segS},
		{"payment routeNone/segmentT", "", &segT},
	}

	require.Len(t, payments, 6, "the payment shapes must be route x segment")

	shapes := 0

	for _, ledger := range ledgers {
		for _, payment := range payments {
			shapes++

			packages := make([]*pack.Package, len(ledger))
			describe := make([]string, len(ledger))

			for n, idx := range ledger {
				k := kinds[idx]

				minAmount, maxAmount := inBandMin, inBandMax
				if !k.inBand {
					minAmount, maxAmount = outBandMin, outBandMax
				}

				packages[n] = &pack.Package{
					ID:               uuid.New(),
					TransactionRoute: k.route,
					SegmentID:        k.segment,
					MinimumAmount:    minAmount,
					MaximumAmount:    maxAmount,
				}
				describe[n] = k.label
			}

			// The candidate set, recomputed here from the rule rather than read
			// off the production filters: a package is a candidate when the
			// payment is inside its band and it matches every constraint it
			// carries. Restating it is what makes this a test instead of a
			// mirror.
			candidates := make([]int, 0, len(packages))
			topCarried := 0
			tiedAtTop := 0

			for n, idx := range ledger {
				k := kinds[idx]

				if !k.inBand {
					continue
				}

				if k.route != nil && *k.route != "" && *k.route != payment.routeID {
					continue
				}

				if k.segment != nil && (payment.segment == nil || *k.segment != *payment.segment) {
					continue
				}

				candidates = append(candidates, n)
			}

			for _, n := range candidates {
				if carried := constraintsCarried(packages[n]); carried > topCarried {
					topCarried = carried
				}
			}

			for _, n := range candidates {
				if constraintsCarried(packages[n]) == topCarried {
					tiedAtTop++
				}
			}

			shape := fmt.Sprintf("ledger %v vs %s", describe, payment.label)

			got, err := FindPackageToCalculateFee(packages, payment.routeID, payment.segment, amount)

			// What both fee-service call sites do with the answer.
			afterCallerBandCheck := got
			if got != nil && (amount.LessThan(got.MinimumAmount) || amount.GreaterThan(got.MaximumAmount)) {
				afterCallerBandCheck = nil
			}

			require.Equal(t, got, afterCallerBandCheck,
				"%s: the selector handed back a package the payment is outside the band of, and only the callers re-check caught it", shape)

			if len(candidates) == 0 {
				require.Nil(t, got, "%s: no package matches this payment, so none may be charged", shape)
				require.NoError(t, err, "%s: no package matches this payment, so it is not ambiguous", shape)

				continue
			}

			// Invariant 1: a ledger holding a candidate never posts free of charge.
			if got == nil {
				require.Error(t, err,
					"%s: %d package(s) match this payment and none was charged, yet the payment was not refused", shape, len(candidates))
				require.GreaterOrEqual(t, tiedAtTop, 2,
					"%s: the payment was refused as ambiguous with only %d candidate(s) at the top constraint count", shape, tiedAtTop)

				continue
			}

			require.NoError(t, err, "%s: a package was charged and the payment was refused at the same time", shape)

			// Invariant 2: a charged package is inside its band and in scope.
			charged := -1

			for _, n := range candidates {
				if packages[n] == got {
					charged = n

					break
				}
			}

			require.NotEqual(t, -1, charged,
				"%s: the charged package is outside its own band or carries a constraint this payment does not match", shape)

			// Invariant 3: the most specific candidate wins, and only when alone.
			require.Equal(t, topCarried, constraintsCarried(got),
				"%s: the charged package carries %d constraint(s) while a candidate carries %d", shape, constraintsCarried(got), topCarried)
			require.Equal(t, 1, tiedAtTop,
				"%s: a package was charged while %d candidates are tied at the top constraint count", shape, tiedAtTop)
		}
	}

	require.Equal(t, 7974, shapes, "every ledger must be driven with every payment shape")
	t.Logf("selector held to three invariants over %d shapes", shapes)
}
