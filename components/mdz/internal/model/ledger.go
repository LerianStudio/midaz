package model

import "time"

type LedgerInput struct {
	Name     string          `json:"name,omitempty"`
	Status   *LedgerStatus   `json:"status,omitempty"`
	Metadata *LedgerMetadata `json:"metadata,omitempty"`
}

type LedgerStatus struct {
	Code        *string `json:"code,omitempty"`
	Description *string `json:"description,omitempty"`
}

type LedgerMetadata struct {
	Chave   *string  `json:"chave,omitempty"`
	Bitcoin *string  `json:"bitcoin,omitempty"`
	Boolean *bool    `json:"boolean,omitempty"`
	Double  *float64 `json:"double,omitempty"`
	Int     *int     `json:"int,omitempty"`
}

type LedgerCreate struct {
	ID             string         `json:"id,omitempty"`
	Name           string         `json:"name,omitempty"`
	OrganizationID string         `json:"organizationId,omitempty"`
	Status         LedgerStatus   `json:"status,omitempty"`
	CreatedAt      time.Time      `json:"createdAt,omitempty"`
	UpdatedAt      time.Time      `json:"updatedAt,omitempty"`
	DeletedAt      time.Time      `json:"deletedAt,omitempty"`
	Metadata       LedgerMetadata `json:"metadata,omitempty"`
}

type LedgerList struct {
	Items []LedgerItems `json:"items"`
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
}

type LedgerItems struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	OrganizationID string          `json:"organizationId"`
	Status         *LedgerStatus   `json:"status"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	DeletedAt      *time.Time      `json:"deletedAt"`
	Metadata       *LedgerMetadata `json:"metadata,omitempty"`
}
