// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operationroute

import (
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// OperationRoutePostgreSQLModel represents the database model for operation routes.
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

// ToEntity converts the database model to a domain model.
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
		e.Account = m.buildAccountRule()
	}

	if m.DeletedAt.Valid {
		e.DeletedAt = &m.DeletedAt.Time
	}

	return e
}

// setAccountRuleValidIf sets the AccountRuleValidIf field based on the rule type and validIf value.
func (m *OperationRoutePostgreSQLModel) setAccountRuleValidIf(ruleType string, validIf any) {
	switch strings.ToLower(ruleType) {
	case constant.AccountRuleTypeAccountType:
		switch v := validIf.(type) {
		case []string:
			m.AccountRuleValidIf = strings.Join(v, ",")
		case []any:
			stringValues := make([]string, 0, len(v))

			for _, item := range v {
				if str, ok := item.(string); ok {
					stringValues = append(stringValues, str)
				}
			}

			m.AccountRuleValidIf = strings.Join(stringValues, ",")
		}
	default:
		if value, ok := validIf.(string); ok {
			m.AccountRuleValidIf = value
		}
	}
}

// buildAccountRule constructs an AccountRule from the model's account rule fields.
func (m *OperationRoutePostgreSQLModel) buildAccountRule() *mmodel.AccountRule {
	account := &mmodel.AccountRule{
		RuleType: m.AccountRuleType,
	}

	if m.AccountRuleValidIf == "" {
		return account
	}

	if strings.EqualFold(m.AccountRuleType, constant.AccountRuleTypeAccountType) {
		values := strings.Split(m.AccountRuleValidIf, ",")
		for i, v := range values {
			values[i] = strings.TrimSpace(v)
		}

		account.ValidIf = values
	} else {
		account.ValidIf = m.AccountRuleValidIf
	}

	return account
}

// FromEntity converts a domain model to the database model.
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
		m.setAccountRuleValidIf(e.Account.RuleType, e.Account.ValidIf)
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
