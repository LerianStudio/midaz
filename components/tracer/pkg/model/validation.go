// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"maps"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
)

// metadataKeyPattern allows only alphanumeric characters and underscores
var metadataKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// Valid account types per API design
var validAccountTypes = map[string]bool{
	"checking": true, "savings": true, "credit": true,
}

// Valid account statuses per API design
var validAccountStatuses = map[string]bool{
	"active": true, "suspended": true, "closed": true,
}

// mccPattern validates 4-digit MCC codes (ISO 18245)
var mccPattern = regexp.MustCompile(`^\d{4}$`)

// countryCodePattern validates ISO 3166-1 alpha-2 codes (2 uppercase letters)
var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

// DefaultClockSkewTolerance is the default maximum allowed time difference between
// the transaction timestamp and the server's current time to account for clock drift.
const DefaultClockSkewTolerance = 1 * time.Minute

// ClockSkewTolerance is the configurable maximum allowed time difference between
// the transaction timestamp and the server's current time. Callers can override
// this value at startup to adjust tolerance (e.g., 100-500ms for stricter checks).
var ClockSkewTolerance = DefaultClockSkewTolerance

// DefaultMaxTimestampAge is the default maximum age allowed for a transaction timestamp.
// Transactions with timestamps older than this duration from the current time are rejected.
// This prevents replay attacks and stale transaction processing.
const DefaultMaxTimestampAge = 24 * time.Hour

// MaxTimestampAge is the configurable maximum age allowed for a transaction timestamp.
// Callers can override this value at startup to adjust the window (e.g., 1h for stricter,
// 48h for more lenient). Timestamps older than now minus MaxTimestampAge are rejected.
var MaxTimestampAge = DefaultMaxTimestampAge

// Decision represents the validation decision
type Decision string

const (
	DecisionAllow  Decision = "ALLOW"
	DecisionDeny   Decision = "DENY"
	DecisionReview Decision = "REVIEW"
)

// ValidationRequest is the input for transaction validation.
// Amount is expressed as a decimal value (e.g., 1000.00 for USD/BRL).
// Use NewValidationRequest() to construct - ensures validation and normalization.
// SubType is normalized to lowercase canonical form; matching is case-insensitive.
type ValidationRequest struct {
	RequestID       uuid.UUID       `json:"requestId" validate:"required" swaggertype:"string" format:"uuid"`
	TransactionType TransactionType `json:"transactionType" validate:"required"`
	// SubType is normalized to lowercase canonical form; matching is case-insensitive.
	SubType              *string           `json:"subType,omitempty" validate:"omitempty,max=50" maxLength:"50" extensions:"x-normalization=lowercase"`
	Amount               decimal.Decimal   `json:"amount" validate:"required" swaggertype:"string" example:"100.00"`
	Currency             string            `json:"currency" validate:"required"`
	TransactionTimestamp time.Time         `json:"transactionTimestamp" format:"date-time" validate:"required"`
	Account              AccountContext    `json:"account" validate:"required"`
	Segment              *SegmentContext   `json:"segment,omitempty"`
	Portfolio            *PortfolioContext `json:"portfolio,omitempty"`
	Merchant             *MerchantContext  `json:"merchant,omitempty"`
	Metadata             map[string]any    `json:"metadata,omitempty"`
}

