// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package accountregistration provides the persistence adapter for the AccountRegistration
// saga entity. It mirrors the layout of the sibling /postgres/account package: this file
// holds the ToEntity/FromEntity translation between the domain model and the SQL row,
// and account_registration.postgresql.go holds the Repository interface + Postgres
// implementation.
package accountregistration

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// AccountRegistrationPostgreSQLModel is the database-row shape for an AccountRegistration.
// Nullable columns are represented with sql.Null* so we can round-trip NULLs without
// falling into the zero-value trap (e.g., an all-zero uuid must remain distinct from NULL).
type AccountRegistrationPostgreSQLModel struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	LedgerID        uuid.UUID
	HolderID        uuid.UUID
	IdempotencyKey  string
	RequestHash     string
	AccountID       uuid.NullUUID
	CRMAliasID      uuid.NullUUID
	Status          string
	FailureCode     sql.NullString
	FailureMessage  sql.NullString
	RetryCount      int
	NextRetryAt     sql.NullTime
	ClaimedBy       sql.NullString
	ClaimedAt       sql.NullTime
	LastRecoveredAt sql.NullTime
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     sql.NullTime
}

// ToEntity converts the SQL row representation to the mmodel domain type.
func (m *AccountRegistrationPostgreSQLModel) ToEntity() *mmodel.AccountRegistration {
	reg := &mmodel.AccountRegistration{
		ID:             m.ID,
		OrganizationID: m.OrganizationID,
		LedgerID:       m.LedgerID,
		HolderID:       m.HolderID,
		IdempotencyKey: m.IdempotencyKey,
		RequestHash:    m.RequestHash,
		Status:         mmodel.AccountRegistrationStatus(m.Status),
		RetryCount:     m.RetryCount,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}

	if m.AccountID.Valid {
		id := m.AccountID.UUID
		reg.AccountID = &id
	}

	if m.CRMAliasID.Valid {
		id := m.CRMAliasID.UUID
		reg.CRMAliasID = &id
	}

	if m.FailureCode.Valid {
		code := m.FailureCode.String
		reg.FailureCode = &code
	}

	if m.FailureMessage.Valid {
		msg := m.FailureMessage.String
		reg.FailureMessage = &msg
	}

	if m.NextRetryAt.Valid {
		t := m.NextRetryAt.Time
		reg.NextRetryAt = &t
	}

	if m.ClaimedBy.Valid {
		v := m.ClaimedBy.String
		reg.ClaimedBy = &v
	}

	if m.ClaimedAt.Valid {
		t := m.ClaimedAt.Time
		reg.ClaimedAt = &t
	}

	if m.LastRecoveredAt.Valid {
		t := m.LastRecoveredAt.Time
		reg.LastRecoveredAt = &t
	}

	if m.CompletedAt.Valid {
		t := m.CompletedAt.Time
		reg.CompletedAt = &t
	}

	return reg
}

// FromEntity populates the SQL row representation from the mmodel domain type. Callers
// are expected to have already assigned CreatedAt/UpdatedAt (rule 16: capture time.Now().UTC()
// once at the upper layer). If reg.ID is uuid.Nil, the caller is responsible for assigning
// one before calling the repository — this helper does not allocate on behalf of the caller.
func (m *AccountRegistrationPostgreSQLModel) FromEntity(reg *mmodel.AccountRegistration) {
	*m = AccountRegistrationPostgreSQLModel{
		ID:             reg.ID,
		OrganizationID: reg.OrganizationID,
		LedgerID:       reg.LedgerID,
		HolderID:       reg.HolderID,
		IdempotencyKey: reg.IdempotencyKey,
		RequestHash:    reg.RequestHash,
		Status:         string(reg.Status),
		RetryCount:     reg.RetryCount,
		CreatedAt:      reg.CreatedAt,
		UpdatedAt:      reg.UpdatedAt,
	}

	if reg.AccountID != nil {
		m.AccountID = uuid.NullUUID{UUID: *reg.AccountID, Valid: true}
	}

	if reg.CRMAliasID != nil {
		m.CRMAliasID = uuid.NullUUID{UUID: *reg.CRMAliasID, Valid: true}
	}

	if reg.FailureCode != nil {
		m.FailureCode = sql.NullString{String: *reg.FailureCode, Valid: true}
	}

	if reg.FailureMessage != nil {
		m.FailureMessage = sql.NullString{String: *reg.FailureMessage, Valid: true}
	}

	if reg.NextRetryAt != nil {
		m.NextRetryAt = sql.NullTime{Time: *reg.NextRetryAt, Valid: true}
	}

	if reg.ClaimedBy != nil {
		m.ClaimedBy = sql.NullString{String: *reg.ClaimedBy, Valid: true}
	}

	if reg.ClaimedAt != nil {
		m.ClaimedAt = sql.NullTime{Time: *reg.ClaimedAt, Valid: true}
	}

	if reg.LastRecoveredAt != nil {
		m.LastRecoveredAt = sql.NullTime{Time: *reg.LastRecoveredAt, Valid: true}
	}

	if reg.CompletedAt != nil {
		m.CompletedAt = sql.NullTime{Time: *reg.CompletedAt, Valid: true}
	}
}
