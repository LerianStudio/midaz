// Package operationroute provides PostgreSQL adapter implementations for operation route management.
// It contains database models and conversion utilities for storing and retrieving
// operation route configurations that define how operations should be processed.
package operationroute

import (
	"database/sql"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// OperationRoutePostgreSQLModel represents the database model for operation routes
type OperationRoutePostgreSQLModel struct {
	ID                 uuid.UUID      `db:"id"`
	OrganizationID     uuid.UUID      `db:"organization_id"`
	LedgerID           uuid.UUID      `db:"ledger_id"`
	Title              string         `db:"title"`
	Description        string         `db:"description"`
	Code               sql.NullString `db:"code"`
	OperationType      string         `db:"operation_type"`
	AccountRuleType    string         `db:"account_rule_type"`
	AccountRuleValidIf string         `db:"account_rule_valid_if"`
	CreatedAt          time.Time      `db:"created_at"`
	UpdatedAt          time.Time      `db:"updated_at"`
	DeletedAt          sql.NullTime   `db:"deleted_at"`
}

// ToEntity converts the database model to a domain model
func (m *OperationRoutePostgreSQLModel) ToEntity() *mmodel.OperationRoute {
	if m == nil {
		return nil
	}

	codeValue := ""
	if m.Code.Valid {
		codeValue = m.Code.String
	}

	e := &mmodel.OperationRoute{
		ID:             m.ID,
		OrganizationID: m.OrganizationID,
		LedgerID:       m.LedgerID,
		Title:          m.Title,
		Description:    m.Description,
		Code:           codeValue,
		OperationType:  m.OperationType,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}

	if m.AccountRuleType != "" || m.AccountRuleValidIf != "" {
		account := &mmodel.AccountRule{
			RuleType: m.AccountRuleType,
		}
		m.processAccountRuleValidIfForEntity(account)
		e.Account = account
	}

	if m.DeletedAt.Valid {
		e.DeletedAt = &m.DeletedAt.Time
	}

	return e
}

// processAccountRuleValidIfForEntity processes the account rule ValidIf field when converting from model to entity
func (m *OperationRoutePostgreSQLModel) processAccountRuleValidIfForEntity(account *mmodel.AccountRule) {
	if m.AccountRuleValidIf == "" {
		return
	}

	if strings.ToLower(m.AccountRuleType) == constant.AccountRuleTypeAccountType {
		values := strings.Split(m.AccountRuleValidIf, ",")
		for i, v := range values {
			values[i] = strings.TrimSpace(v)
		}

		account.ValidIf = values

		return
	}

	account.ValidIf = m.AccountRuleValidIf
}

// setAccountRuleValidIf sets the AccountRuleValidIf field based on the account rule type
func (m *OperationRoutePostgreSQLModel) setAccountRuleValidIf(account *mmodel.AccountRule) {
	if account.ValidIf == nil {
		return
	}

	switch strings.ToLower(account.RuleType) {
	case constant.AccountRuleTypeAccountType:
		if values, ok := account.ValidIf.([]string); ok {
			m.AccountRuleValidIf = strings.Join(values, ",")
			return
		}

		if values, ok := account.ValidIf.([]any); ok {
			stringValues := make([]string, 0, len(values))
			for _, v := range values {
				if str, ok := v.(string); ok {
					stringValues = append(stringValues, str)
				}
			}

			m.AccountRuleValidIf = strings.Join(stringValues, ",")
		}
	default:
		if value, ok := account.ValidIf.(string); ok {
			m.AccountRuleValidIf = value
		}
	}
}

// FromEntity converts a domain model to the database model
func (m *OperationRoutePostgreSQLModel) FromEntity(e *mmodel.OperationRoute) {
	if e == nil {
		return
	}

	m.ID = e.ID
	m.OrganizationID = e.OrganizationID
	m.LedgerID = e.LedgerID
	m.Title = e.Title
	m.Description = e.Description

	if strings.TrimSpace(e.Code) == "" {
		m.Code = sql.NullString{}
	} else {
		m.Code = sql.NullString{String: e.Code, Valid: true}
	}

	m.OperationType = strings.ToLower(e.OperationType)

	if e.Account != nil {
		m.AccountRuleType = e.Account.RuleType
		m.setAccountRuleValidIf(e.Account)
	}

	m.CreatedAt = e.CreatedAt
	m.UpdatedAt = e.UpdatedAt

	if e.DeletedAt != nil {
		m.DeletedAt = sql.NullTime{
			Time:  *e.DeletedAt,
			Valid: true,
		}
	} else {
		m.DeletedAt = sql.NullTime{}
	}
}
