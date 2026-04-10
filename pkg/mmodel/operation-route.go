// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AccountingRubric represents an accounting rubric with a code and description.
//
// @Description AccountingRubric object containing the code and description for a debit or credit entry.
type AccountingRubric struct {
	// The accounting rubric code.
	Code string `json:"code" validate:"required,max=50" msgpack:"code" example:"1001"`
	// The accounting rubric description.
	Description string `json:"description" validate:"required,max=250" msgpack:"description" example:"Cash"`
} // @name AccountingRubric

// AccountingEntry represents a single accounting entry with debit and credit rubrics.
//
// @Description AccountingEntry object containing debit and credit rubrics for a specific action.
// @Description
// @Description Field requirements depend on the parent operationType and scenario:
// @Description   - source + direct or commit: debit is REQUIRED, credit is optional.
// @Description   - source + hold or cancel: both debit AND credit are REQUIRED.
// @Description   - destination + direct or commit: credit is REQUIRED, debit is optional.
// @Description   - destination + hold or cancel: both debit AND credit are REQUIRED.
// @Description   - bidirectional (all scenarios): both debit AND credit are REQUIRED.
// @Description   - revert: only allowed on bidirectional routes; both debit AND credit are REQUIRED.
// @Description
// @Description An entry with neither debit nor credit is always rejected.
type AccountingEntry struct {
	// The debit rubric for this entry. Required based on operationType/scenario matrix (see type description).
	Debit *AccountingRubric `json:"debit" validate:"omitempty" msgpack:"debit"`
	// The credit rubric for this entry. Required based on operationType/scenario matrix (see type description).
	Credit *AccountingRubric `json:"credit" validate:"omitempty" msgpack:"credit"`
} // @name AccountingEntry

// AccountingEntries groups accounting entries by transaction action type.
//
// @Description AccountingEntries object containing optional accounting entries for each action type (direct, hold, commit, cancel, revert).
type AccountingEntries struct {
	// The accounting entry for the direct action.
	Direct *AccountingEntry `json:"direct,omitempty" msgpack:"direct"`
	// The accounting entry for the hold action.
	Hold *AccountingEntry `json:"hold,omitempty" msgpack:"hold"`
	// The accounting entry for the commit action.
	Commit *AccountingEntry `json:"commit,omitempty" msgpack:"commit"`
	// The accounting entry for the cancel action.
	Cancel *AccountingEntry `json:"cancel,omitempty" msgpack:"cancel"`
	// The accounting entry for the revert action.
	Revert *AccountingEntry `json:"revert,omitempty" msgpack:"revert"`
} // @name AccountingEntries

// Actions returns the action names for which this AccountingEntries has non-nil entries.
func (ae *AccountingEntries) Actions() []string {
	if ae == nil {
		return nil
	}

	var actions []string

	if ae.Direct != nil {
		actions = append(actions, "direct")
	}

	if ae.Hold != nil {
		actions = append(actions, "hold")
	}

	if ae.Commit != nil {
		actions = append(actions, "commit")
	}

	if ae.Cancel != nil {
		actions = append(actions, "cancel")
	}

	if ae.Revert != nil {
		actions = append(actions, "revert")
	}

	return actions
}

