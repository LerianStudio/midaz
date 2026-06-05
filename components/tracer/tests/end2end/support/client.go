// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// HTTPClient is a shared HTTP client with timeout for E2E tests.
var HTTPClient = &http.Client{
	Timeout: httpTimeout(),
}

func httpTimeout() time.Duration {
	if v := os.Getenv("E2E_HTTP_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}

	return 15 * time.Second
}

// GetBaseURL returns the base URL for the Tracer instance.
// Reads SERVER_ADDRESS via testutil (set by Makefile from E2E_SERVER).
func GetBaseURL() string {
	return testutil.GetBaseURL()
}

// GetAPIKey returns the API key for the Tracer instance.
// Reads API_KEY via testutil (set by Makefile from E2E_API_KEY).
func GetAPIKey() string {
	return testutil.GetAPIKey()
}

// authHeaders returns the standard authentication headers.
func authHeaders() map[string]string {
	return map[string]string{
		"X-API-Key":    GetAPIKey(),
		"Content-Type": "application/json",
	}
}

// doRequestE executes an HTTP request and returns the response, body, and any error.
// This is the error-returning core that all E2E helpers delegate to.
func doRequestE(method, url string, body io.Reader, headers map[string]string) (*http.Response, []byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("executing request: %w", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if err != nil {
		return resp, nil, fmt.Errorf("reading response body: %w", err)
	}

	return resp, respBody, nil
}

// --- Rule Operations ---

// RuleResponse represents the API response for a rule.
type RuleResponse struct {
	ID            string                `json:"ruleId"`
	Name          string                `json:"name"`
	Description   *string               `json:"description,omitempty"`
	Expression    string                `json:"expression"`
	Action        string                `json:"action"`
	Status        string                `json:"status"`
	Scopes        []testutil.ScopeInput `json:"scopes,omitempty"`
	CreatedAt     string                `json:"createdAt"`
	UpdatedAt     string                `json:"updatedAt"`
	ActivatedAt   *string               `json:"activatedAt,omitempty"`
	DeactivatedAt *string               `json:"deactivatedAt,omitempty"`
}

// CreateRuleE creates a rule and returns the full response body and any error.
func CreateRuleE(name, expression, action string, scopes []testutil.ScopeInput) (RuleResponse, int, error) {
	reqBody := testutil.RuleRequest{
		Name:       name,
		Expression: expression,
		Action:     action,
		Scopes:     scopes,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return RuleResponse{}, 0, fmt.Errorf("marshaling rule request: %w", err)
	}

	resp, respBody, err := doRequestE(http.MethodPost, GetBaseURL()+"/v1/rules", bytes.NewReader(body), authHeaders())
	if err != nil {
		return RuleResponse{}, 0, err
	}

	var rule RuleResponse
	if resp.StatusCode == http.StatusCreated {
		if err := json.Unmarshal(respBody, &rule); err != nil {
			return RuleResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling rule response: %w", err)
		}
	}

	return rule, resp.StatusCode, nil
}

// GetRuleE retrieves a rule by ID.
func GetRuleE(ruleID string) (RuleResponse, int, error) {
	resp, respBody, err := doRequestE(http.MethodGet, GetBaseURL()+"/v1/rules/"+ruleID, nil, authHeaders())
	if err != nil {
		return RuleResponse{}, 0, err
	}

	var rule RuleResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &rule); err != nil {
			return RuleResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling rule response: %w", err)
		}
	}

	return rule, resp.StatusCode, nil
}

// ActivateRuleE activates a rule and returns the response.
func ActivateRuleE(ruleID string) (RuleResponse, int, error) {
	resp, respBody, err := doRequestE(http.MethodPost, GetBaseURL()+"/v1/rules/"+ruleID+"/activate", nil, authHeaders())
	if err != nil {
		return RuleResponse{}, 0, err
	}

	var rule RuleResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &rule); err != nil {
			return RuleResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling rule response: %w", err)
		}
	}

	return rule, resp.StatusCode, nil
}

// DeactivateRuleE deactivates a rule and returns the response.
func DeactivateRuleE(ruleID string) (RuleResponse, int, error) {
	resp, respBody, err := doRequestE(http.MethodPost, GetBaseURL()+"/v1/rules/"+ruleID+"/deactivate", nil, authHeaders())
	if err != nil {
		return RuleResponse{}, 0, err
	}

	var rule RuleResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &rule); err != nil {
			return RuleResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling rule response: %w", err)
		}
	}

	return rule, resp.StatusCode, nil
}

