// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// LimitType represents the period type of a limit
type LimitType string

const (
	LimitTypeDaily          LimitType = "DAILY"
	LimitTypeMonthly        LimitType = "MONTHLY"
	LimitTypePerTransaction LimitType = "PER_TRANSACTION"
	LimitTypeWeekly         LimitType = "WEEKLY"
	LimitTypeCustom         LimitType = "CUSTOM"
)

// LimitStatus represents the lifecycle status of a limit
type LimitStatus string

const (
	LimitStatusDraft    LimitStatus = "DRAFT"
	LimitStatusActive   LimitStatus = "ACTIVE"
	LimitStatusInactive LimitStatus = "INACTIVE"
	LimitStatusDeleted  LimitStatus = "DELETED"
)

// safeNameRegex validates limit names contain only safe ASCII characters.
// Allows: alphanumeric, literal spaces, hyphens, underscores, periods, parentheses.
// Prevents: XSS vectors like <script>, SQL injection attempts, and control whitespace.
// Note: Uses literal space instead of \s to reject tabs, newlines, and other control chars.
// ASCII-only by design: accented and unicode characters are rejected. This restriction is
// enforced at the API/DB boundary; callers should normalize input accordingly.
// Length is validated separately against MaxNameLength (255 bytes, ASCII-safe).
var safeNameRegex = regexp.MustCompile(`^[a-zA-Z0-9 \-_.()]+$`)

// String length constraints (aligned with database schema and HTTP validation)
const (
	MaxNameLength        = 255  // VARCHAR(255) in database
	MaxDescriptionLength = 1000 // TEXT in database, but limited for practical use
	MaxSubTypeLength     = 50   // Maximum length for transaction subType field
)

// safeDescriptionRegex validates description contains no script/HTML tags.
// More permissive than name regex but prevents XSS vectors.
// Allows: most ASCII characters except < and > which could form HTML tags.
// ASCII-only by design: while the regex doesn't explicitly block unicode, the API/DB boundary
// enforces ASCII input; callers should normalize accordingly.
// Length is validated separately against MaxDescriptionLength (1000 bytes, ASCII-safe).
var safeDescriptionRegex = regexp.MustCompile(`^[^<>]*$`)

// Limit represents a transaction limit.
// MaxAmount is expressed as a decimal value (e.g., 1000.00 for USD/BRL).
// ResetAt is calculated based on LimitType:
//   - DAILY: next midnight UTC
//   - MONTHLY: next 1st of month at midnight UTC
//   - WEEKLY: next Monday 00:00 UTC
//   - CUSTOM: customEndDate + 1 day at midnight UTC
//   - PER_TRANSACTION: null (no reset)
//
// ActiveTimeStart/ActiveTimeEnd define the daily time window when the limit is active.
// If both are nil, the limit is active 24/7.
// Overnight windows (e.g., 20:00 to 06:00) are supported.
//
// CustomStartDate/CustomEndDate define the period for CUSTOM limits.
// These are required for CUSTOM limitType and forbidden for other types.
type Limit struct {
	ID              uuid.UUID       `json:"limitId" swaggertype:"string" format:"uuid"`
	Name            string          `json:"name"`
	Description     *string         `json:"description,omitempty"`
	LimitType       LimitType       `json:"limitType"`
	MaxAmount       decimal.Decimal `json:"maxAmount" swaggertype:"string" example:"1000.00"`
	Currency        string          `json:"currency"`
	Scopes          []Scope         `json:"scopes"`
	Status          LimitStatus     `json:"status"`
	ActiveTimeStart *TimeOfDay      `json:"activeTimeStart,omitempty" swaggertype:"string" example:"09:00"`
	ActiveTimeEnd   *TimeOfDay      `json:"activeTimeEnd,omitempty" swaggertype:"string" example:"17:00"`
	CustomStartDate *time.Time      `json:"customStartDate,omitempty" format:"date-time"`
	CustomEndDate   *time.Time      `json:"customEndDate,omitempty" format:"date-time"`
	ResetAt         *time.Time      `json:"resetAt,omitempty" format:"date-time"`
	CreatedAt       time.Time       `json:"createdAt" format:"date-time"`
	UpdatedAt       time.Time       `json:"updatedAt" format:"date-time"`
	DeletedAt       *time.Time      `json:"deletedAt,omitempty" format:"date-time"`
}

