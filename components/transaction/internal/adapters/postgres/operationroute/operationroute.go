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
	ID                 uuid.UUID    `db:"id"`
	OrganizationID     uuid.UUID    `db:"organization_id"`
	LedgerID           uuid.UUID    `db:"ledger_id"`
	Title              string       `db:"title"`
	Description        string       `db:"description"`
	OperationType      string       `db:"operation_type"`
	AccountRuleType    string       `db:"account_rule_type"`
	AccountRuleValidIf string       `db:"account_rule_valid_if"`
	CreatedAt          time.Time    `db:"created_at"`
	UpdatedAt          time.Time    `db:"updated_at"`
	DeletedAt          sql.NullTime `db:"deleted_at"`
}

// ToEntity converts the database model to a domain model
func (m *OperationRoutePostgreSQLModel) ToEntity() *mmodel.OperationRoute {
	if m == nil {
		return nil
	}

	e := &mmodel.OperationRoute{
		ID:             m.ID,
		OrganizationID: m.OrganizationID,
		LedgerID:       m.LedgerID,
		Title:          m.Title,
		Description:    m.Description,
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
	m.OperationType = strings.ToLower(e.OperationType)

	if e.Account != nil {
		m.AccountRuleType = e.Account.RuleType

		if e.Account.ValidIf != nil {
			switch strings.ToLower(e.Account.RuleType) {
			case constant.AccountRuleTypeAccountType:
				if values, ok := e.Account.ValidIf.([]string); ok {
					m.AccountRuleValidIf = strings.Join(values, ",")
				} else if values, ok := e.Account.ValidIf.([]any); ok {
					stringValues := make([]string, len(values))

					for i, v := range values {
						if str, ok := v.(string); ok {
							stringValues[i] = str
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
