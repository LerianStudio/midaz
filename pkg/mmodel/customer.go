package mmodel

import "time"

// Customer represents a customer in the system
type Customer struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	Name           string         `json:"name"`
	Email          string         `json:"email"`
	Type           string         `json:"type"`
	Status         string         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      *time.Time     `json:"deletedAt,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// Customers is a collection of Customer objects
type Customers struct {
	Items []Customer `json:"items"`
	Page  int        `json:"page"`
	Limit int        `json:"limit"`
}

// CreateCustomerInput represents input for creating a new customer
type CreateCustomerInput struct {
	Name     string         `json:"name"`
	Email    string         `json:"email"`
	Type     string         `json:"type"`
	Status   string         `json:"status,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// UpdateCustomerInput represents input for updating a customer
type UpdateCustomerInput struct {
	Name     string         `json:"name,omitempty"`
	Email    string         `json:"email,omitempty"`
	Type     string         `json:"type,omitempty"`
	Status   string         `json:"status,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}