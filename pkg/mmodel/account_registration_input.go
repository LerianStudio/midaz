// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"github.com/google/uuid"
)

// CreateAccountRegistrationInput is the public request body for the Ledger-owned
// account-registration saga. It bundles the CRM holder identity with the Ledger
// account payload and the CRM alias payload so a single API call can coordinate
// both sides atomically (from the caller's perspective).
//
// The saga (see components/ledger/internal/services/command/create_account_registration.go)
// hashes this struct canonically via pkg/utils.CanonicalHashJSON to gate idempotent
// replays: identical bodies with the same Idempotency-Key produce the stored result,
// differing bodies with the same key are rejected.
//
// swagger:model CreateAccountRegistrationInput
// @Description CreateAccountRegistrationRequest payload
type CreateAccountRegistrationInput struct {
	// Unique identifier of the holder on CRM that will own the new account.
	HolderID uuid.UUID `json:"holderId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`

	// Ledger account creation payload. Reuses CreateAccountInput so the saga honors
	// the same validation rules as the standalone POST /accounts endpoint.
	Account CreateAccountInput `json:"account" validate:"required"`

	// CRM alias creation payload. The saga populates LedgerID and AccountID from the
	// path/created account before invoking CRM, so those fields in this struct are
	// advisory — the saga overrides them to prevent client-side spoofing.
	CRMAlias CreateAliasInput `json:"crmAlias" validate:"required"`
} // @name CreateAccountRegistrationRequest
