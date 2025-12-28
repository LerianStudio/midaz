package helpers

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
)

const (
	holderTypeNaturalPerson = "NATURAL_PERSON"
	holderTypeLegalPerson   = "LEGAL_PERSON"
	crmHTTPStatusCreated    = 201
	crmHTTPStatusOK         = 200
)

// HolderResponse represents a holder API response.
type HolderResponse struct {
	ID         string         `json:"id"`
	ExternalID *string        `json:"externalId,omitempty"`
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Document   string         `json:"document"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  string         `json:"createdAt,omitempty"`
	UpdatedAt  string         `json:"updatedAt,omitempty"`
}

// AliasResponse represents an alias API response.
type AliasResponse struct {
	ID        string         `json:"id"`
	HolderID  string         `json:"holderId"`
	LedgerID  string         `json:"ledgerId"`
	AccountID string         `json:"accountId"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"createdAt,omitempty"`
	UpdatedAt string         `json:"updatedAt,omitempty"`
}

// HolderListResponse represents the holder list API response.
type HolderListResponse struct {
	Items []HolderResponse `json:"items"`
}

// AliasListResponse represents the alias list API response.
type AliasListResponse struct {
	Items []AliasResponse `json:"items"`
}

// CreateHolderPayload returns a valid holder creation payload.
func CreateHolderPayload(name, document string, holderType string) map[string]any {
	return map[string]any{
		"type":     holderType,
		"name":     name,
		"document": document,
	}
}

// CreateNaturalPersonPayload returns a natural person holder payload.
func CreateNaturalPersonPayload(name, cpf string) map[string]any {
	return CreateHolderPayload(name, cpf, holderTypeNaturalPerson)
}

// CreateLegalPersonPayload returns a legal person holder payload.
func CreateLegalPersonPayload(name, cnpj string) map[string]any {
	return CreateHolderPayload(name, cnpj, holderTypeLegalPerson)
}

// SetupHolder creates a holder and returns its ID.
func SetupHolder(ctx context.Context, crm *HTTPClient, headers map[string]string, name, document, holderType string) (string, error) {
	payload := CreateHolderPayload(name, document, holderType)

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, payload)
	if err != nil || code != crmHTTPStatusCreated {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("create holder failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var holder HolderResponse
	if err := json.Unmarshal(body, &holder); err != nil || holder.ID == "" {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("parse holder: %w body=%s", err, string(body))
	}

	return holder.ID, nil
}

// GetHolder retrieves a holder by ID.
func GetHolder(ctx context.Context, crm *HTTPClient, headers map[string]string, holderID string) (*HolderResponse, error) {
	path := "/v1/holders/" + holderID
	code, body, err := crm.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != crmHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("get holder failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var holder HolderResponse
	if err := json.Unmarshal(body, &holder); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("parse holder: %w body=%s", err, string(body))
	}

	return &holder, nil
}

// ListHolders retrieves all holders.
func ListHolders(ctx context.Context, crm *HTTPClient, headers map[string]string) (*HolderListResponse, error) {
	code, body, err := crm.Request(ctx, "GET", "/v1/holders", headers, nil)
	if err != nil || code != crmHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("list holders failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var list HolderListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("parse holders list: %w body=%s", err, string(body))
	}

	return &list, nil
}

// UpdateHolder updates a holder and returns the updated holder.
func UpdateHolder(ctx context.Context, crm *HTTPClient, headers map[string]string, holderID string, payload map[string]any) (*HolderResponse, error) {
	path := "/v1/holders/" + holderID
	code, body, err := crm.Request(ctx, "PATCH", path, headers, payload)
	if err != nil || code != crmHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("update holder failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var holder HolderResponse
	if err := json.Unmarshal(body, &holder); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("parse holder: %w body=%s", err, string(body))
	}

	return &holder, nil
}

// DeleteHolder deletes a holder by ID.
func DeleteHolder(ctx context.Context, crm *HTTPClient, headers map[string]string, holderID string) error {
	path := "/v1/holders/" + holderID
	code, body, err := crm.Request(ctx, "DELETE", path, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != 200 && code != 204) {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("delete holder failed: code=%d err=%w body=%s", code, err, string(body))
	}

	return nil
}

// CreateAliasPayload returns a valid alias creation payload.
func CreateAliasPayload(ledgerID, accountID string) map[string]any {
	return map[string]any{
		"ledgerId":  ledgerID,
		"accountId": accountID,
	}
}

// SetupAlias creates an alias for a holder and returns its ID.
func SetupAlias(ctx context.Context, crm *HTTPClient, headers map[string]string, holderID, ledgerID, accountID string) (string, error) {
	payload := CreateAliasPayload(ledgerID, accountID)

	path := fmt.Sprintf("/v1/holders/%s/aliases", holderID)
	code, body, err := crm.Request(ctx, "POST", path, headers, payload)
	if err != nil || code != crmHTTPStatusCreated {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("create alias failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var alias AliasResponse
	if err := json.Unmarshal(body, &alias); err != nil || alias.ID == "" {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("parse alias: %w body=%s", err, string(body))
	}

	return alias.ID, nil
}

// GetAlias retrieves an alias by holder ID and alias ID.
func GetAlias(ctx context.Context, crm *HTTPClient, headers map[string]string, holderID, aliasID string) (*AliasResponse, error) {
	path := fmt.Sprintf("/v1/holders/%s/aliases/%s", holderID, aliasID)
	code, body, err := crm.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != crmHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("get alias failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var alias AliasResponse
	if err := json.Unmarshal(body, &alias); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("parse alias: %w body=%s", err, string(body))
	}

	return &alias, nil
}

// ListAliases retrieves all aliases for a holder.
func ListAliases(ctx context.Context, crm *HTTPClient, headers map[string]string, holderID string) (*AliasListResponse, error) {
	path := fmt.Sprintf("/v1/holders/%s/aliases", holderID)
	code, body, err := crm.Request(ctx, "GET", path, headers, nil)
	// Allow both 200 for list and potentially empty results
	if err != nil || code != crmHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("list aliases failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var list AliasListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("parse aliases list: %w body=%s", err, string(body))
	}

	return &list, nil
}

// ListAllAliases retrieves all aliases across all holders.
func ListAllAliases(ctx context.Context, crm *HTTPClient, headers map[string]string) (*AliasListResponse, error) {
	code, body, err := crm.Request(ctx, "GET", "/v1/aliases", headers, nil)
	if err != nil || code != crmHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("list all aliases failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var list AliasListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("parse aliases list: %w body=%s", err, string(body))
	}

	return &list, nil
}

// UpdateAlias updates an alias and returns the updated alias.
func UpdateAlias(ctx context.Context, crm *HTTPClient, headers map[string]string, holderID, aliasID string, payload map[string]any) (*AliasResponse, error) {
	path := fmt.Sprintf("/v1/holders/%s/aliases/%s", holderID, aliasID)
	code, body, err := crm.Request(ctx, "PATCH", path, headers, payload)
	if err != nil || code != crmHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("update alias failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var alias AliasResponse
	if err := json.Unmarshal(body, &alias); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("parse alias: %w body=%s", err, string(body))
	}

	return &alias, nil
}

// DeleteAlias deletes an alias by holder ID and alias ID.
func DeleteAlias(ctx context.Context, crm *HTTPClient, headers map[string]string, holderID, aliasID string) error {
	path := fmt.Sprintf("/v1/holders/%s/aliases/%s", holderID, aliasID)
	code, body, err := crm.Request(ctx, "DELETE", path, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != 200 && code != 204) {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("delete alias failed: code=%d err=%w body=%s", code, err, string(body))
	}

	return nil
}

// GenerateValidCPF returns a valid Brazilian CPF for testing.
// Uses a calculation-valid CPF format.
func GenerateValidCPF() string {
	// Base CPF digits (first 9 digits) with random variation
	base := fmt.Sprintf("%09d", crmRandIntN(999999999))

	// Calculate first verification digit
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(base[i]-'0') * (10 - i)
	}

	d1 := 11 - (sum % 11)
	if d1 >= 10 {
		d1 = 0
	}

	// Calculate second verification digit
	sum = 0
	for i := 0; i < 9; i++ {
		sum += int(base[i]-'0') * (11 - i)
	}

	sum += d1 * 2

	d2 := 11 - (sum % 11)
	if d2 >= 10 {
		d2 = 0
	}

	return fmt.Sprintf("%s%d%d", base, d1, d2)
}

// GenerateValidCNPJ returns a valid Brazilian CNPJ for testing.
// Uses a calculation-valid CNPJ format.
func GenerateValidCNPJ() string {
	// Base CNPJ digits (first 12 digits) with random variation
	base := fmt.Sprintf("%08d0001", crmRandIntN(99999999))

	// Weights for first digit
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		sum += int(base[i]-'0') * weights1[i]
	}

	d1 := 11 - (sum % 11)
	if d1 >= 10 {
		d1 = 0
	}

	// Weights for second digit
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum = 0
	for i := 0; i < 12; i++ {
		sum += int(base[i]-'0') * weights2[i]
	}

	sum += d1 * weights2[12]

	d2 := 11 - (sum % 11)
	if d2 >= 10 {
		d2 = 0
	}

	return fmt.Sprintf("%s%d%d", base, d1, d2)
}

// crmRandIntN generates a random int in [0, n) using crypto/rand.
func crmRandIntN(n int) int {
	if n <= 0 {
		return 0
	}

	max := big.NewInt(int64(n))
	val, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0
	}

	return int(val.Int64())
}
