// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"

	"github.com/google/uuid"
)

// ptrMatches checks if a pattern pointer matches a value pointer.
// If pattern is nil, returns true (nil means "match any").
// If pattern is not nil but value is nil, returns false.
// If both are not nil, returns *pattern == *value.
func ptrMatches[T comparable](pattern, value *T) bool {
	if pattern == nil {
		return true
	}

	if value == nil {
		return false
	}

	return *pattern == *value
}

// ptrMatchesFold is like ptrMatches but compares string pointers case-insensitively.
// If pattern is nil, returns true (nil pattern means "match any").
// If pattern is not nil but value is nil, returns false.
// If both are not nil, returns strings.EqualFold(*pattern, *value).
func ptrMatchesFold(pattern, value *string) bool {
	if pattern == nil {
		return true
	}

	if value == nil {
		return false
	}

	return strings.EqualFold(*pattern, *value)
}

// normalizeSubTypeRaw returns a lowercase, whitespace-trimmed copy of s.
// Returns nil if s is nil. This is the canonical form persisted and matched
// case-insensitively against transactions via ptrMatchesFold, and is the
// single source of truth for the trim+lowercase canonicalization primitive
// used by both scope-level (normalizeScopeSubType) and request-level
// (NewValidationRequest, NormalizeAndValidate) normalization paths.
func normalizeSubTypeRaw(s *string) *string {
	if s == nil {
		return nil
	}

	v := strings.ToLower(strings.TrimSpace(*s))

	return &v
}

// normalizeScopeSubType normalizes the SubType pointer in-place on a Scope.
// Nil scope or nil SubType pointers remain unchanged. Empty-after-trim values
// are preserved as empty strings (validators decide whether empty is acceptable).
// Delegates to normalizeSubTypeRaw so all call sites share one canonical form.
func normalizeScopeSubType(s *Scope) {
	if s == nil {
		return
	}

	s.SubType = normalizeSubTypeRaw(s.SubType)
}

// cloneAndNormalizeScope returns a deep copy of s with SubType normalized to
// its canonical form. All pointer fields (SegmentID, PortfolioID, AccountID,
// MerchantID, TransactionType, SubType) are copied to fresh allocations so the
// returned Scope is fully independent of the caller's memory. Used by write
// paths (e.g. NewRule, Rule.Update) to prevent external mutation of persisted
// state and to keep the six-field deep-copy + normalize sequence in a single
// source of truth, eliminating drift when new Scope fields are added.
func cloneAndNormalizeScope(s Scope) Scope {
	scopeCopy := s

	if s.SegmentID != nil {
		segmentIDCopy := *s.SegmentID
		scopeCopy.SegmentID = &segmentIDCopy
	}

	if s.PortfolioID != nil {
		portfolioIDCopy := *s.PortfolioID
		scopeCopy.PortfolioID = &portfolioIDCopy
	}

	if s.AccountID != nil {
		accountIDCopy := *s.AccountID
		scopeCopy.AccountID = &accountIDCopy
	}

	if s.MerchantID != nil {
		merchantIDCopy := *s.MerchantID
		scopeCopy.MerchantID = &merchantIDCopy
	}

	if s.TransactionType != nil {
		transactionTypeCopy := *s.TransactionType
		scopeCopy.TransactionType = &transactionTypeCopy
	}

	normalizeScopeSubType(&scopeCopy)

	return scopeCopy
}

// Scope represents a hierarchical scope for rules and limits.
// ID fields are uuid.UUID pointers (validated at input layer).
// At least one field must be set for a scope to be valid.
// Validation tags are used by the HTTP layer for input validation.
// SubType is normalized to lowercase canonical form on write; matching is case-insensitive.
type Scope struct {
	// Segment the scope is restricted to (optional)
	// format: uuid
	SegmentID *uuid.UUID `json:"segmentId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Portfolio the scope is restricted to (optional)
	// format: uuid
	PortfolioID *uuid.UUID `json:"portfolioId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Account the scope is restricted to (optional)
	// format: uuid
	AccountID *uuid.UUID `json:"accountId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Merchant the scope is restricted to (optional)
	// format: uuid
	MerchantID *uuid.UUID `json:"merchantId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Transaction type the scope is restricted to (optional)
	// example: CARD
	TransactionType *TransactionType `json:"transactionType,omitempty" validate:"omitempty,transactiontype" swaggertype:"string" enums:"CARD,WIRE,PIX,CRYPTO" example:"CARD"`

	// SubType is normalized to lowercase canonical form; matching is case-insensitive.
	// example: purchase
	// maxLength: 50
	SubType *string `json:"subType,omitempty" validate:"omitempty,max=50" maxLength:"50" extensions:"x-normalization=lowercase" example:"purchase"`
}

// IsEmpty returns true if all scope fields are nil.
func (s *Scope) IsEmpty() bool {
	return s.SegmentID == nil &&
		s.PortfolioID == nil &&
		s.AccountID == nil &&
		s.MerchantID == nil &&
		s.TransactionType == nil &&
		s.SubType == nil
}

// Matches checks if this scope matches another scope.
// All non-nil fields in this scope must match corresponding fields in other.
// A nil field in this scope means "match any value" for that field.
func (s *Scope) Matches(other *Scope) bool {
	return ptrMatches(s.AccountID, other.AccountID) &&
		ptrMatches(s.SegmentID, other.SegmentID) &&
		ptrMatches(s.PortfolioID, other.PortfolioID) &&
		ptrMatches(s.MerchantID, other.MerchantID) &&
		ptrMatches(s.TransactionType, other.TransactionType) &&
		ptrMatchesFold(s.SubType, other.SubType)
}

// ToMap converts Scope to map[string]any for CEL evaluation.
// Only non-nil fields are included in the result.
func (s *Scope) ToMap() map[string]any {
	result := make(map[string]any)

	if s.SegmentID != nil {
		result["segmentId"] = s.SegmentID.String()
	}

	if s.PortfolioID != nil {
		result["portfolioId"] = s.PortfolioID.String()
	}

	if s.AccountID != nil {
		result["accountId"] = s.AccountID.String()
	}

	if s.MerchantID != nil {
		result["merchantId"] = s.MerchantID.String()
	}

	if s.TransactionType != nil {
		result["transactionType"] = s.TransactionType.String()
	}

	if s.SubType != nil {
		result["subType"] = *s.SubType
	}

	return result
}
