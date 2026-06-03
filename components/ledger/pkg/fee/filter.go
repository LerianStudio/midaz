// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package fee provides utilities for calculating transaction fees based on various rules and package configurations.
package fee

import (
	"errors"
	"strings"

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

// isRepeatingDecimal checks if the decimal is repeating
func isRepeatingDecimal(d decimal.Decimal) bool {
	const maxCycleLength = 6

	const minRepeatCount = 2 // Reduced from 3 to 2 for better detection

	// Get the exact decimal representation without trailing zeros
	s := d.String()

	parts := strings.Split(s, ".")
	if len(parts) != 2 {
		return false
	}

	decimals := parts[1]

	// If the decimal part is empty or all zeros, it's not repeating
	if decimals == "" || strings.TrimRight(decimals, "0") == "" {
		return false
	}

	// Remove trailing zeros to avoid false positives
	decimals = strings.TrimRight(decimals, "0")

	// If the decimal part is too short to have a meaningful repeating pattern, return false
	if len(decimals) < minRepeatCount*2 {
		return false
	}

	for size := 1; size <= maxCycleLength && size <= len(decimals)/2; size++ {
		for start := 0; start+size*minRepeatCount <= len(decimals); start++ {
			pattern := decimals[start : start+size]

			// Skip if pattern is all zeros (which would be trailing zeros)
			if strings.TrimRight(pattern, "0") == "" {
				continue
			}

			// Check if this pattern repeats at least minRepeatCount times
			repeatCount := 1

			for i := 1; start+i*size+size <= len(decimals); i++ {
				begin := start + i*size
				end := begin + size

				if decimals[begin:end] == pattern {
					repeatCount++
				} else {
					break
				}
			}

			if repeatCount >= minRepeatCount {
				return true
			}
		}
	}

	return false
}
