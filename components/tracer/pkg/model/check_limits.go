// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// CheckLimitsInput represents the input for limit checking operations.
// Amount is expressed as a decimal value (e.g., 1000.00 for USD/BRL).
// AccountID is required; SegmentID, PortfolioID, MerchantID, TransactionType and SubType are optional for scope matching.
type CheckLimitsInput struct {
	Amount               decimal.Decimal  `json:"amount"`
	Currency             string           `json:"currency"`
	AccountID            uuid.UUID        `json:"accountId"`
	SegmentID            *uuid.UUID       `json:"segmentId,omitempty"`
	PortfolioID          *uuid.UUID       `json:"portfolioId,omitempty"`
	MerchantID           *uuid.UUID       `json:"merchantId,omitempty"`
	TransactionType      *TransactionType `json:"transactionType,omitempty" swaggertype:"string" enums:"CARD,WIRE,PIX,CRYPTO" example:"CARD"`
	SubType              *string          `json:"subType,omitempty" maxLength:"50"`
	TransactionTimestamp time.Time        `json:"transactionTimestamp"`
}

// NewCheckLimitsInput creates a new CheckLimitsInput with validation.
// Currency is normalized to uppercase.
// Amount must be positive.
// AccountID is required.
// SegmentID, PortfolioID, MerchantID, transactionType and subType are optional scope fields.
func NewCheckLimitsInput(amount decimal.Decimal, currency string, accountID uuid.UUID, segmentID, portfolioID, merchantID *uuid.UUID, transactionType *TransactionType, subType *string, timestamp time.Time) (*CheckLimitsInput, error) {
	normalizedCurrency := strings.ToUpper(strings.TrimSpace(currency))

	input := &CheckLimitsInput{
		Amount:               amount,
		Currency:             normalizedCurrency,
		AccountID:            accountID,
		SegmentID:            segmentID,
		PortfolioID:          portfolioID,
		MerchantID:           merchantID,
		TransactionType:      transactionType,
		SubType:              subType,
		TransactionTimestamp: timestamp,
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	return input, nil
}

// Validate ensures CheckLimitsInput has valid values for the synchronous
// check-limits path, where an account is mandatory.
// Returns ErrCheckLimitsNilInput if called on a nil receiver.
func (i *CheckLimitsInput) Validate() error {
	return i.validate(true)
}

// ValidateForReserve runs the same checks as Validate but permits a nil
// AccountID. The two-phase reserve path (ValidationRequest.ValidateForReserve)
// accepts a transaction whose only source is an external account, which has no
// internal account UUID to scope on; a nil account then matches only
// non-account-scoped limits. Account presence on the synchronous /validations
// path is still enforced upstream by ValidationRequest.Validate, so the
// requirement lives in the orchestrators, not here.
func (i *CheckLimitsInput) ValidateForReserve() error {
	return i.validate(false)
}

func (i *CheckLimitsInput) validate(requireAccount bool) error {
	if i == nil {
		return constant.ErrCheckLimitsNilInput
	}

	if i.Amount.LessThanOrEqual(decimal.Zero) {
		return constant.ErrCheckLimitsInvalidAmount
	}

	if !pkg.IsValidCurrency(i.Currency) {
		return constant.ErrCheckLimitsInvalidCurrency
	}

	if requireAccount && i.AccountID == uuid.Nil {
		return constant.ErrCheckLimitsInvalidAccountID
	}

	if i.SegmentID != nil && *i.SegmentID == uuid.Nil {
		return constant.ErrCheckLimitsInvalidSegmentID
	}

	if i.PortfolioID != nil && *i.PortfolioID == uuid.Nil {
		return constant.ErrCheckLimitsInvalidPortfolioID
	}

	if i.MerchantID != nil && *i.MerchantID == uuid.Nil {
		return constant.ErrCheckLimitsInvalidMerchantID
	}

	if i.TransactionTimestamp.IsZero() {
		return constant.ErrCheckLimitsInvalidTimestamp
	}

	if i.TransactionType != nil && !i.TransactionType.IsValid() {
		return constant.ErrCheckLimitsInvalidTransactionType
	}

	if i.SubType != nil && len(*i.SubType) > MaxSubTypeLength {
		return constant.ErrCheckLimitsInvalidSubType
	}

	return nil
}

// CheckLimitsOutput represents the result of limit checking operations.
// Allowed indicates if the transaction can proceed (no limits exceeded).
// ExceededLimitIDs contains IDs of limits that would be exceeded.
// LimitUsageDetails contains usage information for all checked limits.
// EvaluatedAt is the server timestamp when the limit check was performed.
type CheckLimitsOutput struct {
	Allowed           bool               `json:"allowed"`
	ExceededLimitIDs  []uuid.UUID        `json:"exceededLimitIds"`
	LimitUsageDetails []LimitUsageDetail `json:"limitUsageDetails"`
	EvaluatedAt       time.Time          `json:"evaluatedAt" format:"date-time"`
}

// NewCheckLimitsOutput creates a new CheckLimitsOutput with initialized slices.
// Ensures JSON serialization produces [] instead of null for empty arrays.
// evaluatedAt is the server timestamp when the limit check was performed.
func NewCheckLimitsOutput(allowed bool, evaluatedAt time.Time) *CheckLimitsOutput {
	return &CheckLimitsOutput{
		Allowed:           allowed,
		ExceededLimitIDs:  []uuid.UUID{},
		LimitUsageDetails: []LimitUsageDetail{},
		EvaluatedAt:       evaluatedAt,
	}
}

// WithExceededLimits adds exceeded limit IDs to the output.
// Defensively handles nil receiver and nil input to ensure JSON serializes as [] instead of null.
// Returns self for method chaining; allocates new CheckLimitsOutput if receiver is nil.
func (o *CheckLimitsOutput) WithExceededLimits(ids []uuid.UUID) *CheckLimitsOutput {
	if o == nil {
		o = &CheckLimitsOutput{}
	}

	if ids == nil {
		o.ExceededLimitIDs = []uuid.UUID{}
	} else {
		o.ExceededLimitIDs = append([]uuid.UUID(nil), ids...)
	}

	return o
}

// WithLimitUsageDetails adds limit usage details to the output.
// Defensively handles nil receiver and nil input to ensure JSON serializes as [] instead of null.
// Returns self for method chaining; allocates new CheckLimitsOutput if receiver is nil.
func (o *CheckLimitsOutput) WithLimitUsageDetails(details []LimitUsageDetail) *CheckLimitsOutput {
	if o == nil {
		o = &CheckLimitsOutput{}
	}

	if details == nil {
		o.LimitUsageDetails = []LimitUsageDetail{}
	} else {
		o.LimitUsageDetails = append([]LimitUsageDetail(nil), details...)
	}

	return o
}

// RemainingAmount calculates remaining amount before limit is reached.
// Returns a value clamped between 0 and LimitAmount:
//   - If receiver is nil, returns 0
//   - If limit is exceeded (CurrentUsage > LimitAmount), returns 0
//   - If no usage yet (CurrentUsage <= 0), returns LimitAmount
//   - Otherwise, returns LimitAmount - CurrentUsage
func (d *LimitUsageDetail) RemainingAmount() decimal.Decimal {
	// Nil receiver protection
	if d == nil {
		return decimal.Zero
	}

	// If CurrentUsage is zero or negative, remaining is the full limit
	if d.CurrentUsage.LessThanOrEqual(decimal.Zero) {
		return d.LimitAmount
	}

	// If limit is exceeded, remaining is 0
	if d.CurrentUsage.GreaterThanOrEqual(d.LimitAmount) {
		return decimal.Zero
	}

	return d.LimitAmount.Sub(d.CurrentUsage)
}

// CalculatePeriodKey computes the period key for a given limit type and timestamp.
// Format:
//   - DAILY: "2025-12-28"
//   - MONTHLY: "2025-12"
//   - WEEKLY: "2025-W03" (ISO week format: year-week number)
//   - CUSTOM: "custom" (limit checker uses customStartDate/customEndDate to determine if in period)
//   - PER_TRANSACTION: "" (empty, no period tracking)
//
// Returns ErrCheckLimitsUnknownLimitType for unknown limit types to prevent
// silent bugs where new limit types would be treated as PER_TRANSACTION.
func CalculatePeriodKey(limitType LimitType, timestamp time.Time) (string, error) {
	utc := timestamp.UTC()

	switch limitType {
	case LimitTypeDaily:
		return utc.Format("2006-01-02"), nil
	case LimitTypeMonthly:
		return utc.Format("2006-01"), nil
	case LimitTypeWeekly:
		// ISO week format: "2025-W03" (year-week number)
		year, week := utc.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week), nil
	case LimitTypeCustom:
		// Custom periods use "custom" as the period key
		// The limit_checker will use customStartDate/customEndDate to determine if transaction is within period
		return "custom", nil
	case LimitTypePerTransaction:
		return "", nil
	default:
		return "", fmt.Errorf("%w: %s", constant.ErrCheckLimitsUnknownLimitType, limitType)
	}
}

// CalculateScopeKey computes a deterministic scope key for usage tracking.
// Format: "prefix:uuid|prefix:uuid|..." ordered alphabetically by prefix.
// Prefixes: acct (account), merch (merchant), port (portfolio), seg (segment).
// Returns trcConstant.GlobalScopeKey for empty scopes.
func CalculateScopeKey(scope *Scope) string {
	if scope == nil || scope.IsEmpty() {
		return trcConstant.GlobalScopeKey
	}

	var parts []string

	if scope.AccountID != nil {
		parts = append(parts, "acct:"+scope.AccountID.String())
	}

	if scope.PortfolioID != nil {
		parts = append(parts, "port:"+scope.PortfolioID.String())
	}

	if scope.SegmentID != nil {
		parts = append(parts, "seg:"+scope.SegmentID.String())
	}

	if scope.MerchantID != nil {
		parts = append(parts, "merch:"+scope.MerchantID.String())
	}

	// Sort for deterministic key generation
	sort.Strings(parts)

	return strings.Join(parts, "|")
}