// DeleteRuleE soft-deletes a rule.
func DeleteRuleE(ruleID string) (int, error) {
	resp, _, err := doRequestE(http.MethodDelete, GetBaseURL()+"/v1/rules/"+ruleID, nil, authHeaders())
	if err != nil {
		return 0, err
	}

	return resp.StatusCode, nil
}

// ListRulesResponse represents the list rules API response.
type ListRulesResponse struct {
	Rules      []RuleResponse `json:"rules"`
	NextCursor string         `json:"nextCursor,omitempty"`
	HasMore    bool           `json:"hasMore"`
}

// ListRulesE retrieves rules with optional query parameters.
func ListRulesE(queryParams string) (ListRulesResponse, int, error) {
	url := GetBaseURL() + "/v1/rules"
	if queryParams != "" {
		url += "?" + queryParams
	}

	resp, respBody, err := doRequestE(http.MethodGet, url, nil, authHeaders())
	if err != nil {
		return ListRulesResponse{}, 0, err
	}

	var listResp ListRulesResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &listResp); err != nil {
			return ListRulesResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling list rules: %w", err)
		}
	}

	return listResp, resp.StatusCode, nil
}

// FindRuleByNameE searches for a rule by name using the API's partial match filter.
// Returns the first matching rule or an error if not found.
func FindRuleByNameE(name string) (RuleResponse, error) {
	listResp, status, err := ListRulesE("name=" + url.QueryEscape(name) + "&limit=10")
	if err != nil {
		return RuleResponse{}, fmt.Errorf("listing rules: %w", err)
	}

	if status != http.StatusOK {
		return RuleResponse{}, fmt.Errorf("listing rules: expected 200, got %d", status)
	}

	// The API does partial case-insensitive match; find exact or closest match
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, rule := range listResp.Rules {
		if strings.ToLower(rule.Name) == lower {
			return rule, nil
		}
	}

	return RuleResponse{}, fmt.Errorf("rule %q not found via API", name)
}

// --- Validation Operations ---

// ValidationResponse represents the API response for a validation.
type ValidationResponse struct {
	ValidationID      string             `json:"validationId"`
	RequestID         string             `json:"requestId"`
	Decision          string             `json:"decision"`
	Reason            string             `json:"reason"`
	MatchedRuleIDs    []string           `json:"matchedRuleIds"`
	EvaluatedRuleIDs  []string           `json:"evaluatedRuleIds"`
	LimitUsageDetails []LimitUsageDetail `json:"limitUsageDetails"`
	ProcessingTimeMs  float64            `json:"processingTimeMs"`
}

// LimitUsageDetail represents limit usage in a validation response.
type LimitUsageDetail struct {
	LimitID         string          `json:"limitId"`
	LimitAmount     decimal.Decimal `json:"limitAmount"`
	CurrentUsage    decimal.Decimal `json:"currentUsage"`
	Exceeded        bool            `json:"exceeded"`
	Period          string          `json:"period"`
	Scope           string          `json:"scope"`
	AttemptedAmount decimal.Decimal `json:"attemptedAmount"`
}

// CreateValidationE submits a validation request and returns the response.
// Defaults: TransactionTimestamp → current UTC time, Account → test account UUID.
// Works on a local copy to avoid mutating the caller's request.
func CreateValidationE(req *testutil.ValidationRequest) (ValidationResponse, int, error) {
	if req == nil {
		return ValidationResponse{}, 0, fmt.Errorf("validation request cannot be nil")
	}

	reqCopy := *req

	if reqCopy.TransactionTimestamp == "" {
		reqCopy.TransactionTimestamp = time.Now().UTC().Format(time.RFC3339)
	}

	if reqCopy.Account == nil {
		reqCopy.Account = &testutil.AccountContext{ID: TestAccountUUID()}
	}

	body, err := json.Marshal(&reqCopy)
	if err != nil {
		return ValidationResponse{}, 0, fmt.Errorf("marshaling validation request: %w", err)
	}

	resp, respBody, err := doRequestE(http.MethodPost, GetBaseURL()+"/v1/validations", bytes.NewReader(body), authHeaders())
	if err != nil {
		return ValidationResponse{}, 0, err
	}

	var valResp ValidationResponse
	if resp.StatusCode == http.StatusCreated {
		if err := json.Unmarshal(respBody, &valResp); err != nil {
			return ValidationResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling validation response: %w", err)
		}
	}

	return valResp, resp.StatusCode, nil
}

