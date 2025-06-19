package ports

import (
	"context"
)

// MidazClientPort defines the interface for Midaz API operations
type MidazClientPort interface {
	// Health and connectivity
	HealthCheck(ctx context.Context) error
	ValidateConnection(ctx context.Context) error

	// Authentication
	ValidateAuth(ctx context.Context) error

	// Organization operations
	CreateOrganization(ctx context.Context, org *OrganizationRequest) (*OrganizationResponse, error)
	ListOrganizations(ctx context.Context, limit int, cursor string) (*OrganizationListResponse, error)
	GetOrganization(ctx context.Context, orgID string) (*OrganizationResponse, error)

	// Ledger operations
	CreateLedger(ctx context.Context, orgID string, ledger *LedgerRequest) (*LedgerResponse, error)
	ListLedgers(ctx context.Context, orgID string) (*LedgerListResponse, error)
	GetLedger(ctx context.Context, orgID, ledgerID string) (*LedgerResponse, error)

	// Asset operations
	CreateAsset(ctx context.Context, orgID, ledgerID string, asset *AssetRequest) (*AssetResponse, error)
	ListAssets(ctx context.Context, orgID, ledgerID string) (*AssetListResponse, error)
	GetAsset(ctx context.Context, orgID, ledgerID, assetID string) (*AssetResponse, error)
}

// Request/Response types for organizations
type OrganizationRequest struct {
	LegalName       string         `json:"legal_name"`
	DoingBusinessAs string         `json:"doing_business_as"`
	LegalDocument   string         `json:"legal_document"`
	Address         AddressRequest `json:"address"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type OrganizationResponse struct {
	ID              string          `json:"id"`
	LegalName       string          `json:"legal_name"`
	DoingBusinessAs string          `json:"doing_business_as"`
	LegalDocument   string          `json:"legal_document"`
	Address         AddressResponse `json:"address"`
	Status          StatusResponse  `json:"status"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
}

type OrganizationListResponse struct {
	Items      []OrganizationResponse `json:"items"`
	Pagination PaginationResponse     `json:"pagination"`
}

// Request/Response types for ledgers
type LedgerRequest struct {
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type LedgerResponse struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Status    StatusResponse `json:"status"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type LedgerListResponse struct {
	Items      []LedgerResponse   `json:"items"`
	Pagination PaginationResponse `json:"pagination"`
}

// Request/Response types for assets
type AssetRequest struct {
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Code     string         `json:"code"`
	Scale    int            `json:"scale"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type AssetResponse struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Code      string         `json:"code"`
	Scale     int            `json:"scale"`
	Status    StatusResponse `json:"status"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type AssetListResponse struct {
	Items      []AssetResponse    `json:"items"`
	Pagination PaginationResponse `json:"pagination"`
}

// Common types
type AddressRequest struct {
	Line1   string `json:"line1"`
	Line2   string `json:"line2,omitempty"`
	City    string `json:"city"`
	State   string `json:"state"`
	Country string `json:"country"`
	ZipCode string `json:"zip_code"`
}

type AddressResponse struct {
	Line1   string `json:"line1"`
	Line2   string `json:"line2,omitempty"`
	City    string `json:"city"`
	State   string `json:"state"`
	Country string `json:"country"`
	ZipCode string `json:"zip_code"`
}

type StatusResponse struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type PaginationResponse struct {
	Limit      int    `json:"limit"`
	Cursor     string `json:"cursor,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}
