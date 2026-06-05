// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// validTransactionValidationSortFields defines the whitelist of allowed sort columns for transaction validation queries.
// This prevents SQL injection by validating sort columns before use.
// Uses snake_case convention for sort field names.
var validTransactionValidationSortFields = map[string]bool{
	"created_at":         true, // Default sort field
	"processing_time_ms": true, // Sort by processing time
}

// nowFunc is a replaceable time provider for testing.
// Tests can override this to control time-dependent behavior in SetDefaults.
var nowFunc = time.Now

// IsValidTransactionValidationSortField checks if a sort field is valid for transaction validation queries.
func IsValidTransactionValidationSortField(field string) bool {
	return validTransactionValidationSortFields[field]
}

// MaxTransactionValidationFilterLimit is the maximum number of records that can be returned in a single query.
// This protects against memory exhaustion from unbounded queries.
const MaxTransactionValidationFilterLimit = 1000

// DefaultTransactionValidationFilterLimit is the default number of records returned when no limit is specified.
const DefaultTransactionValidationFilterLimit = 100

// DefaultTransactionValidationDateRangeDays is the default date range (90 days) when no dates are specified.
const DefaultTransactionValidationDateRangeDays = 90

// TransactionValidationFilters defines filtering options for transaction validation queries.
// API Design v1.3.0: Single-tenant (no organizationId filter).
// TRD v1.2.6: Support filtering by decision, account, rule, and date range.
// Uses cursor-based pagination for consistent, efficient pagination with large datasets.
type TransactionValidationFilters struct {
	// StartDate filters records created at or after this timestamp (inclusive).
	// If zero and EndDate is also zero, defaults to 90 days ago.
	StartDate time.Time

	// EndDate filters records created at or before this timestamp (inclusive).
	// If zero and StartDate is also zero, defaults to current time.
	EndDate time.Time

	// Decision filters by validation decision (ALLOW, DENY, REVIEW).
	// If nil, all decisions are included.
	Decision *Decision

	// AccountID filters by the account ID in the request snapshot.
	// If nil, all accounts are included.
	AccountID *uuid.UUID

	// MatchedRuleID filters records where this rule ID appears in matchedRuleIds array.
	// If nil, all records are included regardless of matched rules.
	MatchedRuleID *uuid.UUID

	// ExceededLimitID filters records where this limit ID appears in limitUsageDetails
	// with exceeded=true.
	// If nil, all records are included regardless of exceeded limits.
	ExceededLimitID *uuid.UUID

	// SegmentID filters by the segment ID in the request snapshot.
	// If nil, all segments are included.
	SegmentID *uuid.UUID

	// PortfolioID filters by the portfolio ID in the request snapshot.
	// If nil, all portfolios are included.
	PortfolioID *uuid.UUID

	// TransactionType filters by the transaction type in the request snapshot.
	// If nil, all transaction types are included.
	TransactionType *TransactionType

	// Limit is the maximum number of records to return.
	// A value of 0 means "use DefaultTransactionValidationFilterLimit" (100) via SetDefaults().
	// Non-zero values must be between 1 and MaxTransactionValidationFilterLimit (1000).
	// Validate() rejects negative values and values exceeding MaxTransactionValidationFilterLimit.
	Limit int

	// Cursor is a base64-encoded opaque pagination cursor.
	// When provided, results start from the position encoded in the cursor.
	// Empty string means start from the beginning.
	Cursor string

	// SortBy is the field to sort by. Defaults to "created_at".
	// Must be a valid field from validTransactionValidationSortFields whitelist.
	SortBy string

	// SortOrder is the sort direction: "ASC" or "DESC". Defaults to "DESC".
	SortOrder string
}

// ListTransactionValidationsResult represents the paginated result of a transaction validation list query.
type ListTransactionValidationsResult struct {
	// TransactionValidations is the list of transaction validation records matching the filters.
	TransactionValidations []*TransactionValidation `json:"transactionValidations"`

	// NextCursor is a base64-encoded cursor for fetching the next page.
	// Empty if there are no more results (hasMore is false).
	NextCursor string `json:"nextCursor,omitempty"`

	// HasMore indicates whether there are more results beyond this page.
	HasMore bool `json:"hasMore"`
}

// Validate checks if the filters are valid.
// Returns an error wrapping constant.ErrInvalidTransactionValidationFilters if any constraint is violated.
func (f *TransactionValidationFilters) Validate() error {
	// Validate date range: endDate must be after startDate if both are set
	if !f.StartDate.IsZero() && !f.EndDate.IsZero() {
		if f.EndDate.Before(f.StartDate) {
			return fmt.Errorf("%w: end_date must be on or after start_date", constant.ErrInvalidTransactionValidationFilters)
		}
	}

	// Validate limit bounds
	if f.Limit < 0 {
		return fmt.Errorf("%w: limit cannot be negative", constant.ErrInvalidTransactionValidationFilters)
	}

	if f.Limit > MaxTransactionValidationFilterLimit {
		return fmt.Errorf("%w: limit cannot exceed %d", constant.ErrInvalidTransactionValidationFilters, MaxTransactionValidationFilterLimit)
	}

	// Validate sortBy if provided (empty means use default)
	if f.SortBy != "" && !IsValidTransactionValidationSortField(f.SortBy) {
		return fmt.Errorf("%w: invalid sort_by field", constant.ErrInvalidTransactionValidationFilters)
	}

	// Validate sortOrder if provided
	if f.SortOrder != "" {
		normalizedOrder := strings.ToUpper(f.SortOrder)
		if normalizedOrder != "ASC" && normalizedOrder != "DESC" {
			return fmt.Errorf("%w: sort_order must be ASC or DESC", constant.ErrInvalidTransactionValidationFilters)
		}
	}

	// Validate TransactionType if provided
	if f.TransactionType != nil && !f.TransactionType.IsValid() {
		return fmt.Errorf("%w: invalid transaction_type", constant.ErrInvalidTransactionValidationFilters)
	}

	return nil
}

// SetDefaults sets default values for optional fields.
// Call this before using the filters to ensure sensible defaults.
// Note: Default date range is only applied when BOTH StartDate and EndDate are zero.
// If only one date is specified, the other remains unset to allow open-ended queries.
func (f *TransactionValidationFilters) SetDefaults() {
	// Set default limit if not specified
	if f.Limit == 0 {
		f.Limit = DefaultTransactionValidationFilterLimit
	}

	// Set default date range if neither date is specified
	// Default to last 90 days for performance and relevance
	// Use truncated day boundaries for consistent caching and deterministic queries
	if f.StartDate.IsZero() && f.EndDate.IsZero() {
		now := nowFunc().UTC().Truncate(24 * time.Hour)
		// EndDate is end of today (start of tomorrow minus 1 nanosecond would be complex,
		// so we use start of tomorrow for simplicity - includes all of today)
		f.EndDate = now.Add(24 * time.Hour)
		f.StartDate = now.Add(-DefaultTransactionValidationDateRangeDays * 24 * time.Hour)
	}

	// Set default sort field
	if f.SortBy == "" {
		f.SortBy = "created_at"
	}

	// Set default sort order
	if f.SortOrder == "" {
		f.SortOrder = "DESC"
	} else {
		f.SortOrder = strings.ToUpper(f.SortOrder)
	}
}