// ValidationSummary represents a summary of a validation in list responses.
type ValidationSummary struct {
	ID               string          `json:"validationId"`
	Decision         string          `json:"decision"`
	Reason           string          `json:"reason"`
	Amount           decimal.Decimal `json:"amount"`
	Currency         string          `json:"currency"`
	TransactionType  string          `json:"transactionType"`
	AccountID        string          `json:"accountId"`
	SegmentID        string          `json:"segmentId,omitempty"`
	PortfolioID      string          `json:"portfolioId,omitempty"`
	MatchedRuleIDs   []string        `json:"matchedRuleIds"`
	ExceededLimitIDs []string        `json:"exceededLimitIds"`
	ProcessingTimeMs float64         `json:"processingTimeMs"`
	CreatedAt        string          `json:"createdAt"`
}

// ListValidationsResponse represents the list validations API response.
type ListValidationsResponse struct {
	TransactionValidations []ValidationSummary `json:"transactionValidations"`
	NextCursor             string              `json:"nextCursor,omitempty"`
	HasMore                bool                `json:"hasMore"`
}

// ListValidationsE retrieves validations with optional query parameters.
func ListValidationsE(queryParams string) (ListValidationsResponse, int, error) {
	url := GetBaseURL() + "/v1/validations"
	if queryParams != "" {
		url += "?" + queryParams
	}

	resp, respBody, err := doRequestE(http.MethodGet, url, nil, authHeaders())
	if err != nil {
		return ListValidationsResponse{}, 0, err
	}

	var listResp ListValidationsResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &listResp); err != nil {
			return ListValidationsResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling list response: %w", err)
		}
	}

	return listResp, resp.StatusCode, nil
}

// --- Limit Operations ---

// ListLimitsResponse represents the list limits API response.
type ListLimitsResponse struct {
	Limits     []LimitResponse `json:"limits"`
	NextCursor string          `json:"nextCursor,omitempty"`
	HasMore    bool            `json:"hasMore"`
}

// ListLimitsE retrieves limits with optional query parameters.
func ListLimitsE(queryParams string) (ListLimitsResponse, int, error) {
	url := GetBaseURL() + "/v1/limits"
	if queryParams != "" {
		url += "?" + queryParams
	}

	resp, respBody, err := doRequestE(http.MethodGet, url, nil, authHeaders())
	if err != nil {
		return ListLimitsResponse{}, 0, err
	}

	var listResp ListLimitsResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &listResp); err != nil {
			return ListLimitsResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling list limits: %w", err)
		}
	}

	return listResp, resp.StatusCode, nil
}

// FindLimitByNameE searches for a limit by name using the API's partial match filter.
// Returns the first matching limit or an error if not found.
func FindLimitByNameE(name string) (LimitResponse, error) {
	listResp, status, err := ListLimitsE("name=" + url.QueryEscape(name) + "&limit=10")
	if err != nil {
		return LimitResponse{}, fmt.Errorf("listing limits: %w", err)
	}

	if status != http.StatusOK {
		return LimitResponse{}, fmt.Errorf("listing limits: expected 200, got %d", status)
	}

	// The API does partial case-insensitive match; find exact or closest match
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, limit := range listResp.Limits {
		if strings.ToLower(strings.TrimSpace(limit.Name)) == lower {
			return limit, nil
		}
	}

	return LimitResponse{}, fmt.Errorf("limit %q not found via API", name)
}

// FindLimitsByScopeE searches for limits matching the given scope parameters.
// Scope parameters: account_id, segment_id, portfolio_id, merchant_id, transaction_type, sub_type.
// Returns all matching limits or an error.
func FindLimitsByScopeE(scopeParams map[string]string) ([]LimitResponse, error) {
	params := url.Values{}
	for key, val := range scopeParams {
		if val != "" {
			params.Set(key, val)
		}
	}

	if len(params) == 0 {
		return nil, fmt.Errorf("at least one scope parameter is required")
	}

	params.Set("limit", "100")

	listResp, status, err := ListLimitsE(params.Encode())
	if err != nil {
		return nil, fmt.Errorf("listing limits by scope: %w", err)
	}

	if status != http.StatusOK {
		return nil, fmt.Errorf("listing limits by scope: expected 200, got %d", status)
	}

	return listResp.Limits, nil
}

