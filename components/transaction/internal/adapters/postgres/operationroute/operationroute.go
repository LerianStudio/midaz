// Package operationroute provides the repository implementation for operation route entity persistence.
//
// This package implements the Repository pattern for the OperationRoute entity, providing
// PostgreSQL-based data access. Operation routes define account selection rules for
// automated transaction routing (e.g., match accounts by alias or account type).
package operationroute

import (
	"database/sql"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// OperationRoutePostgreSQLModel represents the PostgreSQL database model for operation routes.
//
// This model stores account selection rules with:
//   - Operation type (source or destination)
//   - Account matching rules (by alias or account_type)
//   - Validation criteria (valid_if)
//   - Soft delete support
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

// ToEntity converts a PostgreSQL model to a domain OperationRoute entity.
//
// Transforms database representation to business logic representation,
// handling account rule composition and DeletedAt conversion.
//
// Returns:
//   - *mmodel.OperationRoute: Domain model with all fields populated
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

		if m.AccountRuleValidIf != "" {
			if strings.ToLower(m.AccountRuleType) == constant.AccountRuleTypeAccountType {
				values := strings.Split(m.AccountRuleValidIf, ",")
				for i, v := range values {
					values[i] = strings.TrimSpace(v)
				}

				account.ValidIf = values
			} else {
				account.ValidIf = m.AccountRuleValidIf
			}
		}

		e.Account = account
	}

	if m.DeletedAt.Valid {
		e.DeletedAt = &m.DeletedAt.Time
	}

	return e
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
	}

	if e.Account != nil && e.Account.ValidIf != nil {
		switch strings.ToLower(e.Account.RuleType) {
		case constant.AccountRuleTypeAccountType:
			if values, ok := e.Account.ValidIf.([]string); ok {
				m.AccountRuleValidIf = strings.Join(values, ",")
			} else if values, ok := e.Account.ValidIf.([]any); ok {
				stringValues := make([]string, 0, len(values))

				for _, v := range values {
					if str, ok := v.(string); ok {
						stringValues = append(stringValues, str)
					}
				}

				m.AccountRuleValidIf = strings.Join(stringValues, ",")
			}
		default:
			if value, ok := e.Account.ValidIf.(string); ok {
				m.AccountRuleValidIf = value
			}
		}
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
