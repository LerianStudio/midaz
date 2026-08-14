// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ValidateCursorConsistency checks if cursor is used with sort_by/sort_order.
// Returns a canonical ErrCursorWithSortParams error if both are present.
// This validation ensures clients don't attempt to override the sort configuration
// already encoded in the cursor (which contains sort_by and sort_order internally).
func ValidateCursorConsistency(cursor, sortBy, sortOrder string) error {
	if cursor != "" && (sortBy != "" || sortOrder != "") {
		return pkg.ValidateBusinessError(constant.ErrCursorWithSortParams, constant.EntityRule)
	}

	return nil
}

// ValidatePaginationLimit validates that limit is within the valid range.
// Returns ErrPaginationLimitInvalid if less than 1, ErrPaginationLimitExceeded if it exceeds maxLimit.
func ValidatePaginationLimit(limit *int, maxLimit int) error {
	if limit == nil {
		return nil
	}

	if *limit < 1 {
		return pkg.ValidateBusinessError(constant.ErrPaginationLimitInvalid, constant.EntityRule)
	}

	if *limit > maxLimit {
		return pkg.ValidateBusinessError(constant.ErrPaginationLimitExceeded, constant.EntityRule, maxLimit)
	}

	return nil
}

// ValidateSortOrder validates that sortOrder is ASC or DESC (case-insensitive).
// Returns ErrInvalidSortOrder if the value is not one of the allowed values.
func ValidateSortOrder(sortOrder string) error {
	if sortOrder == "" {
		return nil
	}

	upperSortOrder := strings.ToUpper(sortOrder)
	if upperSortOrder != "ASC" && upperSortOrder != "DESC" {
		return pkg.ValidateBusinessError(constant.ErrInvalidSortOrder, constant.EntityRule)
	}

	return nil
}

// ValidateSortBy validates that sortBy is in the whitelist of allowed fields.
// Returns ErrInvalidSortColumn if the field is not in the list.
func ValidateSortBy(sortBy string, allowedFields []string) error {
	if sortBy == "" {
		return nil
	}

	for _, field := range allowedFields {
		if sortBy == field {
			return nil
		}
	}

	return pkg.ValidateBusinessError(constant.ErrInvalidSortColumn, constant.EntityRule)
}

// NormalizeSortOrder normalizes sortOrder to uppercase (ASC or DESC).
// If sortOrder is empty, returns the provided default value.
// This function should be called in SetDefaults() after validation.
func NormalizeSortOrder(sortOrder, defaultValue string) string {
	if sortOrder == "" {
		return defaultValue
	}

	return strings.ToUpper(sortOrder)
}