// LimitResponse represents the API response for a limit.
type LimitResponse struct {
	ID        string                `json:"limitId"`
	Name      string                `json:"name"`
	LimitType string                `json:"limitType"`
	MaxAmount decimal.Decimal       `json:"maxAmount"`
	Currency  string                `json:"currency"`
	Status    string                `json:"status"`
	Scopes    []testutil.ScopeInput `json:"scopes,omitempty"`
	CreatedAt string                `json:"createdAt"`
	UpdatedAt string                `json:"updatedAt"`
}

// LimitRequest represents the request body for creating a limit.
type LimitRequest struct {
	Name      string                `json:"name"`
	LimitType string                `json:"limitType"`
	MaxAmount decimal.Decimal       `json:"maxAmount"`
	Currency  string                `json:"currency"`
	Scopes    []testutil.ScopeInput `json:"scopes"`
}

// CreateLimitE creates a limit and returns the response.
func CreateLimitE(req *LimitRequest) (LimitResponse, int, error) {
	if req == nil {
		return LimitResponse{}, 0, fmt.Errorf("limit request cannot be nil")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return LimitResponse{}, 0, fmt.Errorf("marshaling limit request: %w", err)
	}

	resp, respBody, err := doRequestE(http.MethodPost, GetBaseURL()+"/v1/limits", bytes.NewReader(body), authHeaders())
	if err != nil {
		return LimitResponse{}, 0, err
	}

	var limit LimitResponse
	if resp.StatusCode == http.StatusCreated {
		if err := json.Unmarshal(respBody, &limit); err != nil {
			return LimitResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling limit response: %w", err)
		}
	}

	return limit, resp.StatusCode, nil
}

// GetLimitE retrieves a limit by ID.
func GetLimitE(limitID string) (LimitResponse, int, error) {
	resp, respBody, err := doRequestE(http.MethodGet, GetBaseURL()+"/v1/limits/"+limitID, nil, authHeaders())
	if err != nil {
		return LimitResponse{}, 0, err
	}

	var limit LimitResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &limit); err != nil {
			return LimitResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling limit response: %w", err)
		}
	}

	return limit, resp.StatusCode, nil
}

// ActivateLimitE activates a limit.
func ActivateLimitE(limitID string) (LimitResponse, int, error) {
	resp, respBody, err := doRequestE(http.MethodPost, GetBaseURL()+"/v1/limits/"+limitID+"/activate", nil, authHeaders())
	if err != nil {
		return LimitResponse{}, 0, err
	}

	var limit LimitResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &limit); err != nil {
			return LimitResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling limit response: %w", err)
		}
	}

	return limit, resp.StatusCode, nil
}

// DeactivateLimitE deactivates a limit.
func DeactivateLimitE(limitID string) (LimitResponse, int, error) {
	resp, respBody, err := doRequestE(http.MethodPost, GetBaseURL()+"/v1/limits/"+limitID+"/deactivate", nil, authHeaders())
	if err != nil {
		return LimitResponse{}, 0, err
	}

	var limit LimitResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &limit); err != nil {
			return LimitResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling limit response: %w", err)
		}
	}

	return limit, resp.StatusCode, nil
}

// UpdateLimitRequest represents the PATCH body for updating a limit.
type UpdateLimitRequest struct {
	MaxAmount *decimal.Decimal `json:"maxAmount,omitempty"`
	Name      *string          `json:"name,omitempty"`
}

// UpdateLimitE updates a limit via PATCH.
func UpdateLimitE(limitID string, req *UpdateLimitRequest) (LimitResponse, int, error) {
	if req == nil {
		return LimitResponse{}, 0, fmt.Errorf("update limit request cannot be nil")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return LimitResponse{}, 0, fmt.Errorf("marshaling update request: %w", err)
	}

	resp, respBody, err := doRequestE(http.MethodPatch, GetBaseURL()+"/v1/limits/"+limitID, bytes.NewReader(body), authHeaders())
	if err != nil {
		return LimitResponse{}, 0, err
	}

	var limit LimitResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &limit); err != nil {
			return LimitResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling limit response: %w", err)
		}
	}

	return limit, resp.StatusCode, nil
}

