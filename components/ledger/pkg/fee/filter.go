// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package fee provides utilities for calculating transaction fees based on various rules and package configurations.
package fee

import (
	"errors"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// FindPackageToCalculateFee returns the Package to calculate Fee or an error if not exactly one Package is found.
//
// Scope is an AND of route, segment, and amount: a package applies only when
// every constraint it carries matches the transaction, and a constraint it does
// not carry constrains nothing. A package scoped to no route applies on every
// route and a package scoped to no segment applies in every segment.
//
// Every package runs every filter, so nothing is selected on a constraint that
// was not checked. Both callers re-check the amount band on whatever comes
// back.
//
// When more than one package still matches after all three filters, the most
// specific one wins: the package matching the most constraints is charged, so
// route and segment beats route alone or segment alone, and either beats a
// package restricted to nothing. Packages matching the same number of
// constraints are genuinely ambiguous, and the transaction is refused rather
// than charged an arbitrary one of them.
func FindPackageToCalculateFee(packages []*pack.Package, transactionRoute string,
	segmentID *uuid.UUID, amount decimal.Decimal,
) (*pack.Package, error) {
	byRoute := filterByTransactionRoute(packages, transactionRoute)
	bySegment := filterBySegmentID(byRoute, segmentID)
	survivors := preferMostSpecific(filterByAmount(bySegment, amount))

	switch len(survivors) {
	case 0:
		return nil, nil
	case 1:
		return survivors[0], nil
	default:
		return nil, errors.New("more than one package was found")
	}
}

// filterByTransactionRoute Filters the packages by transaction route.
//
// A package applies only when every constraint it carries matches the
// transaction, so a package carrying no route constraint survives whatever
// route the transaction carries, including none, and a package carrying one
// survives an exact match only.
//
// Carrying no route constraint means an empty stored route as well as an absent
// one: the create contract accepts a blank transaction route and stores it, so
// the packages clients saved without choosing a route hold the empty string,
// and they applied to every transaction before route selection was repaired.
func filterByTransactionRoute(packages []*pack.Package, transactionRoute string) []*pack.Package {
	var filtered []*pack.Package

	for _, packValue := range packages {
		packageRoute := packValue.GetTransactionRoute()
		if packageRoute == "" || packageRoute == transactionRoute {
			filtered = append(filtered, packValue)
		}
	}

	return filtered
}

// preferMostSpecific resolves a collision between packages that all match the
// transaction: the most specific one wins, so a package a client restricted to
// this transaction route and this segment is charged rather than one restricted
// to the route alone, and either is charged rather than one restricted to
// nothing.
//
// It runs on the survivors of every filter, and nowhere else. Every survivor
// matches every constraint it carries by then, so counting the constraints a
// package carries counts the constraints it matched. Applied at the route stage
// instead, a package scoped to this route AND to a segment would shut the
// unrestricted package out before the segment filter drops it for a segment the
// transaction does not carry, leaving no package selected and no fee applied to
// a transaction that should have been charged one.
//
// Packages tied on the count stay tied: a package scoped to this route and one
// scoped to this segment are equally specific, as are two packages scoped to the
// same route, and the caller refuses the transaction rather than charging
// whichever one storage returned first. Ranking one dimension above the other
// would be a pricing decision nobody has taken.
//
// A package holding an empty stored route carries no route constraint, matching
// the route filter, so it never outranks a package holding no route at all.
func preferMostSpecific(survivors []*pack.Package) []*pack.Package {
	best := 0

	for _, packValue := range survivors {
		if carried := constraintsCarried(packValue); carried > best {
			best = carried
		}
	}

	kept := make([]*pack.Package, 0, len(survivors))

	for _, packValue := range survivors {
		if constraintsCarried(packValue) == best {
			kept = append(kept, packValue)
		}
	}

	return kept
}

// constraintsCarried counts the scope constraints a package carries: one for a
// transaction route it is restricted to, one for a segment. A package
// restricted to nothing carries none and applies everywhere, which is what
// makes it the least specific of any set of packages that all match.
func constraintsCarried(packValue *pack.Package) int {
	carried := 0

	if packValue.GetTransactionRoute() != "" {
		carried++
	}

	if packValue.SegmentID != nil {
		carried++
	}

	return carried
}

// filterBySegmentID filters the packages by segment id.
//
// A package carrying no segment constraint applies in every segment, which is
// the rule the route filter applies on its own dimension: a constraint a package
// does not carry constrains nothing. So it survives whatever segment the
// transaction carries, including none. A package carrying a segment constraint
// survives an exact match only, so no package is ever selected on a segment that
// was not checked.
func filterBySegmentID(packages []*pack.Package, segmentID *uuid.UUID) []*pack.Package {
	var filtered []*pack.Package

	for _, packValue := range packages {
		if packValue.SegmentID == nil {
			filtered = append(filtered, packValue)

			continue
		}

		if segmentID != nil && *segmentID == *packValue.SegmentID {
			filtered = append(filtered, packValue)
		}
	}

	return filtered
}

// filterByAmount Filters the packages by amount
func filterByAmount(packages []*pack.Package, amount decimal.Decimal) []*pack.Package {
	var filtered []*pack.Package

	for _, packValue := range packages {
		if isTransactionValueBetweenMaxAndMinAmountPackage(*packValue, amount) {
			filtered = append(filtered, packValue)
		}
	}

	return filtered
}

// isTransactionValueBetweenMaxAndMinAmountPackage checks if the transaction value is between the max and min amount of the package
func isTransactionValueBetweenMaxAndMinAmountPackage(p pack.Package, amount decimal.Decimal) bool {
	return amount.GreaterThanOrEqual(p.MinimumAmount) && amount.LessThanOrEqual(p.MaximumAmount)
}
