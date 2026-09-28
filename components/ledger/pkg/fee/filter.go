// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package fee provides utilities for calculating transaction fees based on various rules and package configurations.
package fee

import (
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// FindPackageToCalculateFee returns the Package to calculate Fee, nil when none
// applies, or an error when several apply at the same specificity.
//
// A package applies only when every scope constraint it carries matches the
// transaction and the amount is inside its band; a constraint it does not carry
// constrains nothing. Both callers re-check the band on whatever comes back.
//
// Among the packages that apply, the one carrying the most constraints wins.
// Packages tied on that count are ambiguous and the transaction is refused
// rather than charged an arbitrary one of them.
func FindPackageToCalculateFee(packages []*pack.Package, transactionRoute string,
	segmentID *uuid.UUID, metadata map[string]any, amount decimal.Decimal,
) (*pack.Package, error) {
	byRoute := filterByTransactionRoute(packages, transactionRoute)
	bySegment := filterBySegmentID(byRoute, segmentID)
	bySelector := filterByMetadataSelector(bySegment, metadata)
	survivors := preferMostSpecific(filterByAmount(bySelector, amount))

	switch len(survivors) {
	case 0:
		return nil, nil
	case 1:
		return survivors[0], nil
	default:
		return nil, newAmbiguousPackagesError(survivors)
	}
}

// AmbiguousPackagesError reports that several stored packages matched a payment
// on every constraint each of them carries, at the same specificity. None can be
// charged without picking one arbitrarily, so the payment is refused, and the
// ids of the packages that tied travel with the refusal: they are what an
// operator has to re-scope, and recovering them by hand means replaying the
// selection against every package on the ledger.
type AmbiguousPackagesError struct {
	// PackageIDs holds the ids of the packages that tied, in the order storage
	// returned them.
	PackageIDs []string
}

// Error implements the error interface.
func (e AmbiguousPackagesError) Error() string {
	return "more than one package was found: " + strings.Join(e.PackageIDs, ", ")
}

// newAmbiguousPackagesError collects the ids of the packages that tied.
func newAmbiguousPackagesError(survivors []*pack.Package) error {
	ids := make([]string, 0, len(survivors))
	for _, packValue := range survivors {
		ids = append(ids, packValue.ID.String())
	}

	return AmbiguousPackagesError{PackageIDs: ids}
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

// preferMostSpecific keeps, among packages that all match the transaction, the
// ones carrying the most constraints; the caller refuses the transaction when
// more than one is left, since ranking one dimension above another would be a
// pricing decision nobody has taken.
//
// It runs on the survivors of every filter, so counting the constraints a
// package carries counts the constraints it matched. Run before the amount
// filter it would let an out-of-band specific package shut out the unrestricted
// one and leave a chargeable transaction uncharged.
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
// route, one for a segment, one per metadata pair. A package restricted to
// nothing carries none and is the least specific of any set that all match.
func constraintsCarried(packValue *pack.Package) int {
	carried := len(packValue.MetadataSelector)

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

// filterByMetadataSelector keeps a package carrying no selector, and one carrying
// a selector only when the transaction metadata holds every declared pair.
func filterByMetadataSelector(packages []*pack.Package, metadata map[string]any) []*pack.Package {
	var filtered []*pack.Package

	for _, packValue := range packages {
		if carriesEveryPair(metadata, packValue.MetadataSelector) {
			filtered = append(filtered, packValue)
		}
	}

	return filtered
}

// carriesEveryPair reports whether metadata holds every selector pair, the
// metadata value compared by its string form.
func carriesEveryPair(metadata map[string]any, selector map[string]string) bool {
	for key, want := range selector {
		got, ok := metadata[key]
		if !ok || fmt.Sprint(got) != want {
			return false
		}
	}

	return true
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
