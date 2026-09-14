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
// every constraint it carries matches the transaction.
//
// One package left standing by the route filter, carrying no segment
// constraint, is returned there and then: the segment and amount filters do not
// run on it. That is what the ledger has always done and it is deliberate here,
// because it is what keeps a client running a single unrestricted package
// charged on a payment whose source resolves into a segment. A lone survivor
// that DOES carry a segment constraint falls through, so no package is ever
// selected on a segment that was not checked. Both callers re-check the amount
// band on whatever comes back, so nothing is charged outside the band its
// client configured.
//
// When more than one package still matches after all three filters and the
// specificity tiebreak, the transaction is refused rather than charged an
// arbitrary one of them.
func FindPackageToCalculateFee(packages []*pack.Package, transactionRoute string,
	segmentID *uuid.UUID, amount decimal.Decimal,
) (*pack.Package, error) {
	byRoute := filterByTransactionRoute(packages, transactionRoute)
	if len(byRoute) == 1 && byRoute[0].SegmentID == nil {
		return byRoute[0], nil
	}

	bySegment := filterBySegmentID(byRoute, segmentID)
	survivors := preferMostSpecificRoute(filterByAmount(bySegment, amount))

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

// preferMostSpecificRoute resolves a collision between packages that all match
// the transaction: the most specific one wins, so a package a client restricted
// to this transaction route is charged rather than one the client restricted to
// nothing. With no route-restricted package among the survivors it changes
// nothing.
//
// It runs on the survivors of every filter, and nowhere else. Applied at the
// route stage instead, a package scoped to this route AND to a segment would
// shut the unrestricted package out before the segment filter drops it for a
// segment the transaction does not carry, leaving no package selected and no
// fee applied to a transaction that should have been charged one.
//
// It separates route-restricted packages from unrestricted ones and ranks
// nothing beyond that, so two packages a client restricted to the SAME route
// remain equally specific and the transaction is refused by the caller. That
// refusal is newly reachable: before route selection was repaired both such
// packages were dropped and the transaction posted with no fee at all.
//
// A package holding an empty stored route is unrestricted, matching the route
// filter, so it never outranks a package holding no route at all.
func preferMostSpecificRoute(survivors []*pack.Package) []*pack.Package {
	var routeScoped []*pack.Package

	for _, packValue := range survivors {
		if packValue.GetTransactionRoute() != "" {
			routeScoped = append(routeScoped, packValue)
		}
	}

	if routeScoped == nil {
		return survivors
	}

	return routeScoped
}

// filterBySegmentID Filters the packages by segment id
func filterBySegmentID(packages []*pack.Package, segmentID *uuid.UUID) []*pack.Package {
	var filtered []*pack.Package

	for _, packValue := range packages {
		if segmentID == nil && packValue.SegmentID != nil {
			continue
		}

		if segmentID == nil && packValue.SegmentID == nil {
			filtered = append(filtered, packValue)
			continue
		}

		if segmentID != nil && packValue.SegmentID != nil {
			if *segmentID == *packValue.SegmentID {
				filtered = append(filtered, packValue)
			}
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