// UsageCounter tracks current usage for a limit within a specific scope and period.
// CurrentUsage is expressed as a decimal value.
// Note: Remaining amount is calculated as (Limit.MaxAmount - CurrentUsage), not stored.
// ScopeKey format: "acct:abc-123", "segment:gold", "portfolio:xyz"
// PeriodKey format: "2025-12-28" for DAILY, "2025-12" for MONTHLY
type UsageCounter struct {
	ID            uuid.UUID       `json:"usageCounterId" swaggertype:"string" format:"uuid"`
	LimitID       uuid.UUID       `json:"limitId" swaggertype:"string" format:"uuid"`
	ScopeKey      string          `json:"scopeKey"`
	PeriodKey     string          `json:"periodKey"`
	CurrentUsage  decimal.Decimal `json:"currentUsage" swaggertype:"string" example:"500.00" minimum:"0"`
	LastUpdatedAt time.Time       `json:"lastUpdatedAt" format:"date-time"`
}

// ScanFields returns pointers to all fields for use with sql.Row.Scan or sql.Rows.Scan.
// Field order matches: id, limit_id, scope_key, period_key, current_usage, last_updated_at.
func (c *UsageCounter) ScanFields() []any {
	return []any{&c.ID, &c.LimitID, &c.ScopeKey, &c.PeriodKey, &c.CurrentUsage, &c.LastUpdatedAt}
}

// IsValid validates LimitType enum
func (t LimitType) IsValid() bool {
	switch t {
	case LimitTypeDaily, LimitTypeMonthly, LimitTypePerTransaction, LimitTypeWeekly, LimitTypeCustom:
		return true
	}

	return false
}

// IsValid validates LimitStatus enum
func (s LimitStatus) IsValid() bool {
	switch s {
	case LimitStatusDraft, LimitStatusActive, LimitStatusInactive, LimitStatusDeleted:
		return true
	}

	return false
}

// CalculateResetAt computes next reset time based on limit type.
// For CUSTOM limits, use CalculateCustomResetAt instead with customEndDate.
func CalculateResetAt(limitType LimitType, now time.Time) *time.Time {
	switch limitType {
	case LimitTypeDaily:
		nextDay := now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)

		return &nextDay
	case LimitTypeMonthly:
		year, month, _ := now.UTC().Date()
		nextMonth := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)

		return &nextMonth
	case LimitTypeWeekly:
		// Calculate next Monday at 00:00 UTC
		utcNow := now.UTC()

		daysUntilMonday := (8 - int(utcNow.Weekday())) % 7
		if daysUntilMonday == 0 {
			daysUntilMonday = 7 // If today is Monday, go to next Monday
		}

		nextMonday := utcNow.Truncate(24*time.Hour).AddDate(0, 0, daysUntilMonday)

		return &nextMonday
	case LimitTypePerTransaction:
		return nil
	case LimitTypeCustom:
		// CUSTOM limits need customEndDate, handled by CalculateCustomResetAt
		return nil
	default:
		return nil
	}
}

// CalculateCustomResetAt computes reset time for CUSTOM limits.
// Returns customEndDate + 1 day at midnight UTC.
func CalculateCustomResetAt(customEndDate time.Time) *time.Time {
	resetAt := customEndDate.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)

	return &resetAt
}

// validateCurrency checks if currency is a valid ISO 4217 code (3 uppercase letters)
func validateCurrency(currency string) error {
	if !pkg.IsValidCurrency(currency) {
		return constant.ErrLimitInvalidCurrency
	}

	return nil
}

