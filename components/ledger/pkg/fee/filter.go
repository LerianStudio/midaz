// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package fee provides utilities for calculating transaction fees based on various rules and package configurations.
package fee

import (
	"errors"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// FindPackageToCalculateFee returns the Package to calculate Fee or an error if not exactly one Package is found
func FindPackageToCalculateFee(packages []*pack.Package, transactionRoute string,
	segmentID *uuid.UUID, amount decimal.Decimal,
) (*pack.Package, error) {
	byRoute := filterByTransactionRoute(packages, transactionRoute)
	if len(byRoute) == 1 {
		return byRoute[0], nil
	}

	bySegment := filterBySegmentID(byRoute, segmentID)
	if len(bySegment) == 1 {
		return bySegment[0], nil
	}

	byAmount := filterByAmount(bySegment, amount)
	if len(byAmount) == 1 {
		return byAmount[0], nil
	} else if byAmount == nil {
		return nil, nil
	}

	return nil, errors.New("more than one package was found")
}

// filterByTransactionRoute Filters the packages by transaction route
func filterByTransactionRoute(packages []*pack.Package, transactionRoute string) []*pack.Package {
	var filtered []*pack.Package

	for _, packValue := range packages {
		if (packValue.TransactionRoute == nil && transactionRoute == "") ||
			(packValue.TransactionRoute != nil && *packValue.TransactionRoute == transactionRoute) {
			filtered = append(filtered, packValue)
		}
	}

	return filtered
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
