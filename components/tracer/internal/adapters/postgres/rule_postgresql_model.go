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

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// RulePostgreSQLModel is the database representation of a Rule entity.
// It follows the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
// This model handles:
// - UUID as string for database storage
// - sql.Null* types for nullable database columns
// - JSON serialization for scopes array
type RulePostgreSQLModel struct {
	ID            string         `db:"id"`
	Name          string         `db:"name"`
	Description   sql.NullString `db:"description"`
	Expression    string         `db:"expression"`
	Action        string         `db:"action"`
	Scopes        string         `db:"scopes"`
	Status        string         `db:"status"`
	ContextID     sql.NullString `db:"context_id"` // Smallest segmentId across scopes (deterministic)
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
	ActivatedAt   sql.NullTime   `db:"activated_at"`
	DeactivatedAt sql.NullTime   `db:"deactivated_at"`
	DeletedAt     sql.NullTime   `db:"deleted_at"`
}

// ToEntity converts the database model to a domain entity.
// This method handles:
// - Parsing UUID from string
// - Unmarshaling JSON scopes
// - Converting sql.Null* types to pointers
// - Converting string enums to typed constants
// Returns an error if JSON unmarshaling fails (e.g., corrupted data in database).
func (m *RulePostgreSQLModel) ToEntity() (*model.Rule, error) {
	// Parse UUID from string
	id, err := uuid.Parse(m.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid Rule ID %q: %w", m.ID, err)
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
	var activatedAt *time.Time
	if m.ActivatedAt.Valid {
		activatedAt = &m.ActivatedAt.Time
	}

	var deactivatedAt *time.Time
	if m.DeactivatedAt.Valid {
		deactivatedAt = &m.DeactivatedAt.Time
	}

	var deletedAt *time.Time
	if m.DeletedAt.Valid {
		deletedAt = &m.DeletedAt.Time
	}

	return &model.Rule{
		ID:            id,
		Name:          m.Name,
		Description:   description,
		Expression:    m.Expression,
		Action:        model.Decision(m.Action),
		Scopes:        scopes,
		Status:        model.RuleStatus(m.Status),
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
		ActivatedAt:   activatedAt,
		DeactivatedAt: deactivatedAt,
		DeletedAt:     deletedAt,
	}, nil
}

// FromEntity converts a domain entity to a database model.
// This method handles:
// - Converting UUID to string
// - Marshaling scopes to JSON
// - Converting pointers to sql.Null* types
// - Converting typed constants to strings
// - Deriving context_id from first scope's segmentId
// Returns an error if JSON marshaling fails.
func (m *RulePostgreSQLModel) FromEntity(entity *model.Rule) error {
	if entity == nil {
		return fmt.Errorf("rule entity cannot be nil")
	}

	m.ID = entity.ID.String()
	m.Name = entity.Name
	m.Expression = entity.Expression
	m.Action = string(entity.Action)
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

	// Derive context_id from the smallest segmentId across all scopes (deterministic)
	m.ContextID = sql.NullString{Valid: false}

	var smallest string

	for i := range scopes {
		if scopes[i].SegmentID != nil {
			s := scopes[i].SegmentID.String()
			if !m.ContextID.Valid || s < smallest {
				smallest = s
				m.ContextID = sql.NullString{String: s, Valid: true}
			}
		}
	}

	// Convert pointer timestamps to sql.NullTime
	if entity.ActivatedAt != nil {
		m.ActivatedAt = sql.NullTime{Time: *entity.ActivatedAt, Valid: true}
	} else {
		m.ActivatedAt = sql.NullTime{Valid: false}
	}

	if entity.DeactivatedAt != nil {
		m.DeactivatedAt = sql.NullTime{Time: *entity.DeactivatedAt, Valid: true}
	} else {
		m.DeactivatedAt = sql.NullTime{Valid: false}
	}

	if entity.DeletedAt != nil {
		m.DeletedAt = sql.NullTime{Time: *entity.DeletedAt, Valid: true}
	} else {
		m.DeletedAt = sql.NullTime{Valid: false}
	}

	return nil
}