// newLimitBase performs common normalization and creates base Limit struct.
// This function is private and shared by all NewLimit* constructors to reduce duplication.
// It normalizes textual inputs (name, currency, description), creates defensive copy of scopes,
// and initializes common fields (ID, Status, CreatedAt, UpdatedAt).
func newLimitBase(
	name string,
	limitType LimitType,
	maxAmount decimal.Decimal,
	currency string,
	scopes []Scope,
	description *string,
	createdAt time.Time,
) *Limit {
	now := createdAt.UTC()

	// Normalize textual inputs
	normalizedName := strings.TrimSpace(name)
	normalizedCurrency := strings.ToUpper(strings.TrimSpace(currency))

	var normalizedDescription *string

	if description != nil {
		trimmed := strings.TrimSpace(*description)
		normalizedDescription = &trimmed
	}

	// Defensive copy of scopes to prevent external mutation.
	// SubType is normalized to trimmed lowercase canonical form so DB state is
	// symmetric with runtime case-insensitive matching.
	scopesCopy := append([]Scope(nil), scopes...)
	for i := range scopesCopy {
		normalizeScopeSubType(&scopesCopy[i])
	}

	return &Limit{
		ID:          uuid.New(),
		Name:        normalizedName,
		Description: normalizedDescription,
		LimitType:   limitType,
		MaxAmount:   maxAmount,
		Currency:    normalizedCurrency,
		Scopes:      scopesCopy,
		Status:      LimitStatusDraft,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// validateScopes checks if scopes array is valid
func validateScopes(scopes []Scope) error {
	if len(scopes) == 0 {
		return constant.ErrLimitInvalidScope
	}

	for _, scope := range scopes {
		if scope.IsEmpty() {
			return constant.ErrLimitInvalidScope
		}

		if scope.TransactionType != nil && !scope.TransactionType.IsValid() {
			return constant.ErrLimitInvalidScope
		}
	}

	return nil
}

// NewLimit creates a new Limit entity with validation.
// maxAmount is a decimal value (e.g., 1000.00).
// Scopes ordering is preserved: the returned Limit.Scopes maintains the same order as the input.
// Name and description are trimmed of leading/trailing whitespace before storage.
// The caller provides the current timestamp (createdAt) to enable deterministic testing via clock injection.
func NewLimit(
	name string,
	limitType LimitType,
	maxAmount decimal.Decimal,
	currency string,
	scopes []Scope,
	description *string,
	createdAt time.Time,
) (*Limit, error) {
	limit := newLimitBase(name, limitType, maxAmount, currency, scopes, description, createdAt)

	now := createdAt.UTC()
	limit.ResetAt = CalculateResetAt(limitType, now)

	if err := limit.Validate(); err != nil {
		return nil, err
	}

	return limit, nil
}

// NewLimitWithTimeWindow creates a new Limit entity with an active time window.
// The time window restricts when the limit is evaluated during the day.
// Supports overnight windows (e.g., "20:00" to "06:00").
func NewLimitWithTimeWindow(
	name string,
	limitType LimitType,
	maxAmount decimal.Decimal,
	currency string,
	scopes []Scope,
	description *string,
	activeTimeStart string,
	activeTimeEnd string,
	createdAt time.Time,
) (*Limit, error) {
	// Parse time window strings
	startTime, err := NewTimeOfDay(activeTimeStart)
	if err != nil {
		return nil, err
	}

	endTime, err := NewTimeOfDay(activeTimeEnd)
	if err != nil {
		return nil, err
	}

	// Validate time window
	if err := ValidateTimeWindow(&startTime, &endTime); err != nil {
		return nil, err
	}

	limit := newLimitBase(name, limitType, maxAmount, currency, scopes, description, createdAt)

	now := createdAt.UTC()
	limit.ActiveTimeStart = &startTime
	limit.ActiveTimeEnd = &endTime
	limit.ResetAt = CalculateResetAt(limitType, now)

	if err := limit.Validate(); err != nil {
		return nil, err
	}

	return limit, nil
}

// NewLimitWithCustomPeriod creates a new Limit entity with a CUSTOM period.
// CUSTOM limits have a fixed start and end date and reset at customEndDate + 1 day.
func NewLimitWithCustomPeriod(
	name string,
	limitType LimitType,
	maxAmount decimal.Decimal,
	currency string,
	scopes []Scope,
	description *string,
	customStartDate time.Time,
	customEndDate time.Time,
	createdAt time.Time,
) (*Limit, error) {
	// Validate custom period
	if err := ValidateCustomPeriod(limitType, &customStartDate, &customEndDate, createdAt); err != nil {
		return nil, err
	}

	limit := newLimitBase(name, limitType, maxAmount, currency, scopes, description, createdAt)

	limit.CustomStartDate = &customStartDate
	limit.CustomEndDate = &customEndDate
	limit.ResetAt = CalculateCustomResetAt(customEndDate)

	if err := limit.Validate(); err != nil {
		return nil, err
	}

	return limit, nil
}

// NewLimitWithCustomPeriodAndTimeWindow creates a new Limit entity with both a CUSTOM period
// and an active time window. This supports AC-09: transactions must be inside BOTH the custom
// date range AND the time window to be evaluated.
func NewLimitWithCustomPeriodAndTimeWindow(
	name string,
	limitType LimitType,
	maxAmount decimal.Decimal,
	currency string,
	scopes []Scope,
	description *string,
	customStartDate time.Time,
	customEndDate time.Time,
	activeTimeStart string,
	activeTimeEnd string,
	createdAt time.Time,
) (*Limit, error) {
	if err := ValidateCustomPeriod(limitType, &customStartDate, &customEndDate, createdAt); err != nil {
		return nil, err
	}

	startTime, err := NewTimeOfDay(activeTimeStart)
	if err != nil {
		return nil, err
	}

	endTime, err := NewTimeOfDay(activeTimeEnd)
	if err != nil {
		return nil, err
	}

	if err := ValidateTimeWindow(&startTime, &endTime); err != nil {
		return nil, err
	}

	limit := newLimitBase(name, limitType, maxAmount, currency, scopes, description, createdAt)

	limit.CustomStartDate = &customStartDate
	limit.CustomEndDate = &customEndDate
	limit.ResetAt = CalculateCustomResetAt(customEndDate)
	limit.ActiveTimeStart = &startTime
	limit.ActiveTimeEnd = &endTime

	if err := limit.Validate(); err != nil {
		return nil, err
	}

	return limit, nil
}

// validateName checks if name is valid
func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return constant.ErrLimitNameRequired
	}

	if len(name) > MaxNameLength {
		return constant.ErrLimitNameTooLong
	}

	// Validate name contains only safe characters (XSS/injection prevention)
	if !safeNameRegex.MatchString(name) {
		return constant.ErrLimitNameInvalidChars
	}

	return nil
}

