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
// every constraint it carries matches the transaction. The early returns after
// the route and segment filters are short-circuits that must not skip an
// unverified constraint — a lone route survivor that still carries a segment
// constraint must fall through to the segment filter, otherwise a package scoped
// to route=A AND segment=X would be selected on the route match alone.
func FindPackageToCalculateFee(packages []*pack.Package, transactionRoute string,
	segmentID *uuid.UUID, amount decimal.Decimal,
) (*pack.Package, error) {
	byRoute := filterByTransactionRoute(packages, transactionRoute)
	if len(byRoute) == 1 && byRoute[0].SegmentID == nil {
		return byRoute[0], nil
	}

	bySegment := filterBySegmentID(byRoute, segmentID)
	if len(bySegment) == 1 {
		return bySegment[0], nil
	}

	byAmount := preferMostSpecificRoute(filterByAmount(bySegment, amount))
	if len(byAmount) == 1 {
		return byAmount[0], nil
	} else if byAmount == nil {
		return nil, nil
	}

	return nil, errors.New("more than one package was found")
}

// filterByTransactionRoute Filters the packages by transaction route.
//
// A package applies only when every constraint it carries matches the
// transaction, so a package carrying no route constraint survives whatever
// route the transaction carries, including none, and a package carrying one
// survives an exact match only.
func filterByTransactionRoute(packages []*pack.Package, transactionRoute string) []*pack.Package {
	var filtered []*pack.Package

	for _, packValue := range packages {
		if packValue.TransactionRoute == nil || *packValue.TransactionRoute == transactionRoute {
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
func preferMostSpecificRoute(survivors []*pack.Package) []*pack.Package {
	var routeScoped []*pack.Package

	for _, packValue := range survivors {
		if packValue.TransactionRoute != nil {
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
