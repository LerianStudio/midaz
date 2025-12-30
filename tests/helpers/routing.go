package helpers

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	operationTypeSource        = "source"
	operationTypeDestination   = "destination"
	routingHTTPStatusCreated   = 201
	routingHTTPStatusOK        = 200
	routingHTTPStatusNoContent = 204
)

// AccountRule represents an account selection rule for operation routes.
type AccountRule struct {
	RuleType string `json:"ruleType,omitempty"`
	ValidIf  any    `json:"validIf,omitempty"`
}

// OperationRouteResponse represents an operation route API response.
type OperationRouteResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	LedgerID       string         `json:"ledgerId"`
	Title          string         `json:"title"`
	Description    string         `json:"description,omitempty"`
	Code           string         `json:"code,omitempty"`
	OperationType  string         `json:"operationType"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Account        *AccountRule   `json:"account,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

// TransactionRouteResponse represents a transaction route API response.
type TransactionRouteResponse struct {
	ID              string                   `json:"id"`
	OrganizationID  string                   `json:"organizationId"`
	LedgerID        string                   `json:"ledgerId"`
	Title           string                   `json:"title"`
	Description     string                   `json:"description,omitempty"`
	Metadata        map[string]any           `json:"metadata,omitempty"`
	OperationRoutes []OperationRouteResponse `json:"operationRoutes,omitempty"`
	CreatedAt       string                   `json:"createdAt"`
	UpdatedAt       string                   `json:"updatedAt"`
}

// OperationRouteListResponse represents the operation route list API response.
type OperationRouteListResponse struct {
	Items []OperationRouteResponse `json:"items"`
}

// TransactionRouteListResponse represents the transaction route list API response.
type TransactionRouteListResponse struct {
	Items []TransactionRouteResponse `json:"items"`
}

// CreateOperationRoutePayload returns a valid operation route creation payload.
func CreateOperationRoutePayload(title, operationType string) map[string]any {
	return map[string]any{
		"title":         title,
		"operationType": operationType,
	}
}

// CreateSourceOperationRoutePayload returns a source operation route payload.
func CreateSourceOperationRoutePayload(title string) map[string]any {
	return CreateOperationRoutePayload(title, operationTypeSource)
}

// CreateDestinationOperationRoutePayload returns a destination operation route payload.
func CreateDestinationOperationRoutePayload(title string) map[string]any {
	return CreateOperationRoutePayload(title, operationTypeDestination)
}

// CreateOperationRoutePayloadWithAccount returns an operation route payload with account rule.
func CreateOperationRoutePayloadWithAccount(title, operationType, ruleType string, validIf any) map[string]any {
	return map[string]any{
		"title":         title,
		"operationType": operationType,
		"account": map[string]any{
			"ruleType": ruleType,
			"validIf":  validIf,
		},
	}
}