// validateMaxAmount checks if maxAmount is valid
func validateMaxAmount(maxAmount decimal.Decimal) error {
	if maxAmount.LessThanOrEqual(decimal.Zero) {
		return constant.ErrLimitInvalidMaxAmount
	}

	return nil
}

// validateDescription checks if description is valid (length and XSS prevention).
// Returns nil if description is nil (optional field).
func validateDescription(description *string) error {
	if description == nil {
		return nil
	}

	// Validate description length
	if len(*description) > MaxDescriptionLength {
		return constant.ErrLimitDescriptionTooLong
	}

	// Validate description contains no HTML/script tags (XSS prevention)
	if !safeDescriptionRegex.MatchString(*description) {
		return constant.ErrLimitDescriptionInvalidChars
	}

	return nil
}

// MaxCustomPeriodYears is the maximum allowed duration for CUSTOM limit periods.
const MaxCustomPeriodYears = 5

// ValidateTimeWindow validates the active time window fields.
// Both start and end must be set or both must be nil.
// Start cannot equal end (zero-width window not allowed).
func ValidateTimeWindow(start, end *TimeOfDay) error {
	// Both nil is valid (no time restriction)
	if start == nil && end == nil {
		return nil
	}

	// One set and one nil is invalid
	if (start == nil) != (end == nil) {
		return constant.ErrLimitTimeWindowMismatch
	}

	// Zero-width window is invalid
	if start.Equal(*end) {
		return constant.ErrLimitTimeWindowZeroWidth
	}

	return nil
}

// ValidateCustomPeriod validates the custom period fields for CUSTOM limits.
// For CUSTOM type: both dates are required and start must be before end.
// For non-CUSTOM type: both dates must be nil.
// The now parameter enables deterministic testing via clock injection.
func ValidateCustomPeriod(limitType LimitType, startDate, endDate *time.Time, now time.Time) error {
	if limitType == LimitTypeCustom {
		// CUSTOM requires both dates
		if startDate == nil || endDate == nil {
			return constant.ErrLimitCustomDatesRequired
		}

		// Normalize to UTC for comparison
		startUTC := startDate.UTC()
		endUTC := endDate.UTC()

		// Check end date is after start date
		if !endUTC.After(startUTC) {
			return constant.ErrLimitCustomDatesOrder
		}

		// Check duration does not exceed 5 years
		maxEndDate := startUTC.AddDate(MaxCustomPeriodYears, 0, 0)
		if endUTC.After(maxEndDate) {
			return constant.ErrLimitCustomPeriodTooLong
		}

		// Check custom period is not entirely in the past using injected time
		// NOTE: Allow custom periods that end "today" (same day) to support edge cases
		nowUTC := now.UTC()
		if endUTC.Before(nowUTC.Truncate(24 * time.Hour)) {
			return constant.ErrLimitCustomPeriodExpired
		}

		return nil
	}

	// Non-CUSTOM types must not have custom dates
	if startDate != nil || endDate != nil {
		return constant.ErrLimitCustomDatesNotAllowed
	}

	return nil
}