// OperationRoute is a struct designed to store Operation Route object data.
//
// swagger:model OperationRoute
// @Description OperationRoute object
type OperationRoute struct {
	// The unique identifier of the Operation Route.
	ID uuid.UUID `json:"id,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// The unique identifier of the Organization.
	OrganizationID uuid.UUID `json:"organizationId,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// The unique identifier of the Ledger.
	LedgerID uuid.UUID `json:"ledgerId,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// Short text summarizing the purpose of the operation. Used as an entry note for identification.
	Title string `json:"title,omitempty" example:"Cashin from service charge"`
	// Detailed description of the operation route purpose and usage.
	Description string `json:"description,omitempty" example:"This operation route handles cash-in transactions from service charge collections"`
	// Deprecated: external reference code kept for backward compatibility. Use the rubric codes inside accountingEntries instead.
	// example: EXT-001
	// deprecated: true
	Code string `json:"code,omitempty" example:"EXT-001"`
	// The type of the operation route.
	OperationType string `json:"operationType,omitempty" example:"source" enums:"source,destination,bidirectional"`
	// Optional accounting entries for each action type associated with this operation route.
	AccountingEntries *AccountingEntries `json:"accountingEntries,omitempty"`
	// AccountingEntriesRaw holds the raw JSON for accountingEntries for merge-patch updates.
	// This is not serialized to API responses; it is only used internally during updates.
	AccountingEntriesRaw json.RawMessage `json:"-"`
	// Additional metadata stored as JSON
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	// The account selection rule configuration.
	Account *AccountRule `json:"account,omitempty"`
	// The timestamp when the operation route was created.
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the operation route was last updated.
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the operation route was deleted.
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
} // @name OperationRoute

// CreateOperationRouteInput is a struct designed to store Operation Route input data.
//
// @Description CreateOperationRouteInput payload for creating a new Operation Route with title, description, operation type, and optional account rules.
type CreateOperationRouteInput struct {
	// Short text summarizing the purpose of the operation. Used as an entry note for identification.
	Title string `json:"title,omitempty" validate:"required,max=255" example:"Cashin from service charge"`
	// Detailed description of the operation route purpose and usage.
	Description string `json:"description,omitempty" validate:"max=250" example:"This operation route handles cash-in transactions from service charge collections"`
	// Deprecated: external reference code kept for backward compatibility. Use the rubric codes inside accountingEntries instead.
	// example: EXT-001
	// maxLength: 100
	// deprecated: true
	Code string `json:"code,omitempty" validate:"max=100" example:"EXT-001"`
	// The type of the operation route.
	OperationType string `json:"operationType,omitempty" validate:"required" example:"source" enum:"source,destination,bidirectional"`
	// Optional accounting entries for each action type associated with this operation route.
	AccountingEntries *AccountingEntries `json:"accountingEntries,omitempty"`
	// Additional metadata stored as JSON
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	// The account selection rule configuration.
	Account *AccountRule `json:"account,omitempty"`
} // @name CreateOperationRouteInput

// UpdateOperationRouteInput is a struct designed to store Operation Route input data.
//
// swagger:model UpdateOperationRouteInput
// @Description UpdateOperationRouteInput payload
type UpdateOperationRouteInput struct {
	// Short text summarizing the purpose of the operation. Used as an entry note for identification.
	Title string `json:"title,omitempty" validate:"max=255" example:"Cashin from service charge"`
	// Detailed description of the operation route purpose and usage.
	Description string `json:"description,omitempty" validate:"max=250" example:"This operation route handles cash-in transactions from service charge collections"`
	// Deprecated: external reference code kept for backward compatibility. Use the rubric codes inside accountingEntries instead.
	// example: EXT-001
	// maxLength: 100
	// deprecated: true
	Code string `json:"code,omitempty" validate:"max=100" example:"EXT-001"`
	// Optional accounting entries for each action type associated with this operation route.
	AccountingEntries *AccountingEntries `json:"accountingEntries,omitempty"`
	// AccountingEntriesRaw holds the raw JSON for accountingEntries exactly as sent in the request.
	// This preserves explicit null values for RFC 7396 JSON Merge Patch semantics,
	// allowing the repository to distinguish between "field absent" (keep existing)
	// and "field: null" (remove entry).
	AccountingEntriesRaw json.RawMessage `json:"-"`
	// Additional metadata stored as JSON
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	// The account selection rule configuration.
	Account *AccountRule `json:"account,omitempty"`
} // @name UpdateOperationRouteInput

// AccountRule represents the account selection rule configuration.
//
// @Description AccountRule object containing the rule type and condition for account selection in operation routes.
type AccountRule struct {
	// The rule type for account selection.
	RuleType string `json:"ruleType,omitempty" example:"alias" enum:"alias,account_type"`
	// The rule condition for account selection. String for alias type (e.g. "@cash_account"), array for account_type.
	ValidIf any `json:"validIf,omitempty"`
} // @name AccountRule
