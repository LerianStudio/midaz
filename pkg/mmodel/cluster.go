package mmodel

import "time"

// CreateClusterInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateClusterInput
// @Description CreateClusterInput is the input payload to create a cluster.
type CreateClusterInput struct {
	Name     string         `json:"name" validate:"required,max=256" example:"My Cluster"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateClusterInput

// UpdateClusterInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateClusterInput
// @Description UpdateClusterInput is the input payload to update a cluster.
type UpdateClusterInput struct {
	Name     string         `json:"name" validate:"max=256" example:"My Cluster Updated"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name UpdateClusterInput

// Cluster is a struct designed to encapsulate payload data.
//
// swagger:model Cluster
// @Description Cluster is a struct designed to store cluster data.
type Cluster struct {
	ID             string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name           string         `json:"name" example:"My Cluster"`
	LedgerID       string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt      time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt      *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata       map[string]any `json:"metadata,omitempty"`
} // @name Cluster

// Clusters struct to return get all.
//
// swagger:model Clusters
// @Description Clusters is the struct designed to return a list of clusters with pagination.
type Clusters struct {
	Items []Cluster `json:"items"`
	Page  int       `json:"page" example:"1"`
	Limit int       `json:"limit" example:"10"`
} // @name Clusters