// Update modifies limit fields. Only non-nil parameters are updated.
// maxAmount is a decimal value (e.g., 1000.00).
// Name and description are trimmed of leading/trailing whitespace before storage.
// The caller provides the current timestamp (now) to enable deterministic testing via clock injection.
func (l *Limit) Update(
	name *string,
	maxAmount *decimal.Decimal,
	description *string,
	scopes *[]Scope,
	activeTimeStart *TimeOfDay,
	activeTimeEnd *TimeOfDay,
	customStartDate *time.Time,
	customEndDate *time.Time,
	now time.Time,
) error {
	updated := false

	if name != nil {
		normalizedName := strings.TrimSpace(*name)
		if err := validateName(normalizedName); err != nil {
			return err
		}

		l.Name = normalizedName
		updated = true
	}

	if maxAmount != nil {
		if err := validateMaxAmount(*maxAmount); err != nil {
			return err
		}

		l.MaxAmount = *maxAmount
		updated = true
	}

	if description != nil {
		normalizedDescription := strings.TrimSpace(*description)
		if err := validateDescription(&normalizedDescription); err != nil {
			return err
		}

		l.Description = &normalizedDescription
		updated = true
	}

	if scopes != nil {
		if err := validateScopes(*scopes); err != nil {
			return err
		}

		// Defensive copy to prevent external mutation.
		// SubType is normalized to trimmed lowercase canonical form so DB state is
		// symmetric with runtime case-insensitive matching.
		scopesCopy := append([]Scope(nil), *scopes...)
		for i := range scopesCopy {
			normalizeScopeSubType(&scopesCopy[i])
		}

		l.Scopes = scopesCopy
		updated = true
	}

	// Update time window if both fields provided (must be together or both nil)
	if activeTimeStart != nil || activeTimeEnd != nil {
		if err := ValidateTimeWindow(activeTimeStart, activeTimeEnd); err != nil {
			return err
		}

		l.ActiveTimeStart = activeTimeStart
		l.ActiveTimeEnd = activeTimeEnd
		updated = true
	}

	// Update custom period if both dates provided (must be together or both nil)
	if customStartDate != nil || customEndDate != nil {
		if err := ValidateCustomPeriod(l.LimitType, customStartDate, customEndDate, now); err != nil {
			return err
		}

		l.CustomStartDate = customStartDate
		l.CustomEndDate = customEndDate

		// Recalculate ResetAt for custom periods
		if customEndDate != nil {
			l.ResetAt = CalculateCustomResetAt(*customEndDate)
		}

		updated = true
	}

	if updated {
		l.UpdatedAt = now.UTC()
	}

	return nil
}

// validStatusTransitions defines allowed state transitions.
// DELETED is a terminal state - no transitions allowed from it.
// State machine (aligned with Rules):
// - DRAFT → ACTIVE (activate), DRAFT → DELETED (delete)
// - ACTIVE → INACTIVE (deactivate) - ACTIVE limits CANNOT be deleted directly
// - INACTIVE → ACTIVE (reactivate), INACTIVE → DRAFT (recovery), INACTIVE → DELETED (delete)
var validStatusTransitions = map[LimitStatus][]LimitStatus{
	LimitStatusDraft:    {LimitStatusActive, LimitStatusDeleted},
	LimitStatusActive:   {LimitStatusInactive},
	LimitStatusInactive: {LimitStatusActive, LimitStatusDraft, LimitStatusDeleted},
	LimitStatusDeleted:  {}, // Terminal state
}

// SetStatus changes the limit status with transition validation.
// Idempotent: same-status transitions are no-ops (return nil without updating timestamp).
// DELETED is a terminal state and cannot be transitioned from.
// The caller provides the current timestamp (now) to enable deterministic testing via clock injection.
func (l *Limit) SetStatus(status LimitStatus, now time.Time) error {
	if !status.IsValid() {
		return constant.ErrLimitInvalidStatusChange
	}

	// Idempotency: same status is a no-op
	if l.Status == status {
		return nil
	}

	// Check if transition is allowed
	allowedTransitions := validStatusTransitions[l.Status]
	isValidTransition := false

	for _, allowed := range allowedTransitions {
		if status == allowed {
			isValidTransition = true

			break
		}
	}

	if !isValidTransition {
		return constant.ErrLimitInvalidStatusChange
	}

	l.Status = status
	l.UpdatedAt = now.UTC()

	// Maintain DeletedAt invariant: set when DELETED, clear otherwise
	if status == LimitStatusDeleted {
		utcNow := now.UTC()
		l.DeletedAt = &utcNow
	} else {
		l.DeletedAt = nil
	}

	return nil
}

// IsActive checks if limit is currently active.
func (l *Limit) IsActive() bool {
	return l.Status == LimitStatusActive
}

