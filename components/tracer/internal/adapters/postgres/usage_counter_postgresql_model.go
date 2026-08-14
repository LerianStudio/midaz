// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// UsageCounterPostgreSQLModel is the database representation of a UsageCounter entity.
// It follows the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
// This model handles:
// - UUID as string for database storage
// - All fields are non-nullable for UsageCounter
type UsageCounterPostgreSQLModel struct {
	ID            string          `db:"id"`
	LimitID       string          `db:"limit_id"`
	ScopeKey      string          `db:"scope_key"`
	PeriodKey     string          `db:"period_key"`
	CurrentUsage  decimal.Decimal `db:"current_usage"`
	LastUpdatedAt time.Time       `db:"last_updated_at"`
}

// ToEntity converts the database model to a domain entity.
// This method handles:
// - Parsing UUIDs from strings
// Returns an error for consistency with other model ToEntity methods,
// although UsageCounter has no JSON fields that could fail to unmarshal.
func (m *UsageCounterPostgreSQLModel) ToEntity() (*model.UsageCounter, error) {
	// Parse UUIDs from strings
	id, err := uuid.Parse(m.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid UsageCounter ID %q: %w", m.ID, err)
	}

	limitID, err := uuid.Parse(m.LimitID)
	if err != nil {
		return nil, fmt.Errorf("invalid LimitID %q: %w", m.LimitID, err)
	}

	return &model.UsageCounter{
		ID:            id,
		LimitID:       limitID,
		ScopeKey:      m.ScopeKey,
		PeriodKey:     m.PeriodKey,
		CurrentUsage:  m.CurrentUsage,
		LastUpdatedAt: m.LastUpdatedAt,
	}, nil
}

// FromEntity converts a domain entity to a database model.
// This method handles:
// - Converting UUIDs to strings
// Returns an error if the entity is nil.
func (m *UsageCounterPostgreSQLModel) FromEntity(entity *model.UsageCounter) error {
	if entity == nil {
		return fmt.Errorf("usage counter entity cannot be nil")
	}

	m.ID = entity.ID.String()
	m.LimitID = entity.LimitID.String()
	m.ScopeKey = entity.ScopeKey
	m.PeriodKey = entity.PeriodKey
	m.CurrentUsage = entity.CurrentUsage
	m.LastUpdatedAt = entity.LastUpdatedAt

	return nil
}
