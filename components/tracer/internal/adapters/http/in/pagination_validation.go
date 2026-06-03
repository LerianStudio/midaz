// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"strings"
)

// ValidateCursorConsistency checks if cursor is used with sort_by/sort_order.
// Returns ValidationError with TRC-0045 if both are present.
// This validation ensures clients don't attempt to override the sort configuration
// already encoded in the cursor (which contains sort_by and sort_order internally).
func ValidateCursorConsistency(cursor, sortBy, sortOrder string) error {
	if cursor != "" && (sortBy != "" || sortOrder != "") {
		return &ValidationError{
			Code:    "TRC-0045",
			Message: "sort_by and sort_order cannot be used with cursor; cursor already contains sort configuration",
		}
	}

	return nil
}

// ValidatePaginationLimit validates that limit is within the valid range.
// Returns TRC-0041 if less than 1, TRC-0040 if exceeds maxLimit.
func ValidatePaginationLimit(limit *int, maxLimit int) error {
	if limit == nil {
		return nil
	}

	if *limit < 1 {
		return &ValidationError{
			Code:    "TRC-0041",
			Message: "limit must be at least 1",
		}
	}

	if *limit > maxLimit {
		return &ValidationError{
			Code:    "TRC-0040",
			Message: fmt.Sprintf("limit must not exceed %d", maxLimit),
		}
	}

	return nil
}

// ValidateSortOrder validates that sortOrder is ASC or DESC (case-insensitive).
// Returns TRC-0042 if the value is not one of the allowed values.
func ValidateSortOrder(sortOrder string) error {
	if sortOrder == "" {
		return nil
	}

	upperSortOrder := strings.ToUpper(sortOrder)
	if upperSortOrder != "ASC" && upperSortOrder != "DESC" {
		return &ValidationError{
			Code:    "TRC-0042",
			Message: "sort_order must be ASC or DESC",
		}
	}

	return nil
}

// ValidateSortBy validates that sortBy is in the whitelist of allowed fields.
// Returns TRC-0043 if the field is not in the list.
func ValidateSortBy(sortBy string, allowedFields []string) error {
	if sortBy == "" {
		return nil
	}

	for _, field := range allowedFields {
		if sortBy == field {
			return nil
		}
	}

	return &ValidationError{
		Code:    "TRC-0043",
		Message: fmt.Sprintf("sort_by must be one of %v", allowedFields),
	}
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