// IsWithinTimeWindow checks if the given timestamp falls within the limit's active time window.
// Uses half-open interval semantics [start, end): start is inclusive, end is exclusive.
// Returns true if no time window is configured (both start and end are nil).
// Handles overnight windows (e.g., 20:00 to 06:00) correctly.
func (l *Limit) IsWithinTimeWindow(timestamp time.Time) bool {
	// No time restriction configured
	if l.ActiveTimeStart == nil && l.ActiveTimeEnd == nil {
		return true
	}

	// Should never happen if validation ran, but defensive check
	if l.ActiveTimeStart == nil || l.ActiveTimeEnd == nil {
		return true
	}

	utc := timestamp.UTC()
	currentMins := utc.Hour()*60 + utc.Minute()
	startMins := l.ActiveTimeStart.MinutesSinceMidnight()
	endMins := l.ActiveTimeEnd.MinutesSinceMidnight()

	if startMins < endMins {
		// Normal window (e.g., 09:00 to 17:00)
		return currentMins >= startMins && currentMins < endMins
	}

	// Overnight window (e.g., 20:00 to 06:00)
	return currentMins >= startMins || currentMins < endMins
}

// IsWithinCustomPeriod returns true if the given timestamp falls within the custom period [start, end).
// Returns true for non-CUSTOM limit types or if custom dates are nil (safety).
// Start is inclusive, end is exclusive.
func (l *Limit) IsWithinCustomPeriod(timestamp time.Time) bool {
	if l.LimitType != LimitTypeCustom {
		return true
	}

	if l.CustomStartDate == nil || l.CustomEndDate == nil {
		return true // Safety: don't block on data error
	}

	utc := timestamp.UTC()

	return !utc.Before(*l.CustomStartDate) && utc.Before(*l.CustomEndDate)
}

// Validate ensures Limit entity is valid.
func (l *Limit) Validate() error {
	if err := validateName(l.Name); err != nil {
		return err
	}

	if err := validateDescription(l.Description); err != nil {
		return err
	}

	if !l.LimitType.IsValid() {
		return constant.ErrLimitInvalidType
	}

	if err := validateMaxAmount(l.MaxAmount); err != nil {
		return err
	}

	if err := validateCurrency(l.Currency); err != nil {
		return err
	}

	if err := validateScopes(l.Scopes); err != nil {
		return err
	}

	if !l.Status.IsValid() {
		return constant.ErrLimitInvalidStatusChange
	}

	// Validate time window (if set)
	if err := ValidateTimeWindow(l.ActiveTimeStart, l.ActiveTimeEnd); err != nil {
		return err
	}

	// Validate custom period (required for CUSTOM, forbidden for others)
	if err := l.validateCustomPeriod(); err != nil {
		return err
	}

	// Enforce DeletedAt invariant: must be set iff status is DELETED
	if err := l.validateDeletedAtInvariant(); err != nil {
		return err
	}

	return nil
}

// validateCustomPeriod enforces the custom-period rules: the dates are
// required for CUSTOM limits, forbidden for every other limit type, and when
// present must be correctly ordered and within MaxCustomPeriodYears. Extracted
// from Validate to keep it under the gocyclo budget; behavior is unchanged.
func (l *Limit) validateCustomPeriod() error {
	if l.LimitType == LimitTypeCustom {
		if l.CustomStartDate == nil || l.CustomEndDate == nil {
			return constant.ErrLimitCustomDatesRequired
		}

		// Validate order and duration (skip expiry check - done in constructors)
		startUTC := l.CustomStartDate.UTC()
		endUTC := l.CustomEndDate.UTC()

		if !endUTC.After(startUTC) {
			return constant.ErrLimitCustomDatesOrder
		}

		maxEndDate := startUTC.AddDate(MaxCustomPeriodYears, 0, 0)
		if endUTC.After(maxEndDate) {
			return constant.ErrLimitCustomPeriodTooLong
		}
	} else if l.CustomStartDate != nil || l.CustomEndDate != nil {
		return constant.ErrLimitCustomDatesNotAllowed
	}

	return nil
}

// validateDeletedAtInvariant enforces that DeletedAt is set iff the status is
// DELETED. Extracted from Validate to keep it under the gocyclo budget;
// behavior is unchanged.
func (l *Limit) validateDeletedAtInvariant() error {
	if l.Status == LimitStatusDeleted && l.DeletedAt == nil {
		return constant.ErrLimitDeletedAtInvariant
	}

	if l.Status != LimitStatusDeleted && l.DeletedAt != nil {
		return constant.ErrLimitDeletedAtInvariant
	}

	return nil
}

