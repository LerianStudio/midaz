// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v7/commons/redis"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libObservability "github.com/LerianStudio/lib-observability/v4"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	http "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// packageScopePageSize is the page size the overlap guard reads its scope with.
const packageScopePageSize = 100

// packageLockOptions bound the wait behind another mutation of the same ledger's
// packages to about two seconds; the read and write the lock spans take milliseconds.
var packageLockOptions = libRedis.LockOptions{
	Expiry:      10 * time.Second,
	Tries:       40,
	RetryDelay:  50 * time.Millisecond,
	DriftFactor: 0.01,
}

// lockPackageScope serializes, across replicas, every package mutation of one
// ledger from its overlap guard to its write: the guard reads before it writes, so
// two mutations in flight would each pass against the other's absence. The
// returned unlock may be called more than once.
func (uc *UseCase) lockPackageScope(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID) (func(), error) {
	if uc.PackageLock == nil {
		return func() {}, nil
	}

	key, err := tmvalkey.GetKeyContext(ctx, "lock:"+packageCacheKey(organizationID, ledgerID))
	if err != nil {
		return nil, err
	}

	handle, acquired, err := uc.PackageLock.TryLockWithOptions(ctx, key, packageLockOptions)
	if err != nil {
		return nil, err
	}

	if !acquired {
		return nil, fmt.Errorf("fee packages of ledger %s: %w", ledgerID, libRedis.ErrLockContended)
	}

	var once sync.Once

	return func() {
		once.Do(func() {
			if errUnlock := handle.Unlock(context.WithoutCancel(ctx)); errUnlock != nil {
				logger.Log(ctx, libLog.LevelWarn, "Failed to release the fee package lock", libLog.Err(errUnlock))
			}
		})
	}, nil
}

// ValidatePackageMaxAndMinAmountRange validating max and min amount range of a package
func (uc *UseCase) ValidatePackageMaxAndMinAmountRange(ctx context.Context, logger libLog.Logger,
	maxAmount, minAmount, transactionRoute string, metadataSelector map[string]string,
	organizationID, ledgerID uuid.UUID,
	segmentID, packageID *uuid.UUID,
) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.package.validate_amount_range")
	defer span.End()

	filterPackage := getFilterPackage(organizationID, ledgerID, segmentID, transactionRoute)

	// Every package in scope is read: a colliding one may sit on any page.
	var packs []*pack.Package

	for filterPackage.Page = 1; ; filterPackage.Page++ {
		page, err := uc.packageRepo.FindList(ctx, filterPackage)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to find package list", err)

			return err
		}

		packs = append(packs, page...)

		if len(page) < filterPackage.Limit {
			break
		}
	}

	if len(packs) > 0 {
		newMaxAmount, errMaxAmount := decimal.NewFromString(maxAmount)
		if errMaxAmount != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to convert max amount to decimal", errMaxAmount)

			return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "package.MaxAmount")
		}

		newMinAmount, errMinAmount := decimal.NewFromString(minAmount)
		if errMinAmount != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to convert min amount to decimal", errMinAmount)

			return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "package.MinAmount")
		}

		// Validate if the account exists on midaz
		for _, p := range packs {
			if packageID == nil || p.ID != *packageID {
				// Validate if all package data equals the new package
				if isSamePackage(p, newMinAmount, newMaxAmount, transactionRoute, segmentID, metadataSelector) {
					err := pkg.ValidateBusinessError(constant.ErrDuplicatePackage, constant.EntityPackage)
					libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Duplicate package detected", err)

					return err
				}

				// Validate if max and min amount of new package is within the range of a package
				if isRangeOverlap(p, newMinAmount, newMaxAmount, transactionRoute, segmentID, metadataSelector) {
					err := pkg.ValidateBusinessError(constant.ErrPackageRange, "")
					libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Package amount range overlap detected", err)

					return err
				}
			}
		}
	}

	return nil
}

func getFilterPackage(organizationID, ledgerID uuid.UUID, segmentID *uuid.UUID, transactionRoute string) http.QueryHeader {
	filter := http.QueryHeader{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Limit:          packageScopePageSize,
	}

	if segmentID != nil {
		filter.SegmentID = *segmentID
	}

	if transactionRoute != "" {
		filter.TransactionRoute = &transactionRoute
	}

	return filter
}

// sameScope reports whether p is scoped exactly as the new package: same route,
// same segment and the same metadata selector. Only packages in one scope can collide.
func sameScope(p *pack.Package, transactionRoute string, segmentID *uuid.UUID, metadataSelector map[string]string) bool {
	if segmentID == nil {
		segmentID = &uuid.Nil
	}

	return p.GetSegmentID() == *segmentID &&
		p.GetTransactionRoute() == transactionRoute &&
		maps.Equal(p.MetadataSelector, metadataSelector)
}

// isSamePackage validating if all package data is equal to the new package
func isSamePackage(p *pack.Package, newMin, newMax decimal.Decimal, transactionRoute string, segmentID *uuid.UUID, metadataSelector map[string]string) bool {
	return sameScope(p, transactionRoute, segmentID, metadataSelector) &&
		p.MaximumAmount.Equal(newMax) &&
		p.MinimumAmount.Equal(newMin)
}

// isRangeOverlap validating if max and min amount of new package is inside the range of a package
func isRangeOverlap(p *pack.Package, newMin, newMax decimal.Decimal, transactionRoute string, segmentID *uuid.UUID, metadataSelector map[string]string) bool {
	return sameScope(p, transactionRoute, segmentID, metadataSelector) &&
		newMin.LessThanOrEqual(p.MaximumAmount) &&
		newMax.GreaterThanOrEqual(p.MinimumAmount)
}
