// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestValidateCursorConsistency(t *testing.T) {
	tests := []struct {
		name        string
		cursor      string
		sortBy      string
		sortOrder   string
		entity      string
		wantErr     bool
		wantCode    string
		wantMessage string
	}{
		{
			name:      "valid: no cursor, no sort params",
			cursor:    "",
			sortBy:    "",
			sortOrder: "",
			entity:    constant.EntityLimit,
			wantErr:   false,
		},
		{
			name:      "valid: cursor only",
			cursor:    "abc123",
			sortBy:    "",
			sortOrder: "",
			entity:    constant.EntityLimit,
			wantErr:   false,
		},
		{
			name:      "valid: sort params only",
			cursor:    "",
			sortBy:    "created_at",
			sortOrder: "ASC",
			entity:    constant.EntityLimit,
			wantErr:   false,
		},
		{
			name:        "invalid: cursor with sortBy",
			cursor:      "abc123",
			sortBy:      "created_at",
			sortOrder:   "",
			entity:      constant.EntityAuditEvent,
			wantErr:     true,
			wantCode:    "0334",
			wantMessage: "sort_by and sort_order cannot be used with cursor; cursor already contains sort configuration",
		},
		{
			name:        "invalid: cursor with sortOrder",
			cursor:      "abc123",
			sortBy:      "",
			sortOrder:   "DESC",
			entity:      constant.EntityTransactionValidation,
			wantErr:     true,
			wantCode:    "0334",
			wantMessage: "sort_by and sort_order cannot be used with cursor; cursor already contains sort configuration",
		},
		{
			name:        "invalid: cursor with both sort params",
			cursor:      "abc123",
			sortBy:      "name",
			sortOrder:   "ASC",
			entity:      constant.EntityRule,
			wantErr:     true,
			wantCode:    "0334",
			wantMessage: "sort_by and sort_order cannot be used with cursor; cursor already contains sort configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCursorConsistency(tt.cursor, tt.sortBy, tt.sortOrder, tt.entity)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateCursorConsistency() expected error but got nil")
					return
				}

				var listErr pkg.ValidationError
				if !errors.As(err, &listErr) {
					t.Errorf("ValidateCursorConsistency() error type = %T, want pkg.ValidationError", err)
					return
				}

				if listErr.Code != tt.wantCode {
					t.Errorf("ValidateCursorConsistency() code = %v, want %v", listErr.Code, tt.wantCode)
				}

				if listErr.EntityType != tt.entity {
					t.Errorf("ValidateCursorConsistency() entityType = %v, want %v", listErr.EntityType, tt.entity)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateCursorConsistency() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidatePaginationLimit(t *testing.T) {
	limit1 := 1
	limit0 := 0
	limitNeg := -5
	limit50 := 50
	limit101 := 101

	tests := []struct {
		name        string
		limit       *int
		maxLimit    int
		entity      string
		wantErr     bool
		wantCode    string
		wantMessage string
	}{
		{
			name:     "valid: nil limit",
			limit:    nil,
			maxLimit: 100,
			entity:   constant.EntityLimit,
			wantErr:  false,
		},
		{
			name:     "valid: limit within range",
			limit:    &limit50,
			maxLimit: 100,
			entity:   constant.EntityLimit,
			wantErr:  false,
		},
		{
			name:     "valid: limit equals 1",
			limit:    &limit1,
			maxLimit: 100,
			entity:   constant.EntityLimit,
			wantErr:  false,
		},
		{
			name:        "invalid: limit is 0",
			limit:       &limit0,
			maxLimit:    100,
			entity:      constant.EntityAuditEvent,
			wantErr:     true,
			wantCode:    "0331",
			wantMessage: "limit must be at least 1",
		},
		{
			name:        "invalid: limit is negative",
			limit:       &limitNeg,
			maxLimit:    100,
			entity:      constant.EntityRule,
			wantErr:     true,
			wantCode:    "0331",
			wantMessage: "limit must be at least 1",
		},
		{
			name:        "invalid: limit exceeds max",
			limit:       &limit101,
			maxLimit:    100,
			entity:      constant.EntityTransactionValidation,
			wantErr:     true,
			wantCode:    "0080",
			wantMessage: "limit must not exceed 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePaginationLimit(tt.limit, tt.maxLimit, tt.entity)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidatePaginationLimit() expected error but got nil")
					return
				}

				var listErr pkg.ValidationError
				if !errors.As(err, &listErr) {
					t.Errorf("ValidatePaginationLimit() error type = %T, want pkg.ValidationError", err)
					return
				}

				if listErr.Code != tt.wantCode {
					t.Errorf("ValidatePaginationLimit() code = %v, want %v", listErr.Code, tt.wantCode)
				}

				if listErr.EntityType != tt.entity {
					t.Errorf("ValidatePaginationLimit() entityType = %v, want %v", listErr.EntityType, tt.entity)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePaginationLimit() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateSortOrder(t *testing.T) {
	tests := []struct {
		name        string
		sortOrder   string
		entity      string
		wantErr     bool
		wantCode    string
		wantMessage string
	}{
		{
			name:      "valid: empty string",
			sortOrder: "",
			entity:    constant.EntityLimit,
			wantErr:   false,
		},
		{
			name:      "valid: ASC uppercase",
			sortOrder: "ASC",
			entity:    constant.EntityLimit,
			wantErr:   false,
		},
		{
			name:      "valid: DESC uppercase",
			sortOrder: "DESC",
			entity:    constant.EntityLimit,
			wantErr:   false,
		},
		{
			name:      "valid: asc lowercase",
			sortOrder: "asc",
			entity:    constant.EntityLimit,
			wantErr:   false,
		},
		{
			name:      "valid: desc lowercase",
			sortOrder: "desc",
			entity:    constant.EntityLimit,
			wantErr:   false,
		},
		{
			name:      "valid: mixed case",
			sortOrder: "AsC",
			entity:    constant.EntityLimit,
			wantErr:   false,
		},
		{
			name:        "invalid: random string",
			sortOrder:   "invalid",
			entity:      constant.EntityAuditEvent,
			wantErr:     true,
			wantCode:    "0081",
			wantMessage: "sort_order must be ASC or DESC",
		},
		{
			name:        "invalid: number",
			sortOrder:   "123",
			entity:      constant.EntityTransactionValidation,
			wantErr:     true,
			wantCode:    "0081",
			wantMessage: "sort_order must be ASC or DESC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSortOrder(tt.sortOrder, tt.entity)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateSortOrder() expected error but got nil")
					return
				}

				var listErr pkg.ValidationError
				if !errors.As(err, &listErr) {
					t.Errorf("ValidateSortOrder() error type = %T, want pkg.ValidationError", err)
					return
				}

				if listErr.Code != tt.wantCode {
					t.Errorf("ValidateSortOrder() code = %v, want %v", listErr.Code, tt.wantCode)
				}

				if listErr.EntityType != tt.entity {
					t.Errorf("ValidateSortOrder() entityType = %v, want %v", listErr.EntityType, tt.entity)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateSortOrder() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateSortBy(t *testing.T) {
	allowedFields := []string{"created_at", "updated_at", "name", "status"}

	tests := []struct {
		name        string
		sortBy      string
		entity      string
		wantErr     bool
		wantCode    string
		wantMessage string
	}{
		{
			name:    "valid: empty string",
			sortBy:  "",
			entity:  constant.EntityLimit,
			wantErr: false,
		},
		{
			name:    "valid: created_at",
			sortBy:  "created_at",
			entity:  constant.EntityLimit,
			wantErr: false,
		},
		{
			name:    "valid: updated_at",
			sortBy:  "updated_at",
			entity:  constant.EntityLimit,
			wantErr: false,
		},
		{
			name:    "valid: name",
			sortBy:  "name",
			entity:  constant.EntityLimit,
			wantErr: false,
		},
		{
			name:    "valid: status",
			sortBy:  "status",
			entity:  constant.EntityLimit,
			wantErr: false,
		},
		{
			name:        "invalid: not in whitelist",
			sortBy:      "invalidField",
			entity:      constant.EntityAuditEvent,
			wantErr:     true,
			wantCode:    "0332",
			wantMessage: "sort_by must be one of [created_at updated_at name status]",
		},
		{
			name:        "invalid: camelCase rejected",
			sortBy:      "createdAt",
			entity:      constant.EntityTransactionValidation,
			wantErr:     true,
			wantCode:    "0332",
			wantMessage: "sort_by must be one of [created_at updated_at name status]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSortBy(tt.sortBy, allowedFields, tt.entity)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateSortBy() expected error but got nil")
					return
				}

				var listErr pkg.ValidationError
				if !errors.As(err, &listErr) {
					t.Errorf("ValidateSortBy() error type = %T, want pkg.ValidationError", err)
					return
				}

				if listErr.Code != tt.wantCode {
					t.Errorf("ValidateSortBy() code = %v, want %v", listErr.Code, tt.wantCode)
				}

				if listErr.EntityType != tt.entity {
					t.Errorf("ValidateSortBy() entityType = %v, want %v", listErr.EntityType, tt.entity)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateSortBy() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestListInputsThreadEntityType asserts that a pagination or sort validation
// error raised by each LIST route's Validate() reports that route's own entity
// type, rather than the entity of the validator's default implementation.
func TestListInputsThreadEntityType(t *testing.T) {
	zeroLimit := 0
	invalidSortOrder := "not-a-direction"

	tests := []struct {
		name       string
		input      interface{ Validate() error }
		wantEntity string
	}{
		{
			name:       "limits: pagination error carries Limit entity",
			input:      &ListLimitsInput{Limit: &zeroLimit},
			wantEntity: constant.EntityLimit,
		},
		{
			name:       "limits: sort error carries Limit entity",
			input:      &ListLimitsInput{SortOrder: invalidSortOrder},
			wantEntity: constant.EntityLimit,
		},
		{
			name:       "transaction validations: pagination error carries TransactionValidation entity",
			input:      &ListTransactionValidationsInput{Limit: &zeroLimit},
			wantEntity: constant.EntityTransactionValidation,
		},
		{
			name:       "transaction validations: sort error carries TransactionValidation entity",
			input:      &ListTransactionValidationsInput{SortOrder: invalidSortOrder},
			wantEntity: constant.EntityTransactionValidation,
		},
		{
			name:       "audit events: pagination error carries AuditEvent entity",
			input:      &ListAuditEventsInput{Limit: &zeroLimit},
			wantEntity: constant.EntityAuditEvent,
		},
		{
			name:       "audit events: sort error carries AuditEvent entity",
			input:      &ListAuditEventsInput{SortOrder: invalidSortOrder},
			wantEntity: constant.EntityAuditEvent,
		},
		{
			name:       "rules: pagination error carries Rule entity",
			input:      &ListRulesInput{Limit: &zeroLimit},
			wantEntity: constant.EntityRule,
		},
		{
			name:       "rules: sort error carries Rule entity",
			input:      &ListRulesInput{SortOrder: invalidSortOrder},
			wantEntity: constant.EntityRule,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if err == nil {
				t.Fatalf("Validate() expected error but got nil")
			}

			var listErr pkg.ValidationError
			if !errors.As(err, &listErr) {
				t.Fatalf("Validate() error type = %T, want pkg.ValidationError", err)
			}

			if listErr.EntityType != tt.wantEntity {
				t.Errorf("Validate() entityType = %q, want %q", listErr.EntityType, tt.wantEntity)
			}
		})
	}
}

func TestNormalizeSortOrder(t *testing.T) {
	tests := []struct {
		name         string
		sortOrder    string
		defaultValue string
		expected     string
	}{
		{
			name:         "empty string returns default",
			sortOrder:    "",
			defaultValue: "DESC",
			expected:     "DESC",
		},
		{
			name:         "lowercase asc normalized to ASC",
			sortOrder:    "asc",
			defaultValue: "DESC",
			expected:     "ASC",
		},
		{
			name:         "lowercase desc normalized to DESC",
			sortOrder:    "desc",
			defaultValue: "ASC",
			expected:     "DESC",
		},
		{
			name:         "uppercase ASC remains ASC",
			sortOrder:    "ASC",
			defaultValue: "DESC",
			expected:     "ASC",
		},
		{
			name:         "uppercase DESC remains DESC",
			sortOrder:    "DESC",
			defaultValue: "ASC",
			expected:     "DESC",
		},
		{
			name:         "mixed case AsC normalized to ASC",
			sortOrder:    "AsC",
			defaultValue: "DESC",
			expected:     "ASC",
		},
		{
			name:         "mixed case DeSc normalized to DESC",
			sortOrder:    "DeSc",
			defaultValue: "ASC",
			expected:     "DESC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeSortOrder(tt.sortOrder, tt.defaultValue)

			if result != tt.expected {
				t.Errorf("NormalizeSortOrder() = %v, want %v", result, tt.expected)
			}
		})
	}
}