// NewValidationRequest creates a new ValidationRequest with validation and normalization.
// Currency is normalized to uppercase and trimmed (auto-corrects case).
// SubType is trimmed and lowercased (canonical form) if provided; matching is case-insensitive.
// Metadata is shallow-copied (top-level keys only) to detach from the original map.
// Note: nested maps/slices within metadata values remain shared references.
// Returns error if validation fails after normalization.
//
// Use this constructor when:
// - Building requests programmatically where currency normalization is desired
// - You want automatic currency case correction (e.g., "usd" → "USD")
//
// For strict post-JSON-parse validation without currency normalization, use NormalizeAndValidate() instead.
func NewValidationRequest(
	now time.Time,
	requestID uuid.UUID,
	transactionType TransactionType,
	subType *string,
	amount decimal.Decimal,
	currency string,
	transactionTimestamp time.Time,
	account AccountContext,
	segment *SegmentContext,
	portfolio *PortfolioContext,
	merchant *MerchantContext,
	metadata map[string]any,
) (*ValidationRequest, error) {
	// Normalize currency (uppercase and trim)
	normalizedCurrency := strings.ToUpper(strings.TrimSpace(currency))

	// Normalize subType if provided (trim + lowercase for canonical form)
	normalizedSubType := normalizeSubTypeRaw(subType)

	// Shallow copy of metadata to detach top-level map entries
	// Note: nested maps/slices share references with original (acceptable trade-off)
	var metadataCopy map[string]any
	if metadata != nil {
		metadataCopy = make(map[string]any, len(metadata))
		maps.Copy(metadataCopy, metadata)
	}

	// Defensive copy of nested context metadata maps using Clone() methods
	segmentCopy := segment.Clone()
	portfolioCopy := portfolio.Clone()
	merchantCopy := merchant.Clone()

	req := &ValidationRequest{
		RequestID:            requestID,
		TransactionType:      transactionType,
		SubType:              normalizedSubType,
		Amount:               amount,
		Currency:             normalizedCurrency,
		TransactionTimestamp: transactionTimestamp,
		Account:              account,
		Segment:              segmentCopy,
		Portfolio:            portfolioCopy,
		Merchant:             merchantCopy,
		Metadata:             metadataCopy,
	}

	if err := req.Validate(now); err != nil {
		return nil, err
	}

	return req, nil
}

// NormalizeAndValidate normalizes non-critical fields and validates the request in-place.
// This method is useful after JSON parsing where the struct is already constructed.
// SubType is trimmed and lowercased (canonical form) and Metadata maps are defensively copied at all levels:
// - Top-level Metadata map is shallow-copied
// - Nested context metadata (Segment.Metadata, Portfolio.Metadata, Merchant.Metadata) are also shallow-copied
// Note: Values within metadata maps remain shared references if they are maps/slices themselves.
// Currency is NOT normalized - API enforces strict ISO 4217 uppercase validation (e.g., "usd" will fail).
// Returns error if validation fails after normalization.
//
// Atomicity: If validation fails, the receiver is NOT modified. Normalization is only applied
// after successful validation. This allows callers to safely retry or inspect the original values.
//
// Use this method when:
// - Validating after JSON deserialization where strict ISO 4217 uppercase currency is required
// - You want to enforce that clients send properly formatted currency codes
//
// For programmatic construction with automatic currency normalization, use NewValidationRequest() instead.
func (r *ValidationRequest) NormalizeAndValidate(now time.Time) error {
	return r.normalizeAndValidateWith(now, (*ValidationRequest).Validate)
}

// NormalizeAndValidateForReserve normalizes the request exactly as
// NormalizeAndValidate does, then validates it with the relaxed reserve rules
// (ValidateForReserve): transactionType and account are optional. Same atomic
// commit semantics — the receiver is only mutated when validation succeeds.
func (r *ValidationRequest) NormalizeAndValidateForReserve(now time.Time) error {
	return r.normalizeAndValidateWith(now, (*ValidationRequest).ValidateForReserve)
}

// normalizeAndValidateWith applies the shared normalization (subType canonical
// form, defensive metadata copies at all levels) on a temporary copy, runs the
// supplied validator against that copy, and commits the normalized values to
// the receiver only when validation succeeds. The validate parameter is the one
// difference between the strict (Validate) and reserve (ValidateForReserve)
// paths, so both share one normalization body.
func (r *ValidationRequest) normalizeAndValidateWith(now time.Time, validate func(*ValidationRequest, time.Time) error) error {
	// Prepare normalized values without mutating the receiver yet
	// SubType canonical form is lowercase (trim + lower).
	normalizedSubType := normalizeSubTypeRaw(r.SubType)

	// Prepare shallow copy of top-level metadata
	var metadataCopy map[string]any
	if r.Metadata != nil {
		metadataCopy = make(map[string]any, len(r.Metadata))
		maps.Copy(metadataCopy, r.Metadata)
	}

	// Create temporary copy with normalized values for validation
	temp := *r
	temp.SubType = normalizedSubType
	temp.Metadata = metadataCopy

	// Deep copy nested context metadata to prevent shared references using Clone() methods
	temp.Segment = temp.Segment.Clone()
	temp.Portfolio = temp.Portfolio.Clone()
	temp.Merchant = temp.Merchant.Clone()

	// Validate on temp - if error, original r remains unchanged
	if err := validate(&temp, now); err != nil {
		return err
	}

	// Only apply changes if validation succeeded (atomic commit)
	r.SubType = normalizedSubType
	r.Metadata = metadataCopy
	r.Segment = temp.Segment
	r.Portfolio = temp.Portfolio
	r.Merchant = temp.Merchant

	return nil
}