// SetupOperationRoute creates an operation route and returns its ID.
func SetupOperationRoute(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, title, operationType string) (string, error) {
	payload := CreateOperationRoutePayload(title, operationType)
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)

	code, body, err := trans.Request(ctx, "POST", path, headers, payload)
	if err != nil || code != routingHTTPStatusCreated {
		return "", fmt.Errorf("create operation route failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var route OperationRouteResponse
	if err := json.Unmarshal(body, &route); err != nil || route.ID == "" {
		return "", fmt.Errorf("parse operation route: %w body=%s", err, string(body))
	}

	return route.ID, nil
}

// GetOperationRoute retrieves an operation route by ID.
func GetOperationRoute(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, routeID string) (*OperationRouteResponse, error) {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes/%s", orgID, ledgerID, routeID)

	code, body, err := trans.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != routingHTTPStatusOK {
		return nil, fmt.Errorf("get operation route failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var route OperationRouteResponse
	if err := json.Unmarshal(body, &route); err != nil {
		return nil, fmt.Errorf("parse operation route: %w body=%s", err, string(body))
	}

	return &route, nil
}

// ListOperationRoutes retrieves all operation routes for a ledger.
func ListOperationRoutes(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID string) (*OperationRouteListResponse, error) {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes", orgID, ledgerID)

	code, body, err := trans.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != routingHTTPStatusOK {
		return nil, fmt.Errorf("list operation routes failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var list OperationRouteListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parse operation routes list: %w body=%s", err, string(body))
	}

	return &list, nil
}

// UpdateOperationRoute updates an operation route and returns the updated route.
func UpdateOperationRoute(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, routeID string, payload map[string]any) (*OperationRouteResponse, error) {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes/%s", orgID, ledgerID, routeID)

	code, body, err := trans.Request(ctx, "PATCH", path, headers, payload)
	if err != nil || code != routingHTTPStatusOK {
		return nil, fmt.Errorf("update operation route failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var route OperationRouteResponse
	if err := json.Unmarshal(body, &route); err != nil {
		return nil, fmt.Errorf("parse operation route: %w body=%s", err, string(body))
	}

	return &route, nil
}

// DeleteOperationRoute deletes an operation route by ID.
func DeleteOperationRoute(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, routeID string) error {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes/%s", orgID, ledgerID, routeID)
	code, body, err := trans.Request(ctx, "DELETE", path, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != routingHTTPStatusOK && code != routingHTTPStatusNoContent) {
		return fmt.Errorf("delete operation route failed: code=%d err=%w body=%s", code, err, string(body))
	}

	return nil
}

// CreateTransactionRoutePayload returns a valid transaction route creation payload.
func CreateTransactionRoutePayload(title string, operationRouteIDs []string) map[string]any {
	return map[string]any{
		"title":           title,
		"operationRoutes": operationRouteIDs,
	}
}

// SetupTransactionRoute creates a transaction route and returns its ID.
func SetupTransactionRoute(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, title string, operationRouteIDs []string) (string, error) {
	payload := CreateTransactionRoutePayload(title, operationRouteIDs)
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)

	code, body, err := trans.Request(ctx, "POST", path, headers, payload)
	if err != nil || code != routingHTTPStatusCreated {
		return "", fmt.Errorf("create transaction route failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var route TransactionRouteResponse
	if err := json.Unmarshal(body, &route); err != nil || route.ID == "" {
		return "", fmt.Errorf("parse transaction route: %w body=%s", err, string(body))
	}

	return route.ID, nil
}

// GetTransactionRoute retrieves a transaction route by ID.
func GetTransactionRoute(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, routeID string) (*TransactionRouteResponse, error) {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes/%s", orgID, ledgerID, routeID)

	code, body, err := trans.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != routingHTTPStatusOK {
		return nil, fmt.Errorf("get transaction route failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var route TransactionRouteResponse
	if err := json.Unmarshal(body, &route); err != nil {
		return nil, fmt.Errorf("parse transaction route: %w body=%s", err, string(body))
	}

	return &route, nil
}

// ListTransactionRoutes retrieves all transaction routes for a ledger.
func ListTransactionRoutes(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID string) (*TransactionRouteListResponse, error) {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes", orgID, ledgerID)

	code, body, err := trans.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != routingHTTPStatusOK {
		return nil, fmt.Errorf("list transaction routes failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var list TransactionRouteListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parse transaction routes list: %w body=%s", err, string(body))
	}

	return &list, nil
}

// UpdateTransactionRoute updates a transaction route and returns the updated route.
func UpdateTransactionRoute(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, routeID string, payload map[string]any) (*TransactionRouteResponse, error) {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes/%s", orgID, ledgerID, routeID)

	code, body, err := trans.Request(ctx, "PATCH", path, headers, payload)
	if err != nil || code != routingHTTPStatusOK {
		return nil, fmt.Errorf("update transaction route failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var route TransactionRouteResponse
	if err := json.Unmarshal(body, &route); err != nil {
		return nil, fmt.Errorf("parse transaction route: %w body=%s", err, string(body))
	}

	return &route, nil
}

// DeleteTransactionRoute deletes a transaction route by ID.
func DeleteTransactionRoute(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, routeID string) error {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transaction-routes/%s", orgID, ledgerID, routeID)
	code, body, err := trans.Request(ctx, "DELETE", path, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != routingHTTPStatusOK && code != routingHTTPStatusNoContent) {
		return fmt.Errorf("delete transaction route failed: code=%d err=%w body=%s", code, err, string(body))
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// ASSET RATE HELPERS
// ═══════════════════════════════════════════════════════════════════════════════

// AssetRateResponse represents an asset rate API response.
type AssetRateResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	LedgerID       string         `json:"ledgerId"`
	ExternalID     string         `json:"externalId,omitempty"`
	From           string         `json:"from"`
	To             string         `json:"to"`
	Rate           string         `json:"rate"`
	Scale          int            `json:"scale,omitempty"`
	Source         string         `json:"source,omitempty"`
	TTL            int            `json:"ttl,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

// AssetRateListResponse represents the asset rate list API response.
type AssetRateListResponse struct {
	Items []AssetRateResponse `json:"items"`
}

// CreateAssetRatePayload returns a valid asset rate creation payload.
func CreateAssetRatePayload(from, to, rate string) map[string]any {
	return map[string]any{
		"from": from,
		"to":   to,
		"rate": rate,
	}
}

// SetupAssetRate creates an asset rate and returns its external ID.
func SetupAssetRate(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, from, to, rate string) (string, error) {
	payload := CreateAssetRatePayload(from, to, rate)
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates", orgID, ledgerID)

	code, body, err := trans.Request(ctx, "PUT", path, headers, payload)
	if err != nil || code != routingHTTPStatusOK {
		return "", fmt.Errorf("create asset rate failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var assetRate AssetRateResponse
	if err := json.Unmarshal(body, &assetRate); err != nil || assetRate.ID == "" {
		return "", fmt.Errorf("parse asset rate: %w body=%s", err, string(body))
	}

	return assetRate.ExternalID, nil
}

// GetAssetRateByExternalID retrieves an asset rate by external ID.
func GetAssetRateByExternalID(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, externalID string) (*AssetRateResponse, error) {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/%s", orgID, ledgerID, externalID)

	code, body, err := trans.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != routingHTTPStatusOK {
		return nil, fmt.Errorf("get asset rate failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var assetRate AssetRateResponse
	if err := json.Unmarshal(body, &assetRate); err != nil {
		return nil, fmt.Errorf("parse asset rate: %w body=%s", err, string(body))
	}

	return &assetRate, nil
}

// ListAssetRatesByAssetCode retrieves all asset rates for a given source asset code.
func ListAssetRatesByAssetCode(ctx context.Context, trans *HTTPClient, headers map[string]string, orgID, ledgerID, assetCode string) (*AssetRateListResponse, error) {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/asset-rates/from/%s", orgID, ledgerID, assetCode)

	code, body, err := trans.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != routingHTTPStatusOK {
		return nil, fmt.Errorf("list asset rates failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var list AssetRateListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parse asset rates list: %w body=%s", err, string(body))
	}

	return &list, nil
}
