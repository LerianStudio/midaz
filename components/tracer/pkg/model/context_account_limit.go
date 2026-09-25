// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ContextAccountLimit is a trusted active limit snapshot with an explicitly
// resolved asset identity. It is not a transport DTO or an inferred mapping from
// a currency code. Definition.ID and account scope keys retain existing usage.
// A repository must preserve unresolved candidates so validation can reject
// missing migration/configuration instead of silently removing a control.
type ContextAccountLimit struct {
	Definition Limit
	Asset      tracercontract.AssetRef
}

// Validate checks the account-only profile without applying legacy ISO asset
// restrictions or silently ignoring unsupported dimensions. Administrative
// labels are not part of runtime selection. Even expired windows must be valid.
func (l ContextAccountLimit) Validate(ctx context.Context, namespace string, bounds tracercontract.Limits, maxScopes int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	d := l.Definition

	if err := bounds.Validate(); err != nil {
		return err
	}

	if d.ID == uuid.Nil || d.Status != LimitStatusActive || d.DeletedAt != nil ||
		!d.LimitType.IsValid() || !d.MaxAmount.IsPositive() {
		return constant.ErrContextLimitsUnavailable
	}

	if err := l.Asset.Validate(namespace, bounds.MaxTextBytes); err != nil {
		return constant.ErrContextLimitsUnavailable
	}

	if d.Asset != l.Asset.Code {
		return constant.ErrContextLimitsUnavailable
	}

	return ValidateContextLimitDefinition(ctx, d, bounds, maxScopes)
}

// ValidateContextLimitDefinition checks shared-profile definitions before
// persistence, including draft limits which have not received an AssetRef yet.
func ValidateContextLimitDefinition(ctx context.Context, d Limit, bounds tracercontract.Limits, maxScopes int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := bounds.Validate(); err != nil {
		return err
	}

	if !d.MaxAmount.IsPositive() {
		return constant.ErrContextLimitsUnavailable
	}

	if _, err := tracercontract.AmountFromDecimal(ctx, d.MaxAmount, bounds); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return constant.ErrContextLimitsUnavailable
	}

	if err := validateContextLimitWindow(d); err != nil {
		return err
	}

	return validateContextAccountScopes(ctx, d.Scopes, maxScopes)
}

func validateContextLimitWindow(d Limit) error {
	if err := ValidateTimeWindow(d.ActiveTimeStart, d.ActiveTimeEnd); err != nil {
		return constant.ErrContextLimitsUnavailable
	}

	if d.ActiveTimeStart != nil && (d.ActiveTimeStart.IsZero() || d.ActiveTimeEnd.IsZero()) {
		return constant.ErrContextLimitsUnavailable
	}

	if err := d.validateCustomPeriod(); err != nil {
		return constant.ErrContextLimitsUnavailable
	}

	if d.CustomStartDate != nil && (d.CustomStartDate.IsZero() || d.CustomEndDate.IsZero() ||
		d.CustomStartDate.Year() < 1 || d.CustomEndDate.Year() > 9999) {
		return constant.ErrContextLimitsUnavailable
	}

	return nil
}

func validateContextAccountScopes(ctx context.Context, scopes []Scope, maxScopes int) error {
	if maxScopes <= 0 || len(scopes) == 0 || len(scopes) > maxScopes {
		return constant.ErrContextLimitsUnavailable
	}

	seen := make(map[uuid.UUID]struct{}, len(scopes))
	for _, scope := range scopes {
		if err := ctx.Err(); err != nil {
			return err
		}

		if scope.AccountID == nil || *scope.AccountID == uuid.Nil || scope.SegmentID != nil ||
			scope.PortfolioID != nil || scope.MerchantID != nil || scope.TransactionType != nil || scope.SubType != nil {
			return constant.ErrContextLimitsUnavailable
		}

		if _, exists := seen[*scope.AccountID]; exists {
			return constant.ErrContextLimitsUnavailable
		}

		seen[*scope.AccountID] = struct{}{}
	}

	return nil
}