// LimitUsageDetail contains usage information for a checked limit.
// Amounts are expressed as decimal values.
// Note: RemainingAmount is calculated as (LimitAmount - CurrentUsage), not stored.
type LimitUsageDetail struct {
	LimitID     uuid.UUID       `json:"limitId" swaggertype:"string" format:"uuid"`
	LimitAmount decimal.Decimal `json:"limitAmount" swaggertype:"string" example:"1000.00"`
	// Scope is a human-readable string representation of the limit's scope
	// (e.g., "account:uuid" or "segment:uuid" or "global").
	Scope string `json:"scope"`
	// Period indicates the type of limit (DAILY, WEEKLY, MONTHLY, CUSTOM, PER_TRANSACTION).
	Period LimitType `json:"period" swaggertype:"string"`
	// CurrentUsage represents the PROJECTED usage after applying the transaction amount,
	// not the actual persisted counter value. This is calculated as:
	// (counter.CurrentUsage + input.Amount) for DAILY/WEEKLY/MONTHLY/CUSTOM limits, or 0 for PER_TRANSACTION.
	// When Exceeded=true, the counter was NOT incremented, but CurrentUsage still shows
	// what the usage would have been if the transaction were allowed.
	CurrentUsage decimal.Decimal `json:"currentUsage" swaggertype:"string" example:"500.00"`
	// AttemptedAmount is the transaction amount being validated against this limit.
	// Matches the amount from the validation request.
	AttemptedAmount decimal.Decimal `json:"attemptedAmount" swaggertype:"string" example:"100.00"`
	Exceeded        bool            `json:"exceeded"`
	// Skipped indicates whether this limit was skipped during evaluation (not enforced).
	// When true, the counter was NOT incremented and Exceeded is always false.
	Skipped bool `json:"skipped,omitempty"`
	// SkipReason explains why the limit was skipped (only set when Skipped=true).
	// Values: "outside_time_window" (outside active hours), "outside_custom_period" (outside custom date range).
	SkipReason string `json:"skipReason,omitempty" example:"outside_time_window"`

	// Internal fields for transactional rollback - not serialized to JSON.
	// InternalLimitType stores the persistent limit type to skip PER_TRANSACTION limits
	// (no persistent counters) without needing to re-fetch the limit from the database.
	// Note: Period (above) is the API-facing field; this is for internal use only.
	InternalLimitType LimitType `json:"-"`
	// Scopes contains the limit's scopes, used to calculate scopeKey without
	// needing to re-fetch the limit from the database.
	Scopes []Scope `json:"-"`
	// InternalPeriodKey stores the period key computed during CheckLimits.
	// Targets the exact same period counter that was incremented, preventing
	// period key mismatch when rollback crosses a period boundary.
	// Empty for PER_TRANSACTION limits (no period counters).
	InternalPeriodKey string `json:"-"`
}

// ValidationResponse is the output of transaction validation.
// Embeds EvaluationResult to avoid field duplication.
// Aligned with TRD v1.2.4: arrays for matched/evaluated rules and limit details.
type ValidationResponse struct {
	ValidationID      uuid.UUID `json:"validationId" swaggertype:"string" format:"uuid"`
	RequestID         uuid.UUID `json:"requestId" swaggertype:"string" format:"uuid"`
	EvaluationResult  `swaggerignore:"true"`
	LimitUsageDetails []LimitUsageDetail `json:"limitUsageDetails"`
	ProcessingTimeMs  float64            `json:"processingTimeMs"`
	EvaluatedAt       time.Time          `json:"evaluatedAt" format:"date-time"`
}

