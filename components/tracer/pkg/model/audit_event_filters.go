// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// validAuditEventSortFields defines allowed sort columns.
var validAuditEventSortFields = map[string]bool{
	"created_at": true,
	"event_type": true,
}

// IsValidAuditEventSortField checks if a sort field is valid.
func IsValidAuditEventSortField(field string) bool {
	return validAuditEventSortFields[field]
}

// MaxAuditEventFilterLimit is the maximum records per query.
const MaxAuditEventFilterLimit = 1000

// DefaultAuditEventFilterLimit is the default records per query.
const DefaultAuditEventFilterLimit = 100

// DefaultAuditEventDateRangeDays is the default date range (90 days) when no dates are specified.
const DefaultAuditEventDateRangeDays = 90

// AuditEventFilters defines filtering options for audit event queries.
type AuditEventFilters struct {
	// Date range (using StartDate/EndDate for consistency with TransactionValidationFilters)
	StartDate time.Time
	EndDate   time.Time

	// Core filters
	EventType    *AuditEventType
	Action       *AuditAction
	Result       *AuditResult
	ResourceType *ResourceType
	ResourceID   *string

	// Actor filters
	ActorType *ActorType
	ActorID   *string

	// JSONB filters (extracted from context.request)
	// These use JSONB indexes for efficient querying
	AccountID       *uuid.UUID       // context.request.account.id
	SegmentID       *uuid.UUID       // context.request.account.segmentId
	PortfolioID     *uuid.UUID       // context.request.account.portfolioId
	TransactionType *TransactionType // context.request.transactionType
	MatchedRuleID   *uuid.UUID       // context.response.matchedRuleIds (array contains)

	// Pagination
	Limit     int
	Cursor    string
	SortBy    string
	SortOrder string
}

// ListAuditEventsResult represents paginated audit event results.
type ListAuditEventsResult struct {
	AuditEvents []*AuditEvent `json:"auditEvents"`
	NextCursor  string        `json:"nextCursor,omitempty"`
	HasMore     bool          `json:"hasMore"`
}

// Validate checks if filters are valid.
func (f *AuditEventFilters) Validate() error {
	if !f.StartDate.IsZero() && !f.EndDate.IsZero() {
		if f.EndDate.Before(f.StartDate) {
			return fmt.Errorf("%w: end_date must be on or after start_date", constant.ErrInvalidAuditEventFilters)
		}
	}

	if f.Limit < 0 {
		return fmt.Errorf("%w: limit cannot be negative", constant.ErrInvalidAuditEventFilters)
	}

	if f.Limit > MaxAuditEventFilterLimit {
		return fmt.Errorf("%w: limit cannot exceed %d", constant.ErrInvalidAuditEventFilters, MaxAuditEventFilterLimit)
	}

	if f.SortBy != "" && !IsValidAuditEventSortField(f.SortBy) {
		return fmt.Errorf("%w: invalid sort_by field", constant.ErrInvalidAuditEventFilters)
	}

	if f.SortOrder != "" {
		normalizedOrder := strings.ToUpper(f.SortOrder)
		if normalizedOrder != "ASC" && normalizedOrder != "DESC" {
			return fmt.Errorf("%w: sort_order must be ASC or DESC", constant.ErrInvalidAuditEventFilters)
		}
	}

	return nil
}

// SetDefaults sets default values for optional fields.
// Call this before using the filters to ensure sensible defaults.
// Note: Default date range is only applied when BOTH StartDate and EndDate are zero.
// If only one date is specified, the other remains unset to allow open-ended queries.
func (f *AuditEventFilters) SetDefaults() {
	if f.Limit == 0 {
		f.Limit = DefaultAuditEventFilterLimit
	}

	// Set default date range if neither date is specified
	// Default to last 90 days for performance and relevance
	// Use truncated day boundaries for consistent caching and deterministic queries
	if f.StartDate.IsZero() && f.EndDate.IsZero() {
		now := time.Now().UTC().Truncate(24 * time.Hour)
		// EndDate is end of today (start of tomorrow for inclusive range)
		f.EndDate = now.Add(24 * time.Hour)
		f.StartDate = now.Add(-DefaultAuditEventDateRangeDays * 24 * time.Hour)
	}

	if f.SortBy == "" {
		f.SortBy = "created_at"
	}

	if f.SortOrder == "" {
		f.SortOrder = "DESC"
	} else {
		f.SortOrder = strings.ToUpper(f.SortOrder)
	}
}

// HashChainVerificationResult represents the result of hash chain verification.
type HashChainVerificationResult struct {
	IsValid        bool   `json:"isValid" example:"true"`
	FirstInvalidID *int64 `json:"firstInvalidId,omitempty" example:"42"`
	TotalChecked   int64  `json:"totalChecked" example:"1000"`
	Message        string `json:"message" example:"hash chain intact"`
} //	@name	HashChainVerificationResult