// NewUsageCounter creates a new UsageCounter entity.
// Returns constant.ErrUsageCounterLimitIDRequired if limitID is uuid.Nil.
// Returns constant.ErrUsageCounterScopeKeyRequired if scopeKey is empty or whitespace-only.
// Returns constant.ErrUsageCounterPeriodKeyRequired if periodKey is empty or whitespace-only.
// ScopeKey and periodKey are trimmed of leading/trailing whitespace before storage.
func NewUsageCounter(
	limitID uuid.UUID,
	scopeKey string,
	periodKey string,
	createdAt time.Time,
) (*UsageCounter, error) {
	if limitID == uuid.Nil {
		return nil, constant.ErrUsageCounterLimitIDRequired
	}

	normalizedScopeKey := strings.TrimSpace(scopeKey)
	if normalizedScopeKey == "" {
		return nil, constant.ErrUsageCounterScopeKeyRequired
	}

	normalizedPeriodKey := strings.TrimSpace(periodKey)
	if normalizedPeriodKey == "" {
		return nil, constant.ErrUsageCounterPeriodKeyRequired
	}

	return &UsageCounter{
		ID:            uuid.New(),
		LimitID:       limitID,
		ScopeKey:      normalizedScopeKey,
		PeriodKey:     normalizedPeriodKey,
		CurrentUsage:  decimal.Zero,
		LastUpdatedAt: createdAt.UTC(),
	}, nil
}

// Increment adds amount to current usage.
// amount is a decimal value.
// Returns constant.ErrUsageCounterIncrementNonNegative if amount < 0.
func (u *UsageCounter) Increment(amount decimal.Decimal, now time.Time) error {
	if amount.IsNegative() {
		return constant.ErrUsageCounterIncrementNonNegative
	}

	if amount.IsZero() {
		return nil
	}

	u.CurrentUsage = u.CurrentUsage.Add(amount)
	u.LastUpdatedAt = now.UTC()

	return nil
}

// Validate ensures UsageCounter is valid.
// ScopeKey and PeriodKey are invalid if empty or whitespace-only.
func (u *UsageCounter) Validate() error {
	if u.LimitID == uuid.Nil {
		return constant.ErrUsageCounterLimitIDRequired
	}

	if strings.TrimSpace(u.ScopeKey) == "" {
		return constant.ErrUsageCounterScopeKeyRequired
	}

	if strings.TrimSpace(u.PeriodKey) == "" {
		return constant.ErrUsageCounterPeriodKeyRequired
	}

	if u.CurrentUsage.IsNegative() {
		return constant.ErrUsageCounterCurrentUsageNegative
	}

	return nil
}

// ListLimitsFilter defines filters for listing limits.
// Cursor-based pagination: Cursor contains base64-encoded cursor with sort info.
type ListLimitsFilter struct {
	Name        *string      `json:"name,omitempty"` // Filter by name (case-insensitive partial match / contains)
	Status      *LimitStatus `json:"status,omitempty"`
	LimitType   *LimitType   `json:"limitType,omitempty"`
	Currency    *string      `json:"currency,omitempty"`
	ScopeFilter *Scope       `json:"scopeFilter,omitempty"` // Optional scope filter for JSONB scope matching
	Limit       int          `json:"limit"`
	Cursor      string       `json:"cursor,omitempty"`
	SortBy      string       `json:"sortBy,omitempty"`
	SortOrder   string       `json:"sortOrder,omitempty"`
}

// DefaultLimitSortField is the default sort column for limit queries.
const DefaultLimitSortField = "created_at"

// validLimitSortFields defines the whitelist of valid sort fields for limits.
// This is the single source of truth - used by both model validation and repository.
// Unexported with read-only access via IsValidLimitSortField() to prevent external mutation.
var validLimitSortFields = map[string]bool{
	"name":                true,
	DefaultLimitSortField: true,
	"updated_at":          true,
	"max_amount":          true,
}

// IsValidLimitSortField checks if the given field is a valid sort field for limits.
func IsValidLimitSortField(field string) bool {
	return validLimitSortFields[field]
}

// ApplyDefaults sets default values for Limit, SortBy, and SortOrder fields.
// This method mutates the filter in-place.
// - Limit defaults to trcConstant.DefaultPaginationLimit if <= 0
// - Limit is capped at trcConstant.MaxPaginationLimit
// - SortBy defaults to "created_at" if empty (snake_case)
// - SortOrder defaults to "desc" if empty, normalized to lowercase
func (f *ListLimitsFilter) ApplyDefaults() {
	if f.Limit <= 0 {
		f.Limit = trcConstant.DefaultPaginationLimit
	} else if f.Limit > trcConstant.MaxPaginationLimit {
		f.Limit = trcConstant.MaxPaginationLimit
	}

	if f.SortBy == "" {
		f.SortBy = DefaultLimitSortField
	}
	// Note: SortBy is NOT lowercased because it uses snake_case (e.g., "created_at")

	if f.SortOrder == "" {
		f.SortOrder = string(trcConstant.Desc)
	} else {
		f.SortOrder = strings.ToLower(f.SortOrder)
	}
}