// NewValidationResponse creates a ValidationResponse with initialized slices.
// Ensures JSON serialization produces [] instead of null for empty arrays.
// validationID is the server-generated unique identifier for the audit record.
// evaluatedAt is the server timestamp when the evaluation started.
func NewValidationResponse(validationID, requestID uuid.UUID, decision Decision, evaluatedAt time.Time) *ValidationResponse {
	return &ValidationResponse{
		ValidationID: validationID,
		RequestID:    requestID,
		EvaluationResult: EvaluationResult{
			Decision:         decision,
			MatchedRuleIDs:   []uuid.UUID{},
			EvaluatedRuleIDs: []uuid.UUID{},
			Reason:           "",
		},
		LimitUsageDetails: []LimitUsageDetail{},
		EvaluatedAt:       evaluatedAt,
	}
}

// IsValid checks if the decision is valid
func (d Decision) IsValid() bool {
	switch d {
	case DecisionAllow, DecisionDeny, DecisionReview:
		return true
	default:
		return false
	}
}

// String returns the string representation of the decision
func (d Decision) String() string {
	return string(d)
}

// Validate checks that all required fields in ValidationRequest are present and valid.
// The now parameter is used for timestamp validation, enabling deterministic testing
// via clock injection. In production, pass time.Now() from a clock interface.
// In tests with MOCK_TIME, pass the mocked time.
// Returns specific error constants for each validation failure.
func (r *ValidationRequest) Validate(now time.Time) error {
	if r.RequestID == uuid.Nil {
		return constant.ErrValidationRequestIDRequired
	}

	if !r.TransactionType.IsValid() {
		return constant.ErrValidationInvalidTransactionType
	}

	if err := r.validateAmountCurrencyTimestamp(now); err != nil {
		return err
	}

	if r.Account.ID == uuid.Nil {
		return constant.ErrValidationAccountRequired
	}

	if err := r.validateOptionalFields(); err != nil {
		return err
	}

	if err := r.validateMerchant(); err != nil {
		return err
	}

	return r.validateMetadata()
}

// ValidateForReserve validates the request for the two-phase reserve path. It
// runs the SAME core checks as the synchronous validate path (requestId,
// positive amount, ISO-4217 currency, in-window timestamp) but relaxes two
// fields the ledger legitimately cannot supply at the reserve anchor:
//
//   - transactionType: optional. The ledger is a double-entry ledger with no
//     card-rail nature; when empty the tracer matches account-scoped limits
//     without a transaction-type constraint. When present it must still be a
//     valid type.
//   - account: optional. A ledger transaction whose only source is an external
//     account has no internal account UUID to scope on; when absent the tracer
//     matches non-account-scoped (segment/portfolio/global) limits. When
//     present it must be a non-nil UUID.
//
// The synchronous /v1/validations path keeps both fields mandatory via
// Validate; this relaxation is scoped to reserve only.
func (r *ValidationRequest) ValidateForReserve(now time.Time) error {
	if r.RequestID == uuid.Nil {
		return constant.ErrValidationRequestIDRequired
	}

	if err := r.validateAmountCurrencyTimestamp(now); err != nil {
		return err
	}

	if r.TransactionType != "" && !r.TransactionType.IsValid() {
		return constant.ErrValidationInvalidTransactionType
	}

	if err := r.validateOptionalFields(); err != nil {
		return err
	}

	if err := r.validateMerchant(); err != nil {
		return err
	}

	return r.validateMetadata()
}

// validateAmountCurrencyTimestamp validates the value/currency/timestamp core
// shared by the synchronous validate path and the reserve path: a positive
// amount, an ISO-4217 currency, and an in-window (not-future / not-too-far-past)
// timestamp. The requestId, transactionType-enum, and account-presence checks
// live in the orchestrators (Validate / ValidateForReserve) because their
// requiredness differs between the two paths.
func (r *ValidationRequest) validateAmountCurrencyTimestamp(now time.Time) error {
	if r.Amount.LessThanOrEqual(decimal.Zero) {
		return constant.ErrValidationAmountNonPositive
	}

	if r.Currency == "" {
		return constant.ErrValidationCurrencyRequired
	}

	if !pkg.IsValidCurrency(r.Currency) {
		return constant.ErrValidationInvalidCurrency
	}

	if r.TransactionTimestamp.IsZero() {
		return constant.ErrValidationTimestampRequired
	}

	// Use injected `now` instead of time.Now() for testability and MOCK_TIME support
	maxAllowedTime := now.Add(ClockSkewTolerance)
	if r.TransactionTimestamp.After(maxAllowedTime) {
		return constant.ErrValidationTimestampFuture
	}

	minAllowedTime := now.Add(-MaxTimestampAge)
	if !r.TransactionTimestamp.After(minAllowedTime) {
		return constant.ErrValidationTimestampPast
	}

	return nil
}

