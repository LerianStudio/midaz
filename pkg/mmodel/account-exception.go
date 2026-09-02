// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"time"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// AccountException is a struct designed to store Account Exception object data.
//
// An account exception carves a narrow, auditable hole in an account block: while the
// account is blocked, the operational types listed here remain permitted, optionally
// restricted to a single balance and optionally bounded by a validity window.
type AccountException struct {
	// The unique identifier of the Account Exception.
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	// format: uuid
	ID string `json:"id" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`
	// The unique identifier of the Organization that owns the exception.
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	// format: uuid
	OrganizationID string `json:"organizationId" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`
	// The unique identifier of the Ledger that owns the exception.
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	// format: uuid
	LedgerID string `json:"ledgerId" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`
	// The unique identifier of the blocked Account the exception applies to.
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	// format: uuid
	AccountID string `json:"accountId" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`
	// The operational type codes that stay permitted while the account is blocked.
	// Always present in the response and never empty.
	// example: ["PIX_IN","TED_OUT"]
	OperationalTypeCodes []string `json:"operationalTypeCodes" example:"PIX_IN,TED_OUT"`
	// The balance key the exception is restricted to. Absent means the exception applies
	// to every balance of the account.
	// example: asset-freeze
	// maxLength: 100
	BalanceKey *string `json:"balanceKey,omitempty" example:"asset-freeze" maxLength:"100"`
	// The justification recorded for the exception, kept for audit.
	// example: Judicial order 12345/2026
	// maxLength: 256
	Context string `json:"context" example:"Judicial order 12345/2026" maxLength:"256"`
	// The timestamp from which the exception takes effect. Absent means it is effective
	// as soon as it is created.
	EffectiveAt *time.Time `json:"effectiveAt,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp at which the exception stops applying. Absent means indeterminate
	// validity: the exception stands until it is deleted.
	ExpiresAt *time.Time `json:"expiresAt,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the account exception was created.
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the account exception was last updated.
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the account exception was deleted.
	DeletedAt *time.Time `json:"deletedAt,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`
}

// CreateAccountExceptionInput is a struct designed to store Account Exception input data.
type CreateAccountExceptionInput struct {
	// The operational type codes that stay permitted while the account is blocked.
	// At least one code is required and each code must be non-blank.
	// required: true
	// example: ["PIX_IN","TED_OUT"]
	OperationalTypeCodes []string `json:"operationalTypeCodes" validate:"required,min=1,dive,nowhitespaces,max=100" example:"PIX_IN,TED_OUT"`
	// The balance key the exception is restricted to. Omit it to apply the exception to
	// every balance of the account.
	// required: false
	// maxLength: 100
	// example: asset-freeze
	BalanceKey *string `json:"balanceKey,omitempty" validate:"omitempty,nowhitespaces,max=100" example:"asset-freeze" maxLength:"100"`
	// The justification recorded for the exception, kept for audit.
	// required: true
	// maxLength: 256
	// example: Judicial order 12345/2026
	Context string `json:"context" validate:"required,max=256" example:"Judicial order 12345/2026" maxLength:"256"`
	// The timestamp from which the exception takes effect. Omit it to make the exception
	// effective as soon as it is created.
	// required: false
	EffectiveAt *time.Time `json:"effectiveAt,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp at which the exception stops applying. Omit it for indeterminate
	// validity. When both bounds are sent, expiresAt must be later than effectiveAt.
	// required: false
	ExpiresAt *time.Time `json:"expiresAt,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`
}

// UpdateAccountExceptionInput is a struct designed to store Account Exception input data.
//
// Every field is optional and PATCH semantics apply: a field absent from the payload
// leaves the stored value unchanged.
type UpdateAccountExceptionInput struct {
	// The operational type codes that stay permitted while the account is blocked.
	// Absent leaves the stored codes unchanged; when present the array replaces them
	// wholesale and must still carry at least one non-blank code.
	// required: false
	// example: ["PIX_IN","TED_OUT"]
	OperationalTypeCodes []string `json:"operationalTypeCodes,omitempty" validate:"omitempty,min=1,dive,nowhitespaces,max=100" example:"PIX_IN,TED_OUT"`
	// The balance key the exception is restricted to. Absent leaves the stored key
	// unchanged; an empty string ("") CLEARS the restriction, widening the scope back to
	// every balance of the account.
	//
	// The tag is nowhitespacesorempty rather than nowhitespaces precisely because of that
	// clear sentinel: omitempty does NOT skip a non-nil pointer to "", so nowhitespaces
	// would reject the one value the clear semantics are expressed with. A non-empty key
	// is held to the same no-whitespace rule as on create.
	// required: false
	// maxLength: 100
	// example: asset-freeze
	BalanceKey *string `json:"balanceKey,omitempty" validate:"omitempty,nowhitespacesorempty,max=100" example:"asset-freeze" maxLength:"100"`
	// The justification recorded for the exception. Absent leaves it unchanged.
	// required: false
	// maxLength: 256
	// example: Judicial order 12345/2026
	Context *string `json:"context,omitempty" validate:"omitempty,max=256" example:"Judicial order 12345/2026" maxLength:"256"`
	// The timestamp from which the exception takes effect. Absent leaves it unchanged.
	// required: false
	EffectiveAt *time.Time `json:"effectiveAt,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp at which the exception stops applying. Absent leaves it unchanged.
	// The merged result must still satisfy expiresAt later than effectiveAt.
	// required: false
	ExpiresAt *time.Time `json:"expiresAt,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`
}

// ValidateAccountExceptionWindow enforces the cross-field invariant of the account
// exception validity window: when BOTH bounds are present, the expiration must be
// strictly after the start.
//
// A single bound is always valid — an absent effectiveAt means "effective now" and an
// absent expiresAt means indeterminate validity — so only the both-present case can
// fail. The comparison is on the instant, so two zones describing the same moment are
// still rejected as a zero-length window.
//
// Callers MUST run this over the FINAL state: the input on create, and the merged
// result on update, since a PATCH that moves only one bound can invert a window that
// was valid when it was stored.
//
// Parameters:
//   - effectiveAt: the start of the window, or nil when unbounded.
//   - expiresAt: the end of the window, or nil when unbounded.
//
// Returns:
//   - error: ErrInvalidAccountExceptionValidityWindow (0505) rendered as a
//     ValidationError when the window is not strictly positive, nil otherwise.
func ValidateAccountExceptionWindow(effectiveAt, expiresAt *time.Time) error {
	if effectiveAt == nil || expiresAt == nil {
		return nil
	}

	if !expiresAt.After(*effectiveAt) {
		return pkg.ValidateBusinessError(constant.ErrInvalidAccountExceptionValidityWindow, constant.EntityAccountException)
	}

	return nil
}
