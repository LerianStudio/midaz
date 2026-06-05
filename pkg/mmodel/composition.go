// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

// CreateHolderAccountInput is the composite request body for opening a
// holder-owned account and, optionally, its instrument in a single call.
//
// The account fields MIRROR CreateAccountInput field-by-field so the composite
// binds an identical account contract; HolderID is intentionally omitted
// because the holder is sourced from the path parameter, never the body
// (no-new-null-semantics: the account is always owned by the path holder).
// The instrument fields (BankingDetails/RegulatoryFields/RelatedParties) are
// optional; when none are present the composition writes no instrument.
// The instrument's LedgerID/AccountID are NOT in the body either: the service
// threads the just-created account's IDs into the instrument create.
//
// A reflection-based drift guard (composition_test.go) asserts every
// CreateAccountInput field except HolderID is mirrored here, so a future field
// addition to CreateAccountInput breaks the build's tests instead of being
// silently dropped on the composite wire.
//
// swagger:model CreateHolderAccountInput
//
//	@Description	Composite request payload for opening a holder-owned account with an optional instrument in one call. The account fields mirror the standard account-create payload; the holder is taken from the path. When banking/regulatory/related-party fields are present an instrument is created and linked to the new account; otherwise only the account is created.
type CreateHolderAccountInput struct {
	// Human-readable name of the account
	// required: false
	// example: Corporate Checking Account
	// maxLength: 256
	Name string `json:"name" validate:"max=256" example:"Corporate Checking Account" maxLength:"256"`

	// ID of the parent account if this is a subaccount (optional)
	// required: false
	// format: uuid
	ParentAccountID *string `json:"parentAccountId" validate:"omitempty,uuid" format:"uuid"`

	// Free-form external reference for linking to external systems. This is NOT the
	// ownership link: the path holder is the formal owner of the account.
	// required: false
	// example: EXT-ACC-12345
	// maxLength: 256
	EntityID *string `json:"entityId" validate:"omitempty,max=256" example:"EXT-ACC-12345" maxLength:"256"`

	// Asset code that this account will use for balances and transactions
	// required: true
	// example: USD
	// maxLength: 100
	AssetCode string `json:"assetCode" validate:"required,max=100" example:"USD" maxLength:"100"`

	// ID of the portfolio this account belongs to (optional)
	// required: false
	// format: uuid
	PortfolioID *string `json:"portfolioId" validate:"omitempty,uuid" format:"uuid"`

	// ID of the segment this account belongs to (optional)
	// required: false
	// format: uuid
	SegmentID *string `json:"segmentId" validate:"omitempty,uuid" format:"uuid"`

	// Current operating status of the account
	// required: false
	Status Status `json:"status"`

	// Unique alias for the account (optional, must follow alias format rules)
	// required: false
	// example: @treasury_checking
	// maxLength: 100
	Alias *string `json:"alias" validate:"omitempty,max=100,prohibitedexternalaccountprefix,invalidaliascharacters" example:"@treasury_checking" maxLength:"100"`

	// Type of the account
	// required: true
	// example: deposit
	// maxLength: 256
	Type string `json:"type" validate:"required,max=256,invalidstrings=external" example:"deposit"`

	// Whether the account should start blocked
	// required: false
	// default: false
	Blocked *bool `json:"blocked"`

	// Custom key-value pairs for extending the account information
	// required: false
	// example: {"department": "Treasury", "purpose": "Operating Expenses", "region": "Global"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`

	// Object with banking information of the account. When present (or any other
	// instrument field is present) an instrument is created and linked to the
	// new account; when absent no instrument is written.
	// required: false
	BankingDetails *BankingDetails `json:"bankingDetails,omitempty"`

	// Object with regulatory fields for the instrument (optional).
	// required: false
	RegulatoryFields *RegulatoryFields `json:"regulatoryFields,omitempty"`

	// List of related parties for the instrument (optional).
	// required: false
	RelatedParties []*RelatedParty `json:"relatedParties,omitempty"`
} //	@name	CreateHolderAccountInput

// ToCreateAccountInput projects the composite body onto the account-create
// contract, attaching the path-sourced holder as the account owner. It carries
// every mirrored account field across; the instrument fields are dropped (they
// are threaded into the instrument create by the service, not the account).
//
// holderID is the path :id; the caller passes it explicitly so the account is
// always owned by the path holder and never by a body-supplied value.
func (in *CreateHolderAccountInput) ToCreateAccountInput(holderID string) *CreateAccountInput {
	return &CreateAccountInput{
		Name:            in.Name,
		ParentAccountID: in.ParentAccountID,
		EntityID:        in.EntityID,
		HolderID:        &holderID,
		AssetCode:       in.AssetCode,
		PortfolioID:     in.PortfolioID,
		SegmentID:       in.SegmentID,
		Status:          in.Status,
		Alias:           in.Alias,
		Type:            in.Type,
		Blocked:         in.Blocked,
		Metadata:        in.Metadata,
	}
}

// HolderAccountResponse is the composite response for opening a holder-owned
// account. Account is always present on success. Instrument is null when none
// was requested (account-only path) or when the instrument write failed.
// InstrumentError is set ONLY when the account committed but the instrument
// write failed: the account remains persisted and usable (no compensating
// delete), and the failure is surfaced for client-driven retry.
//
// swagger:model HolderAccountResponse
//
//	@Description	Composite response for opening a holder-owned account. Account is always present on success. Instrument is present when one was requested and created. When the account succeeded but the instrument write failed, instrumentError carries a typed, client-actionable failure block and the account remains persisted (no rollback).
type HolderAccountResponse struct {
	// The account that was created (always present on success).
	Account *Account `json:"account"`

	// The instrument that was created, or null when none was requested or the
	// instrument write failed.
	Instrument *Instrument `json:"instrument"`

	// Typed failure block, set only when the account committed but the
	// instrument write failed. Omitted on full success and on the account-only
	// path.
	InstrumentError *InstrumentFailure `json:"instrumentError,omitempty"`
} //	@name	HolderAccountResponse

// InstrumentFailure is the typed partial-failure block surfaced when the
// account committed but the instrument write failed. Reason is a stable,
// client-actionable code (never raw internal error text), so a client can
// decide to retry the standalone instrument create against the surviving
// account.
//
// swagger:model InstrumentFailure
//
//	@Description	Typed partial-failure block returned when an account was created but its instrument could not be written. Status reflects the instrument outcome; Reason is a stable, client-actionable code (not internal error text).
type InstrumentFailure struct {
	// Outcome status of the instrument write (e.g. FAILED).
	// example: FAILED
	Status string `json:"status" example:"FAILED"`

	// Stable, client-actionable reason code for the instrument-write failure.
	// example: 0001
	Reason string `json:"reason" example:"0001"`
} //	@name	InstrumentFailure