// Validate ensures ListLimitsFilter has valid values.
// Call ApplyDefaults() before Validate() if you want defaults applied.
// Returns error if Status, LimitType, Limit, SortBy, or SortOrder are invalid.
func (f *ListLimitsFilter) Validate() error {
	if f.Status != nil && !f.Status.IsValid() {
		return constant.ErrLimitInvalidStatusFilter
	}

	if f.LimitType != nil && !f.LimitType.IsValid() {
		return constant.ErrLimitInvalidTypeFilter
	}

	if f.Limit <= 0 {
		return constant.ErrPaginationLimitInvalid
	}

	if f.Limit > trcConstant.MaxPaginationLimit {
		return constant.ErrPaginationLimitExceeded
	}

	if f.SortBy != "" && !IsValidLimitSortField(f.SortBy) {
		return constant.ErrInvalidSortColumn
	}

	if f.SortOrder != "" {
		sortOrder := strings.ToLower(f.SortOrder)
		if sortOrder != string(trcConstant.Asc) && sortOrder != string(trcConstant.Desc) {
			return constant.ErrInvalidSortOrder
		}
	}

	return nil
}

// ListLimitsResult defines the result of listing limits.
type ListLimitsResult struct {
	Limits     []Limit `json:"limits"`
	NextCursor string  `json:"nextCursor,omitempty"`
	HasMore    bool    `json:"hasMore"`
}

// UsageSnapshot represents aggregated usage information for a limit.
// This is the response structure for GetLimitUsage as defined in api-design.md section 4.3.3.
// For PER_TRANSACTION limits, CurrentUsage is always 0 and ResetAt is nil.
type UsageSnapshot struct {
	// Limit identifier
	LimitID uuid.UUID `json:"limitId" swaggertype:"string" format:"uuid"`
	// Current usage amount (sum of all counters)
	CurrentUsage decimal.Decimal `json:"currentUsage" swaggertype:"string" example:"500.00"`
	// Total limit amount (from Limit.MaxAmount)
	LimitAmount decimal.Decimal `json:"limitAmount" swaggertype:"string" example:"1000.00"`
	// Usage percentage (currentUsage / limitAmount * 100)
	UtilizationPercent float64 `json:"utilizationPercent" example:"50.0"`
	// True if usage > 80%
	NearLimit bool `json:"nearLimit" example:"false"`
	// When counter resets (nil for PER_TRANSACTION)
	ResetAt *time.Time `json:"resetAt,omitempty" format:"date-time"`
}

// NearLimitThreshold is the threshold percentage (80%) above which nearLimit is true.
const NearLimitThreshold = 80.0

// NewUsageSnapshot creates a UsageSnapshot from a Limit and its usage counters.
// For PER_TRANSACTION limits, currentUsage is always 0 and resetAt is nil.
func NewUsageSnapshot(limit *Limit, counters []UsageCounter) *UsageSnapshot {
	currentUsage := decimal.Zero

	// For PER_TRANSACTION limits, currentUsage is always 0
	if limit.LimitType != LimitTypePerTransaction {
		for _, counter := range counters {
			currentUsage = currentUsage.Add(counter.CurrentUsage)
		}
	}

	// Calculate utilization percentage
	var utilizationPercent float64
	if limit.MaxAmount.IsPositive() {
		// (currentUsage / maxAmount) * 100
		utilizationPercent, _ = currentUsage.Div(limit.MaxAmount).Mul(decimal.NewFromInt(100)).Float64()
	}

	// nearLimit is true if usage > 80% (strictly greater, not >=)
	nearLimit := utilizationPercent > NearLimitThreshold

	// ResetAt is nil for PER_TRANSACTION limits
	var resetAt *time.Time
	if limit.LimitType != LimitTypePerTransaction {
		resetAt = limit.ResetAt
	}

	return &UsageSnapshot{
		LimitID:            limit.ID,
		CurrentUsage:       currentUsage,
		LimitAmount:        limit.MaxAmount,
		UtilizationPercent: utilizationPercent,
		NearLimit:          nearLimit,
		ResetAt:            resetAt,
	}
}
