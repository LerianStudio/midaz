package mmodel

import "time"

// CreateSegmentInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateSegmentInput
// @Description CreateSegmentInput is the input payload to create a segment.
type CreateSegmentInput struct {
	Name     string         `json:"name" validate:"required,max=256" example:"My Segment"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateSegmentInput

// UpdateSegmentInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateSegmentInput
// @Description UpdateSegmentInput is the input payload to update a segment.
type UpdateSegmentInput struct {
	Name     string         `json:"name" validate:"max=256" example:"My Segment Updated"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name UpdateSegmentInput

// Segment is a struct designed to encapsulate payload data.
//
// swagger:model Segment
// @Description Segment is a struct designed to store segment data.
type Segment struct {
	ID             string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name           string         `json:"name" example:"My Segment"`
	LedgerID       string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt      time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt      *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata       map[string]any `json:"metadata,omitempty"`
} // @name Segment

// Segments struct to return get all.
//
// swagger:model Segments
// @Description Segments is the struct designed to return a list of segments with pagination.
type Segments struct {
	Items []Segment `json:"items"`
	Page  int       `json:"page" example:"1"`
	Limit int       `json:"limit" example:"10"`
} // @name Segments