// UsageSnapshot represents the response from GET /v1/limits/{id}/usage.
type UsageSnapshot struct {
	LimitID            string          `json:"limitId"`
	CurrentUsage       decimal.Decimal `json:"currentUsage"`
	LimitAmount        decimal.Decimal `json:"limitAmount"`
	UtilizationPercent float64         `json:"utilizationPercent"`
	NearLimit          bool            `json:"nearLimit"`
	ResetAt            *string         `json:"resetAt,omitempty"`
}

// GetLimitUsageE retrieves usage for a limit.
func GetLimitUsageE(limitID string) (UsageSnapshot, int, error) {
	resp, respBody, err := doRequestE(http.MethodGet, GetBaseURL()+"/v1/limits/"+limitID+"/usage", nil, authHeaders())
	if err != nil {
		return UsageSnapshot{}, 0, err
	}

	var usage UsageSnapshot
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &usage); err != nil {
			return UsageSnapshot{}, resp.StatusCode, fmt.Errorf("unmarshaling usage response: %w", err)
		}
	}

	return usage, resp.StatusCode, nil
}

// --- Audit Event Operations ---

// AuditEvent represents an audit event from the API.
type AuditEvent struct {
	EventID      string         `json:"eventId"`
	EventType    string         `json:"eventType"`
	Action       string         `json:"action"`
	Result       string         `json:"result"`
	ResourceID   string         `json:"resourceId"`
	ResourceType string         `json:"resourceType"`
	Hash         string         `json:"hash,omitempty"`
	PreviousHash string         `json:"previousHash,omitempty"`
	CreatedAt    string         `json:"createdAt"`
	Context      map[string]any `json:"context,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// ListAuditEventsResponse represents the list audit events API response.
type ListAuditEventsResponse struct {
	AuditEvents []AuditEvent `json:"auditEvents"`
	NextCursor  string       `json:"nextCursor,omitempty"`
	HasMore     bool         `json:"hasMore"`
}

// ListAuditEventsE retrieves audit events with optional query parameters.
func ListAuditEventsE(queryParams string) (ListAuditEventsResponse, int, error) {
	url := GetBaseURL() + "/v1/audit-events"
	if queryParams != "" {
		url += "?" + queryParams
	}

	resp, respBody, err := doRequestE(http.MethodGet, url, nil, authHeaders())
	if err != nil {
		return ListAuditEventsResponse{}, 0, err
	}

	var listResp ListAuditEventsResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &listResp); err != nil {
			return ListAuditEventsResponse{}, resp.StatusCode, fmt.Errorf("unmarshaling audit events: %w", err)
		}
	}

	return listResp, resp.StatusCode, nil
}

// HashChainVerification represents the response from hash chain verification.
type HashChainVerification struct {
	IsValid        bool   `json:"isValid"`
	TotalChecked   int64  `json:"totalChecked"`
	Message        string `json:"message"`
	FirstInvalidID *int64 `json:"firstInvalidId,omitempty"`
}

// VerifyHashChainE verifies the audit hash chain up to the given event ID.
func VerifyHashChainE(eventID string) (HashChainVerification, int, error) {
	resp, respBody, err := doRequestE(http.MethodGet, GetBaseURL()+"/v1/audit-events/"+eventID+"/verify", nil, authHeaders())
	if err != nil {
		return HashChainVerification{}, 0, err
	}

	var result HashChainVerification
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBody, &result); err != nil {
			return HashChainVerification{}, resp.StatusCode, fmt.Errorf("unmarshaling verification: %w", err)
		}
	}

	return result, resp.StatusCode, nil
}

// --- Merchant UUID helpers ---

// uuidMu protects all deterministic UUID maps and counters from concurrent access.
var uuidMu sync.Mutex

// Initial UUID base values — single source of truth for both package-level
// vars and ResetDeterministicUUIDMaps.
const (
	initialMerchantBase int64 = 70100
	initialSegmentBase  int64 = 80100
	initialAccountBase  int64 = 90100
)

func defaultMerchantUUIDBaseMap() map[string]int64 {
	return map[string]int64{
		"supermart": 70001, "fuelco": 70002, "globalshop": 70003,
		"localstore": 70004, "trustedcorp": 70005, "unknownshop": 70006,
	}
}

func defaultSegmentUUIDBaseMap() map[string]int64 {
	return map[string]int64{
		"corporate": 80001, "retail": 80002, "premium": 80003,
	}
}

func defaultAccountUUIDBaseMap() map[string]int64 {
	return map[string]int64{
		"company abc": 90001, "company xyz": 90002,
	}
}

// merchantUUIDBaseMap maps normalized (lowercase) merchant names to deterministic UUID bases.
var merchantUUIDBaseMap = defaultMerchantUUIDBaseMap()

// nextMerchantBase is used for merchant names not in the predefined map.
var nextMerchantBase = initialMerchantBase

// DeterministicMerchantUUID returns a consistent UUID for a merchant name.
// The name is normalized (trimmed + lowercased) so lookups are case-insensitive.
func DeterministicMerchantUUID(name string) string {
	uuidMu.Lock()
	defer uuidMu.Unlock()

	key := strings.ToLower(strings.TrimSpace(name))

	if base, ok := merchantUUIDBaseMap[key]; ok {
		return testutil.MustDeterministicUUID(base).String()
	}

	// For unknown merchants, assign a new base
	nextMerchantBase++
	merchantUUIDBaseMap[key] = nextMerchantBase

	return testutil.MustDeterministicUUID(nextMerchantBase).String()
}

// --- Deterministic UUID helpers ---

var segmentUUIDBaseMap = defaultSegmentUUIDBaseMap()

var nextSegmentBase = initialSegmentBase

// DeterministicSegmentUUID returns a deterministic UUID for a segment name.
func DeterministicSegmentUUID(name string) string {
	uuidMu.Lock()
	defer uuidMu.Unlock()

	lower := strings.ToLower(strings.TrimSpace(name))
	if base, ok := segmentUUIDBaseMap[lower]; ok {
		return testutil.MustDeterministicUUID(base).String()
	}

	nextSegmentBase++
	segmentUUIDBaseMap[lower] = nextSegmentBase

	return testutil.MustDeterministicUUID(nextSegmentBase).String()
}

var accountUUIDBaseMap = defaultAccountUUIDBaseMap()

var nextAccountBase = initialAccountBase

func DeterministicAccountUUID(customer string) string {
	uuidMu.Lock()
	defer uuidMu.Unlock()

	lower := strings.ToLower(strings.TrimSpace(customer))
	if base, ok := accountUUIDBaseMap[lower]; ok {
		return testutil.MustDeterministicUUID(base).String()
	}

	nextAccountBase++
	accountUUIDBaseMap[lower] = nextAccountBase

	return testutil.MustDeterministicUUID(nextAccountBase).String()
}

// ResetDeterministicUUIDMaps resets all UUID maps and counters to initial state.
// Call in test setup/teardown for scenario isolation.
func ResetDeterministicUUIDMaps() {
	uuidMu.Lock()
	defer uuidMu.Unlock()

	merchantUUIDBaseMap = defaultMerchantUUIDBaseMap()
	nextMerchantBase = initialMerchantBase

	segmentUUIDBaseMap = defaultSegmentUUIDBaseMap()
	nextSegmentBase = initialSegmentBase

	accountUUIDBaseMap = defaultAccountUUIDBaseMap()
	nextAccountBase = initialAccountBase
}

// TestAccountUUID returns a fixed UUID for the generic "test account" used in J3.
func TestAccountUUID() string {
	return testutil.MustDeterministicUUID(99001).String()
}

// --- Cleanup helpers ---

// CleanupRuleE attempts to delete a rule (best-effort, for test cleanup).
func CleanupRuleE(ruleID string) error {
	// Deactivate first (ignore errors — rule may already be inactive/deleted)
	_, _, _ = DeactivateRuleE(ruleID)
	// Delete
	_, err := DeleteRuleE(ruleID)

	return err
}

// CleanupLimitE attempts to delete a limit (best-effort, for test cleanup).
func CleanupLimitE(limitID string) error {
	// Deactivate first
	_, _, _ = DeactivateLimitE(limitID)
	// Delete
	_, _, err := doRequestE(http.MethodDelete, GetBaseURL()+"/v1/limits/"+limitID, nil, authHeaders())

	return err
}
