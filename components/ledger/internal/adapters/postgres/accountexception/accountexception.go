// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountexception

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// emptyOperationalTypeCodes is the value written to operational_type_codes when
// the entity carries no codes. The column is JSONB NOT NULL (migration 000022),
// and json.Marshal of a nil []string produces the literal `null`, which is a
// legal JSONB scalar but not the empty collection the domain means. Persisting
// `[]` keeps every stored row an array, so readers never have to distinguish a
// JSON null from a missing list.
var emptyOperationalTypeCodes = []byte(`[]`)

// AccountExceptionPostgreSQLModel represents the database model for account
// exceptions, mirroring the account_exception table created by migration
// 000022.
//
// The scope identifiers are held as strings rather than uuid.UUID because
// mmodel.AccountException exposes them as strings, exactly like mmodel.Account
// and its AccountPostgreSQLModel counterpart. Keeping the same representation
// on both sides means FromEntity stays total: there is no parse that could fail
// inside a signature that cannot report an error.
type AccountExceptionPostgreSQLModel struct {
	ID                   string         `db:"id"`
	OrganizationID       string         `db:"organization_id"`
	LedgerID             string         `db:"ledger_id"`
	AccountID            string         `db:"account_id"`
	OperationalTypeCodes []byte         `db:"operational_type_codes"`
	BalanceKey           sql.NullString `db:"balance_key"`
	Context              string         `db:"context"`
	EffectiveAt          sql.NullTime   `db:"effective_at"`
	ExpiresAt            sql.NullTime   `db:"expires_at"`
	CreatedAt            time.Time      `db:"created_at"`
	UpdatedAt            time.Time      `db:"updated_at"`
	DeletedAt            sql.NullTime   `db:"deleted_at"`
}

// ToEntity converts the database model to a domain model.
//
// Every nullable column maps to a nil pointer on the entity, which is what the
// domain reads as "unbounded": a NULL balance_key applies the exception to every
// balance, a NULL effective_at makes it effective immediately, and a NULL
// expires_at leaves its validity indeterminate.
//
// Returns:
//   - *mmodel.AccountException: the domain entity, or nil when the receiver is nil.
func (m *AccountExceptionPostgreSQLModel) ToEntity() *mmodel.AccountException {
	if m == nil {
		return nil
	}

	e := &mmodel.AccountException{
		ID:             m.ID,
		OrganizationID: m.OrganizationID,
		LedgerID:       m.LedgerID,
		AccountID:      m.AccountID,
		Context:        m.Context,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}

	if len(m.OperationalTypeCodes) > 0 {
		var codes []string
		// A row whose JSONB cannot be decoded degrades to no codes rather than
		// failing the read: the mapping has no error channel, and a panic here
		// would take down a request over one malformed row.
		if err := json.Unmarshal(m.OperationalTypeCodes, &codes); err == nil {
			e.OperationalTypeCodes = codes
		}
	}

	if m.BalanceKey.Valid {
		balanceKey := m.BalanceKey.String
		e.BalanceKey = &balanceKey
	}

	if m.EffectiveAt.Valid {
		effectiveAt := m.EffectiveAt.Time
		e.EffectiveAt = &effectiveAt
	}

	if m.ExpiresAt.Valid {
		expiresAt := m.ExpiresAt.Time
		e.ExpiresAt = &expiresAt
	}

	if m.DeletedAt.Valid {
		deletedAt := m.DeletedAt.Time
		e.DeletedAt = &deletedAt
	}

	return e
}

// FromEntity converts a domain model to the database model.
//
// A nil pointer on the entity becomes an invalid sql.Null* — SQL NULL — while a
// non-nil pointer to the zero value stays a written value: a balanceKey of ""
// is the cleared-restriction sentinel the update contract uses, and collapsing
// it to NULL would be indistinguishable from "leave unchanged".
//
// Parameters:
//   - e: the domain entity; a nil entity is a no-op.
func (m *AccountExceptionPostgreSQLModel) FromEntity(e *mmodel.AccountException) {
	if e == nil {
		return
	}

	m.ID = e.ID
	m.OrganizationID = e.OrganizationID
	m.LedgerID = e.LedgerID
	m.AccountID = e.AccountID
	m.Context = e.Context
	m.CreatedAt = e.CreatedAt
	m.UpdatedAt = e.UpdatedAt

	m.OperationalTypeCodes = emptyOperationalTypeCodes

	if len(e.OperationalTypeCodes) > 0 {
		if data, err := json.Marshal(e.OperationalTypeCodes); err == nil {
			m.OperationalTypeCodes = data
		}
	}

	if e.BalanceKey != nil {
		m.BalanceKey = sql.NullString{String: *e.BalanceKey, Valid: true}
	} else {
		m.BalanceKey = sql.NullString{}
	}

	if e.EffectiveAt != nil {
		m.EffectiveAt = sql.NullTime{Time: *e.EffectiveAt, Valid: true}
	} else {
		m.EffectiveAt = sql.NullTime{}
	}

	if e.ExpiresAt != nil {
		m.ExpiresAt = sql.NullTime{Time: *e.ExpiresAt, Valid: true}
	} else {
		m.ExpiresAt = sql.NullTime{}
	}

	if e.DeletedAt != nil {
		m.DeletedAt = sql.NullTime{Time: *e.DeletedAt, Valid: true}
	} else {
		m.DeletedAt = sql.NullTime{}
	}
}
