// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"maps"

	"github.com/google/uuid"
)

// AccountContext contains account information for validation.
// Type should be one of: "checking", "savings", "credit"
// Status should be one of: "active", "suspended", "closed"
type AccountContext struct {
	ID       uuid.UUID      `json:"accountId" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	Type     string         `json:"type" example:"checking"`
	Status   string         `json:"status" example:"active"`
	Metadata map[string]any `json:"metadata,omitempty"`
} //	@name	AccountContext

// Clone creates a copy of AccountContext.
// Returns nil if the receiver is nil.
// Metadata map entries are shallow-copied; nested mutable values will be shared.
func (a *AccountContext) Clone() *AccountContext {
	if a == nil {
		return nil
	}

	clone := *a
	if a.Metadata != nil {
		clone.Metadata = make(map[string]any, len(a.Metadata))
		maps.Copy(clone.Metadata, a.Metadata)
	}

	return &clone
}

// ToMap converts AccountContext to map[string]any for CEL evaluation.
// Returns nil if the receiver is nil.
func (a *AccountContext) ToMap() map[string]any {
	if a == nil {
		return nil
	}

	metadata := a.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	return map[string]any{
		"accountId": a.ID.String(),
		"type":      a.Type,
		"status":    a.Status,
		"metadata":  metadata,
	}
}

// MerchantContext contains merchant information for validation.
type MerchantContext struct {
	ID       uuid.UUID      `json:"merchantId" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	Name     string         `json:"name" example:"Acme Store"`
	Category string         `json:"category" example:"5411"` // e.g., ISO 18245 MCC code
	Country  string         `json:"country" example:"US"`    // ISO 3166-1 alpha-2 code
	Metadata map[string]any `json:"metadata,omitempty"`
} //	@name	MerchantContext

// Clone creates a copy of MerchantContext.
// Returns nil if the receiver is nil.
// Metadata map entries are shallow-copied; nested mutable values will be shared.
func (m *MerchantContext) Clone() *MerchantContext {
	if m == nil {
		return nil
	}

	clone := *m
	if m.Metadata != nil {
		clone.Metadata = make(map[string]any, len(m.Metadata))
		maps.Copy(clone.Metadata, m.Metadata)
	}

	return &clone
}

// ToMap converts MerchantContext to map[string]any for CEL evaluation.
// Returns nil if the receiver is nil.
func (m *MerchantContext) ToMap() map[string]any {
	if m == nil {
		return nil
	}

	metadata := m.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	return map[string]any{
		"merchantId": m.ID.String(),
		"name":       m.Name,
		"category":   m.Category,
		"country":    m.Country,
		"metadata":   metadata,
	}
}

// SegmentContext contains segment information for transaction categorization.
type SegmentContext struct {
	ID       uuid.UUID      `json:"segmentId" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	Name     string         `json:"name,omitempty" example:"retail"`
	Metadata map[string]any `json:"metadata,omitempty"`
} //	@name	SegmentContext

// Clone creates a copy of SegmentContext.
// Returns nil if the receiver is nil.
// Metadata map entries are shallow-copied; nested mutable values will be shared.
func (s *SegmentContext) Clone() *SegmentContext {
	if s == nil {
		return nil
	}

	clone := *s
	if s.Metadata != nil {
		clone.Metadata = make(map[string]any, len(s.Metadata))
		maps.Copy(clone.Metadata, s.Metadata)
	}

	return &clone
}

// ToMap converts SegmentContext to map[string]any for CEL evaluation.
// Returns nil if the receiver is nil.
func (s *SegmentContext) ToMap() map[string]any {
	if s == nil {
		return nil
	}

	metadata := s.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	return map[string]any{
		"segmentId": s.ID.String(),
		"name":      s.Name,
		"metadata":  metadata,
	}
}

// PortfolioContext contains portfolio information for transaction categorization.
type PortfolioContext struct {
	ID       uuid.UUID      `json:"portfolioId" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	Name     string         `json:"name,omitempty" example:"growth"`
	Metadata map[string]any `json:"metadata,omitempty"`
} //	@name	PortfolioContext

// Clone creates a copy of PortfolioContext.
// Returns nil if the receiver is nil.
// Metadata map entries are shallow-copied; nested mutable values will be shared.
func (p *PortfolioContext) Clone() *PortfolioContext {
	if p == nil {
		return nil
	}

	clone := *p
	if p.Metadata != nil {
		clone.Metadata = make(map[string]any, len(p.Metadata))
		maps.Copy(clone.Metadata, p.Metadata)
	}

	return &clone
}

// ToMap converts PortfolioContext to map[string]any for CEL evaluation.
// Returns nil if the receiver is nil.
func (p *PortfolioContext) ToMap() map[string]any {
	if p == nil {
		return nil
	}

	metadata := p.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	return map[string]any{
		"portfolioId": p.ID.String(),
		"name":        p.Name,
		"metadata":    metadata,
	}
}
