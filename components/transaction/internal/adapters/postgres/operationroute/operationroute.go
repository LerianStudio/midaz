// Package operationroute provides PostgreSQL data models for operation routing rules.
//
// This package implements the infrastructure layer for operation route storage in PostgreSQL,
// following the hexagonal architecture pattern. Operation routes define rules for
// validating and routing individual debit/credit operations within transactions.
//
// Domain Concept:
//
// An OperationRoute in the ledger system:
//   - Defines validation rules for individual operations
//   - Specifies which accounts are valid for specific operation types
//   - Supports account type filtering (deposit, credit, etc.)
//   - Can be linked to transaction routes for complete routing logic
//
// Routing Purpose:
//
// Operation routes enable:
//   - Account type restrictions (e.g., only credit cards can be debited)
//   - Alias pattern validation (e.g., must match @customer/*)
//   - Business rule enforcement at operation level
//   - Segregation of debit vs credit routing rules
//
// Data Flow:
//
//	Domain Entity (mmodel.OperationRoute) -> OperationRoutePostgreSQLModel -> PostgreSQL
//	PostgreSQL -> OperationRoutePostgreSQLModel -> Domain Entity (mmodel.OperationRoute)
//
// Related Packages:
//   - transactionroute: Parent routing rules containing operation routes
//   - mmodel: Domain model definitions
//   - constant: Operation route type constants
package operationroute

import (
	"database/sql"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// OperationRoutePostgreSQLModel represents the operation route entity in PostgreSQL.
//
// This model maps directly to the 'operation_route' table with SQL-specific types.
// It stores routing rules that determine which accounts are valid for specific
// operation types (debit/credit).
//
// Table Schema:
//
//	CREATE TABLE operation_route (
//	    id UUID PRIMARY KEY,
//	    organization_id UUID NOT NULL,
//	    ledger_id UUID NOT NULL,
//	    title VARCHAR(255) NOT NULL,
//	    description TEXT,
//	    code VARCHAR(100),
//	    operation_type VARCHAR(20) NOT NULL,  -- 'debit' or 'credit'
//	    account_rule_type VARCHAR(50),        -- 'account_type', 'alias', etc.
//	    account_rule_valid_if TEXT,           -- Comma-separated values or pattern
//	    created_at TIMESTAMP WITH TIME ZONE,
//	    updated_at TIMESTAMP WITH TIME ZONE,
//	    deleted_at TIMESTAMP WITH TIME ZONE
//	);
//
// Account Rule Types:
//
// The AccountRuleType and AccountRuleValidIf fields work together:
//   - account_type: ValidIf contains comma-separated account types
//   - alias: ValidIf contains alias pattern to match
//   - Other types: ValidIf contains type-specific validation value
//
// Thread Safety:
//
// OperationRoutePostgreSQLModel is not thread-safe. Each goroutine should work
// with its own instance.
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

// ToEntity converts an OperationRoutePostgreSQLModel to the domain model.
//
// This method implements the outbound mapping in hexagonal architecture,
// transforming the persistence model back to the domain representation.
//
// Mapping Process:
//  1. Map all direct fields (ID, title, description, etc.)
//  2. Handle nullable Code field
//  3. Parse AccountRule based on RuleType:
//     - account_type: Split comma-separated values into []string
//     - Other types: Use raw string value
//  4. Handle nullable DeletedAt for soft delete support
//
// Account Rule Parsing:
//
// For account_type rules, the ValidIf value is stored as comma-separated
// in the database but returned as []string in the domain model for easier
// validation logic.
//
// Returns:
//   - *mmodel.OperationRoute: Domain model with all fields mapped
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

// FromEntity converts a domain model to OperationRoutePostgreSQLModel.
//
// This method implements the inbound mapping in hexagonal architecture,
// transforming the domain representation to the persistence model.
//
// Mapping Process:
//  1. Map all direct fields with type conversions
//  2. Handle nullable Code field (empty string -> sql.NullString{})
//  3. Normalize operation type to lowercase
//  4. Serialize AccountRule based on RuleType:
//     - account_type: Join []string with commas
//     - Other types: Store raw string value
//  5. Convert nullable DeletedAt to sql.NullTime
//
// Parameters:
//   - e: Domain OperationRoute model to convert
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
