// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// AccountRegistrationStatus enumerates the states of a Ledger-owned account-registration
// saga. The saga orchestrates the coordinated creation of a Ledger account and its CRM
// holder alias so that a customer account cannot transact until both sides are consistent.
//
// The ten values below form a directed state machine whose transitions are enforced by
// the saga orchestrator (Phase 4) and the recovery worker (Phase 5). Values are persisted
// as plain strings in the account_registration table and must match the CHECK constraint
// declared in the corresponding migration.
type AccountRegistrationStatus string

const (
	// AccountRegistrationReceived indicates the request has been accepted and persisted,
	// but no downstream work has started yet.
	AccountRegistrationReceived AccountRegistrationStatus = "RECEIVED"

	// AccountRegistrationHolderValidated indicates the CRM holder was fetched and
	// validated to exist and be eligible for a new alias.
	AccountRegistrationHolderValidated AccountRegistrationStatus = "HOLDER_VALIDATED"

	// AccountRegistrationLedgerAccountCreated indicates the Ledger account row was
	// created in PENDING_CRM_LINK state (not yet transactable).
	AccountRegistrationLedgerAccountCreated AccountRegistrationStatus = "LEDGER_ACCOUNT_CREATED"

	// AccountRegistrationCRMAliasCreated indicates the CRM holder alias was created
	// (or confirmed to exist, for replayed requests) and references the Ledger account.
	AccountRegistrationCRMAliasCreated AccountRegistrationStatus = "CRM_ALIAS_CREATED"

	// AccountRegistrationAccountActivated indicates the Ledger account transitioned from
	// PENDING_CRM_LINK to ACTIVE with blocked=false, and the default balance was unblocked.
	AccountRegistrationAccountActivated AccountRegistrationStatus = "ACCOUNT_ACTIVATED"

	// AccountRegistrationCompleted indicates every orchestration step succeeded and the
	// saga is finalized. Terminal success state.
	AccountRegistrationCompleted AccountRegistrationStatus = "COMPLETED"

	// AccountRegistrationCompensating indicates the saga encountered an unrecoverable
	// failure and is rolling back any work that was already committed.
	AccountRegistrationCompensating AccountRegistrationStatus = "COMPENSATING"

	// AccountRegistrationCompensated indicates compensation completed and the system is
	// back to a consistent state. Terminal failure state.
	AccountRegistrationCompensated AccountRegistrationStatus = "COMPENSATED"

	// AccountRegistrationFailedRetryable indicates a transient failure; the recovery
	// worker should retry the saga after NextRetryAt.
	AccountRegistrationFailedRetryable AccountRegistrationStatus = "FAILED_RETRYABLE"

	// AccountRegistrationFailedTerminal indicates a terminal failure that cannot be
	// retried (for example: holder not found, idempotency conflict). No further automated
	// attempts will be made. Terminal failure state.
	AccountRegistrationFailedTerminal AccountRegistrationStatus = "FAILED_TERMINAL"
)

// AccountRegistration is the durable state record for one account-opening saga. Every
// orchestration attempt is keyed by (OrganizationID, LedgerID, IdempotencyKey) and carries
// a canonical RequestHash so replayed requests with the same payload are idempotent and
// replayed requests with a different payload are rejected.
//
// Following the mmodel package convention, this struct carries no business logic; it is
// a pure data container consumed by the repository (pkg/adapters/postgres/accountregistration)
// and the saga orchestrator.
type AccountRegistration struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	LedgerID        uuid.UUID
	HolderID        uuid.UUID
	IdempotencyKey  string
	RequestHash     string // lowercase hex SHA-256 of the canonical request body
	AccountID       *uuid.UUID
	CRMAliasID      *uuid.UUID
	Status          AccountRegistrationStatus
	FailureCode     *string
	FailureMessage  *string
	RetryCount      int
	NextRetryAt     *time.Time
	ClaimedBy       *string
	ClaimedAt       *time.Time
	LastRecoveredAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
}