func (r *ValidationRequest) validateOptionalFields() error {
	if r.SubType != nil && len(*r.SubType) > MaxSubTypeLength {
		return constant.ErrValidationSubTypeTooLong
	}

	if r.Segment != nil && r.Segment.ID == uuid.Nil {
		return constant.ErrValidationSegmentIDRequired
	}

	if r.Portfolio != nil && r.Portfolio.ID == uuid.Nil {
		return constant.ErrValidationPortfolioIDRequired
	}

	if r.Account.Type != "" && !validAccountTypes[r.Account.Type] {
		return constant.ErrValidationInvalidAccountType
	}

	if r.Account.Status != "" && !validAccountStatuses[r.Account.Status] {
		return constant.ErrValidationInvalidAccountStatus
	}

	return nil
}

func (r *ValidationRequest) validateMerchant() error {
	if r.Merchant == nil {
		return nil
	}

	if r.Merchant.ID == uuid.Nil {
		return constant.ErrValidationMerchantIDRequired
	}

	if r.Merchant.Category != "" && !mccPattern.MatchString(r.Merchant.Category) {
		return constant.ErrValidationInvalidMerchantCategory
	}

	if r.Merchant.Country != "" && !countryCodePattern.MatchString(r.Merchant.Country) {
		return constant.ErrValidationInvalidMerchantCountry
	}

	return nil
}

func (r *ValidationRequest) validateMetadata() error {
	if r.Metadata == nil {
		return nil
	}

	if len(r.Metadata) > constant.MaxMetadataEntries {
		return constant.ErrMetadataEntriesExceeded
	}

	for key := range r.Metadata {
		if len(key) > constant.MaxMetadataKeyLength {
			return constant.ErrMetadataKeyLengthExceeded
		}

		if !metadataKeyPattern.MatchString(key) {
			return constant.ErrMetadataKeyInvalidChars
		}
	}

	return nil
}

// ToCheckLimitsInput converts ValidationRequest to CheckLimitsInput for limit checking.
// Used by Validation Orchestration to prepare input for Limit Checking.
func (r *ValidationRequest) ToCheckLimitsInput() *CheckLimitsInput {
	input := &CheckLimitsInput{
		Amount:               r.Amount,
		Currency:             r.Currency,
		AccountID:            r.Account.ID,
		TransactionType:      &r.TransactionType,
		SubType:              r.SubType,
		TransactionTimestamp: r.TransactionTimestamp,
	}

	if r.Segment != nil {
		input.SegmentID = &r.Segment.ID
	}

	if r.Portfolio != nil {
		input.PortfolioID = &r.Portfolio.ID
	}

	if r.Merchant != nil {
		input.MerchantID = &r.Merchant.ID
	}

	return input
}

// ToTransactionScope builds a single Scope from the ValidationRequest context fields.
// This is used for scope matching in rule evaluation - rules with specific scopes
// should only evaluate against transactions that have matching scopes.
// A transaction has exactly one scope derived from its context
// objects (Account, Segment, Portfolio, Merchant, TransactionType).
func (r *ValidationRequest) ToTransactionScope() *Scope {
	scope := &Scope{
		AccountID:       &r.Account.ID,
		TransactionType: &r.TransactionType,
		SubType:         r.SubType,
	}

	if r.Segment != nil {
		scope.SegmentID = &r.Segment.ID
	}

	if r.Portfolio != nil {
		scope.PortfolioID = &r.Portfolio.ID
	}

	if r.Merchant != nil {
		scope.MerchantID = &r.Merchant.ID
	}

	return scope
}
