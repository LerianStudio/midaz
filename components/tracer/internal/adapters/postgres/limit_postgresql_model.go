// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// LimitPostgreSQLModel is the database representation of a Limit entity.
// It follows the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
// This model handles:
// - UUID as string for database storage
// - sql.Null* types for nullable database columns
// - JSON serialization for scopes array
type LimitPostgreSQLModel struct {
	ID              string          `db:"id"`
	Name            string          `db:"name"`
	Description     sql.NullString  `db:"description"`
	LimitType       string          `db:"limit_type"`
	MaxAmount       decimal.Decimal `db:"max_amount"`
	Currency        string          `db:"currency"`
	Scopes          string          `db:"scopes"`
	Status          string          `db:"status"`
	ResetAt         sql.NullTime    `db:"reset_at"`
	ActiveTimeStart sql.NullString  `db:"active_time_start"`
	ActiveTimeEnd   sql.NullString  `db:"active_time_end"`
	CustomStartDate sql.NullTime    `db:"custom_start_date"`
	CustomEndDate   sql.NullTime    `db:"custom_end_date"`
	CreatedAt       time.Time       `db:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at"`
	DeletedAt       sql.NullTime    `db:"deleted_at"`
}

// ToEntity converts the database model to a domain entity.
// This method handles:
// - Parsing UUID from string
// - Unmarshaling JSON scopes
// - Converting sql.Null* types to pointers
// - Converting string enums to typed constants
// Returns an error if JSON unmarshaling fails (e.g., corrupted data in database).
func (m *LimitPostgreSQLModel) ToEntity() (*model.Limit, error) {
	// Parse UUID from string
	id, err := uuid.Parse(m.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid Limit ID %q: %w", m.ID, err)
	}

	// Convert nullable description to pointer
	var description *string
	if m.Description.Valid {
		description = &m.Description.String
	}

	// Unmarshal scopes from JSON
	var scopes []model.Scope
	if m.Scopes != "" {
		if err := json.Unmarshal([]byte(m.Scopes), &scopes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal scopes: %w", err)
		}
	}

	// Ensure scopes is never nil (return empty slice instead of null in JSON)
	if scopes == nil {
		scopes = []model.Scope{}
	}

	// Convert nullable timestamps to pointers
	var resetAt *time.Time
	if m.ResetAt.Valid {
		resetAt = &m.ResetAt.Time
	}

	var deletedAt *time.Time
	if m.DeletedAt.Valid {
		deletedAt = &m.DeletedAt.Time
	}

	limitType := model.LimitType(m.LimitType)
	if !limitType.IsValid() {
		return nil, fmt.Errorf("invalid limit type in database: %s", m.LimitType)
	}

	status := model.LimitStatus(m.Status)
	if !status.IsValid() {
		return nil, fmt.Errorf("invalid limit status in database: %s", m.Status)
	}

	// Convert time windows from database strings to TimeOfDay
	var activeTimeStart, activeTimeEnd *model.TimeOfDay

	if m.ActiveTimeStart.Valid {
		parsed, err := model.NewTimeOfDay(m.ActiveTimeStart.String)
		if err != nil {
			return nil, fmt.Errorf("invalid active_time_start in database: %w", err)
		}

		activeTimeStart = &parsed
	}

	if m.ActiveTimeEnd.Valid {
		parsed, err := model.NewTimeOfDay(m.ActiveTimeEnd.String)
		if err != nil {
			return nil, fmt.Errorf("invalid active_time_end in database: %w", err)
		}

		activeTimeEnd = &parsed
	}

	// Convert custom period dates
	var customStartDate, customEndDate *time.Time
	if m.CustomStartDate.Valid {
		customStartDate = &m.CustomStartDate.Time
	}

	if m.CustomEndDate.Valid {
		customEndDate = &m.CustomEndDate.Time
	}

	return &model.Limit{
		ID:              id,
		Name:            m.Name,
		Description:     description,
		LimitType:       limitType,
		MaxAmount:       m.MaxAmount,
		Currency:        m.Currency,
		Scopes:          scopes,
		Status:          status,
		ResetAt:         resetAt,
		ActiveTimeStart: activeTimeStart,
		ActiveTimeEnd:   activeTimeEnd,
		CustomStartDate: customStartDate,
		CustomEndDate:   customEndDate,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
		DeletedAt:       deletedAt,
	}, nil
}

// FromEntity converts a domain entity to a database model.
// This method handles:
// - Converting UUID to string
// - Marshaling scopes to JSON
// - Converting pointers to sql.Null* types
// - Converting typed constants to strings
// Returns an error if JSON marshaling fails.
func (m *LimitPostgreSQLModel) FromEntity(entity *model.Limit) error {
	if entity == nil {
		return fmt.Errorf("limit entity cannot be nil")
	}

	m.ID = entity.ID.String()
	m.Name = entity.Name
	m.LimitType = string(entity.LimitType)
	m.MaxAmount = entity.MaxAmount
	m.Currency = entity.Currency
	m.Status = string(entity.Status)
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt

	// Convert pointer description to sql.NullString
	if entity.Description != nil {
		m.Description = sql.NullString{String: *entity.Description, Valid: true}
	} else {
		m.Description = sql.NullString{Valid: false}
	}

	// Marshal scopes to JSON, defaulting to empty array for nil
	scopes := entity.Scopes
	if scopes == nil {
		scopes = []model.Scope{}
	}

	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return fmt.Errorf("failed to marshal scopes: %w", err)
	}

	m.Scopes = string(scopesJSON)

	// Convert pointer timestamps to sql.NullTime
	if entity.ResetAt != nil {
		m.ResetAt = sql.NullTime{Time: *entity.ResetAt, Valid: true}
	} else {
		m.ResetAt = sql.NullTime{Valid: false}
	}

	if entity.DeletedAt != nil {
		m.DeletedAt = sql.NullTime{Time: *entity.DeletedAt, Valid: true}
	} else {
		m.DeletedAt = sql.NullTime{Valid: false}
	}

	// Convert time windows to strings
	if entity.ActiveTimeStart != nil {
		m.ActiveTimeStart = sql.NullString{String: entity.ActiveTimeStart.String(), Valid: true}
	} else {
		m.ActiveTimeStart = sql.NullString{Valid: false}
	}

	if entity.ActiveTimeEnd != nil {
		m.ActiveTimeEnd = sql.NullString{String: entity.ActiveTimeEnd.String(), Valid: true}
	} else {
		m.ActiveTimeEnd = sql.NullString{Valid: false}
	}

	// Convert custom period dates
	if entity.CustomStartDate != nil {
		m.CustomStartDate = sql.NullTime{Time: *entity.CustomStartDate, Valid: true}
	} else {
		m.CustomStartDate = sql.NullTime{Valid: false}
	}

	if entity.CustomEndDate != nil {
		m.CustomEndDate = sql.NullTime{Time: *entity.CustomEndDate, Valid: true}
	} else {
		m.CustomEndDate = sql.NullTime{Valid: false}
	}

	return nil
}
