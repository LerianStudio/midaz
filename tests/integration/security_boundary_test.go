package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// ══════════════════════════════════════════════════════════════════════════════════
// AUTHENTICATION BOUNDARY TESTS
// ══════════════════════════════════════════════════════════════════════════════════

// TestIntegration_Security_NoAuthHeader tests that API endpoints reject requests
// without an Authorization header.
func TestIntegration_Security_NoAuthHeader(t *testing.T) {
	t.Parallel()
	requireAuthEnabled(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	// Headers without Authorization
	noAuthHeaders := map[string]string{
		"Content-Type": "application/json",
		"X-Request-Id": h.RandHex(8),
	}

	testCases := []struct {
		name     string
		client   *h.HTTPClient
		method   string
		path     string
		body     any
		wantCode int
	}{
		{
			name:     "list_organizations_no_auth",
			client:   onboard,
			method:   http.MethodGet,
			path:     "/v1/organizations",
			body:     nil,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:   "create_organization_no_auth",
			client: onboard,
			method: http.MethodPost,
			path:   "/v1/organizations",
			body: map[string]any{
				"legalName":     "Test Org",
				"legalDocument": "123456789",
				"address":       map[string]any{"country": "US"},
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "get_organization_no_auth",
			client:   onboard,
			method:   http.MethodGet,
			path:     "/v1/organizations/00000000-0000-0000-0000-000000000001",
			body:     nil,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "list_ledgers_no_auth",
			client:   onboard,
			method:   http.MethodGet,
			path:     "/v1/organizations/00000000-0000-0000-0000-000000000001/ledgers",
			body:     nil,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "list_transactions_no_auth",
			client:   trans,
			method:   http.MethodGet,
			path:     "/v1/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions",
			body:     nil,
			wantCode: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, body, err := tc.client.Request(ctx, tc.method, tc.path, noAuthHeaders, tc.body)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if code != tc.wantCode {
				t.Errorf("expected status %d, got %d; body=%s", tc.wantCode, code, string(body))
			}
		})
	}
}

// TestIntegration_Security_InvalidToken tests that API endpoints reject requests
// with invalid Bearer tokens.
func TestIntegration_Security_InvalidToken(t *testing.T) {
	t.Parallel()
	requireAuthEnabled(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	// Headers with invalid token
	invalidTokenHeaders := map[string]string{
		"Content-Type":  "application/json",
		"X-Request-Id":  h.RandHex(8),
		"Authorization": "Bearer invalid_token_12345",
	}

	testCases := []struct {
		name     string
		method   string
		path     string
		body     any
		wantCode int
	}{
		{
			name:     "list_organizations_invalid_token",
			method:   http.MethodGet,
			path:     "/v1/organizations",
			body:     nil,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:   "create_organization_invalid_token",
			method: http.MethodPost,
			path:   "/v1/organizations",
			body: map[string]any{
				"legalName":     "Test Org",
				"legalDocument": "123456789",
				"address":       map[string]any{"country": "US"},
			},
			wantCode: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, body, err := onboard.Request(ctx, tc.method, tc.path, invalidTokenHeaders, tc.body)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if code != tc.wantCode {
				t.Errorf("expected status %d, got %d; body=%s", tc.wantCode, code, string(body))
			}
		})
	}
}

// TestIntegration_Security_MalformedToken tests that API endpoints reject requests
// with malformed Authorization headers.
func TestIntegration_Security_MalformedToken(t *testing.T) {
	t.Parallel()
	requireAuthEnabled(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	malformedTokenCases := []struct {
		name        string
		authHeader  string
		description string
	}{
		{
			name:        "missing_bearer_prefix",
			authHeader:  "token_without_bearer_prefix",
			description: "token without Bearer prefix",
		},
		{
			name:        "wrong_scheme",
			authHeader:  "Basic dXNlcjpwYXNz",
			description: "Basic auth instead of Bearer",
		},
		{
			name:        "empty_bearer_token",
			authHeader:  "Bearer ",
			description: "Bearer with empty token",
		},
		{
			name:        "bearer_lowercase",
			authHeader:  "bearer some_token",
			description: "lowercase bearer prefix",
		},
		{
			name:        "double_bearer",
			authHeader:  "Bearer Bearer token",
			description: "double Bearer prefix",
		},
		{
			name:        "special_chars_token",
			authHeader:  "Bearer <script>alert('xss')</script>",
			description: "token with HTML/script injection",
		},
	}

	for _, tc := range malformedTokenCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := map[string]string{
				"Content-Type":  "application/json",
				"X-Request-Id":  h.RandHex(8),
				"Authorization": tc.authHeader,
			}

			code, body, err := onboard.Request(ctx, http.MethodGet, "/v1/organizations", headers, nil)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			// Accept 400 (bad request) or 401 (unauthorized) for malformed tokens
			if code != http.StatusUnauthorized && code != http.StatusBadRequest {
				t.Errorf("%s: expected status 400 or 401, got %d; body=%s", tc.description, code, string(body))
			}
		})
	}
}

// ══════════════════════════════════════════════════════════════════════════════════
// AUTHORIZATION (IDOR) BOUNDARY TESTS
// ══════════════════════════════════════════════════════════════════════════════════

// TestIntegration_Security_CrossTenantOrganizationAccess tests that users cannot
// access organizations they don't own.
func TestIntegration_Security_CrossTenantOrganizationAccess(t *testing.T) {
	t.Parallel()
	requireAuthEnabled(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	// Create organization with tenant A auth
	tenantAHeaders := h.AuthHeaders("tenant-a-" + h.RandHex(8))

	orgID, err := h.SetupOrganization(ctx, onboard, tenantAHeaders, "TenantA-Org-"+h.RandString(6))
	if err != nil {
		t.Fatalf("failed to create organization for tenant A: %v", err)
	}

	// Create different auth headers simulating tenant B
	// In a real multi-tenant system, this would be a different authenticated user
	tenantBHeaders := map[string]string{
		"Content-Type":  "application/json",
		"X-Request-Id":  "tenant-b-" + h.RandHex(8),
		"Authorization": "Bearer different_tenant_token",
	}

	t.Run("get_other_tenant_organization", func(t *testing.T) {
		// Tenant B tries to access Tenant A's organization
		code, body, err := onboard.Request(ctx, http.MethodGet, "/v1/organizations/"+orgID, tenantBHeaders, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		// Should get 401 (unauthorized), 403 (forbidden) or 404 (not found)
		// depending on the API's security model
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("expected status 401, 403 or 404 when accessing other tenant's org, got %d; body=%s", code, string(body))
		}
	})

	t.Run("update_other_tenant_organization", func(t *testing.T) {
		updatePayload := map[string]any{
			"legalName": "Hacked Organization Name",
		}
		code, body, err := onboard.Request(ctx, http.MethodPatch, "/v1/organizations/"+orgID, tenantBHeaders, updatePayload)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("expected status 401, 403 or 404 when updating other tenant's org, got %d; body=%s", code, string(body))
		}
	})

	t.Run("delete_other_tenant_organization", func(t *testing.T) {
		code, body, err := onboard.Request(ctx, http.MethodDelete, "/v1/organizations/"+orgID, tenantBHeaders, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("expected status 401, 403 or 404 when deleting other tenant's org, got %d; body=%s", code, string(body))
		}
	})
}

// TestIntegration_Security_CrossTenantLedgerAccess tests that users cannot
// access ledgers in organizations they don't have access to.
func TestIntegration_Security_CrossTenantLedgerAccess(t *testing.T) {
	t.Parallel()
	requireAuthEnabled(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)

	// Create organization and ledger with tenant A auth
	tenantAHeaders := h.AuthHeaders("tenant-a-ledger-" + h.RandHex(8))

	orgID, err := h.SetupOrganization(ctx, onboard, tenantAHeaders, "TenantA-OrgLedger-"+h.RandString(6))
	if err != nil {
		t.Fatalf("failed to create organization: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, tenantAHeaders, orgID, "TenantA-Ledger-"+h.RandString(6))
	if err != nil {
		t.Fatalf("failed to create ledger: %v", err)
	}

	// Create different auth headers simulating tenant B
	tenantBHeaders := map[string]string{
		"Content-Type":  "application/json",
		"X-Request-Id":  "tenant-b-ledger-" + h.RandHex(8),
		"Authorization": "Bearer different_tenant_token_ledger",
	}

	t.Run("get_other_tenant_ledger", func(t *testing.T) {
		code, body, err := onboard.Request(ctx, http.MethodGet, fmt.Sprintf("/v1/organizations/%s/ledgers/%s", orgID, ledgerID), tenantBHeaders, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("expected status 401, 403 or 404 when accessing other tenant's ledger, got %d; body=%s", code, string(body))
		}
	})

	t.Run("list_other_tenant_ledgers", func(t *testing.T) {
		code, body, err := onboard.Request(ctx, http.MethodGet, fmt.Sprintf("/v1/organizations/%s/ledgers", orgID), tenantBHeaders, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("expected status 401, 403 or 404 when listing other tenant's ledgers, got %d; body=%s", code, string(body))
		}
	})

	t.Run("create_ledger_in_other_tenant_org", func(t *testing.T) {
		code, body, err := onboard.Request(ctx, http.MethodPost, fmt.Sprintf("/v1/organizations/%s/ledgers", orgID), tenantBHeaders, map[string]any{"name": "Malicious Ledger"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("expected status 401, 403 or 404 when creating ledger in other tenant's org, got %d; body=%s", code, string(body))
		}
	})
}

// TestIntegration_Security_CrossTenantAccountAccess tests that users cannot
// access accounts belonging to other organizations.
func TestIntegration_Security_CrossTenantAccountAccess(t *testing.T) {
	t.Parallel()
	requireAuthEnabled(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	// Create organization, ledger, asset, and account with tenant A auth
	tenantAHeaders := h.AuthHeaders("tenant-a-account-" + h.RandHex(8))

	orgID, err := h.SetupOrganization(ctx, onboard, tenantAHeaders, "TenantA-OrgAcct-"+h.RandString(6))
	if err != nil {
		t.Fatalf("failed to create organization: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, tenantAHeaders, orgID, "TenantA-LedgerAcct-"+h.RandString(6))
	if err != nil {
		t.Fatalf("failed to create ledger: %v", err)
	}

	// Create USD asset
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, tenantAHeaders); err != nil {
		t.Fatalf("failed to create USD asset: %v", err)
	}

	// Create account
	accountAlias := "tenant-a-acct-" + h.RandString(6)
	accountID, err := h.SetupAccount(ctx, onboard, tenantAHeaders, orgID, ledgerID, accountAlias, "USD")
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Create different auth headers simulating tenant B
	tenantBHeaders := map[string]string{
		"Content-Type":  "application/json",
		"X-Request-Id":  "tenant-b-account-" + h.RandHex(8),
		"Authorization": "Bearer different_tenant_token_account",
	}

	t.Run("get_other_tenant_account", func(t *testing.T) {
		code, body, err := onboard.Request(ctx, http.MethodGet, fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", orgID, ledgerID, accountID), tenantBHeaders, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("expected status 401, 403 or 404 when accessing other tenant's account, got %d; body=%s", code, string(body))
		}
	})

	t.Run("list_other_tenant_accounts", func(t *testing.T) {
		code, body, err := onboard.Request(ctx, http.MethodGet, fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgID, ledgerID), tenantBHeaders, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("expected status 401, 403 or 404 when listing other tenant's accounts, got %d; body=%s", code, string(body))
		}
	})

	t.Run("get_other_tenant_account_balances", func(t *testing.T) {
		code, body, err := trans.Request(ctx, http.MethodGet, fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID), tenantBHeaders, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("expected status 401, 403 or 404 when accessing other tenant's balances, got %d; body=%s", code, string(body))
		}
	})

	t.Run("create_transaction_in_other_tenant", func(t *testing.T) {
		txPayload := map[string]any{
			"send": map[string]any{
				"asset": "USD",
				"value": "100",
				"distribute": map[string]any{
					"to": []map[string]any{{
						"accountAlias": accountAlias,
						"amount":       map[string]any{"asset": "USD", "value": "100"},
					}},
				},
			},
		}
		code, body, err := trans.Request(ctx, http.MethodPost, fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgID, ledgerID), tenantBHeaders, txPayload)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("expected status 401, 403 or 404 when creating transaction in other tenant's ledger, got %d; body=%s", code, string(body))
		}
	})
}

// ══════════════════════════════════════════════════════════════════════════════════
// PATH INJECTION TESTS
// ══════════════════════════════════════════════════════════════════════════════════

// TestIntegration_Security_PathTraversalInOrgID tests that path traversal attempts
// in organization IDs are rejected.
func TestIntegration_Security_PathTraversalInOrgID(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	pathTraversalAttempts := []struct {
		name  string
		orgID string
	}{
		{
			name:  "dot_dot_slash",
			orgID: "../etc/passwd",
		},
		{
			name:  "url_encoded_traversal",
			orgID: "..%2F..%2Fetc%2Fpasswd",
		},
		{
			name:  "double_url_encoded",
			orgID: "..%252F..%252Fetc%252Fpasswd",
		},
		{
			name:  "backslash_traversal",
			orgID: "..\\..\\etc\\passwd",
		},
		{
			name:  "mixed_slashes",
			orgID: "../..\\etc/passwd",
		},
		{
			name:  "null_byte_injection",
			orgID: "valid-id\x00../etc/passwd",
		},
		{
			name:  "valid_uuid_with_traversal",
			orgID: "00000000-0000-0000-0000-000000000001/../other",
		},
	}

	for _, tc := range pathTraversalAttempts {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, body, err := onboard.Request(ctx, http.MethodGet, "/v1/organizations/"+tc.orgID, headers, nil)
			if err != nil {
				// If the URL couldn't be created due to invalid control characters,
				// Go's net/url is protecting us at the URL parsing level - this is expected behavior
				if strings.Contains(err.Error(), "invalid control character") {
					t.Logf("URL correctly rejected by Go's net/url (control character protection): %q", tc.orgID)
					return
				}
				t.Fatalf("request failed: %v", err)
			}
			// Should get 400 (bad request) or 404 (not found) - never 200 with sensitive data
			if code == http.StatusOK {
				t.Errorf("path traversal attempt succeeded unexpectedly: %s; body=%s", tc.orgID, string(body))
			}
			// Verify it's not returning sensitive file contents
			if body != nil && (containsSensitivePatterns(body)) {
				t.Errorf("response may contain sensitive data: %s", string(body))
			}
		})
	}
}

// TestIntegration_Security_SQLInjectionInIDs tests that SQL injection attempts
// in path parameters are rejected.
func TestIntegration_Security_SQLInjectionInIDs(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// First, create a valid org to test against
	orgID, err := h.SetupOrganization(ctx, onboard, headers, "SQLITest-Org-"+h.RandString(6))
	if err != nil {
		t.Fatalf("failed to create organization: %v", err)
	}

	sqlInjectionAttempts := []struct {
		name     string
		ledgerID string
	}{
		{
			name:     "basic_union",
			ledgerID: "1' UNION SELECT * FROM users--",
		},
		{
			name:     "or_true",
			ledgerID: "1' OR '1'='1",
		},
		{
			name:     "semicolon_drop",
			ledgerID: "1; DROP TABLE organizations;--",
		},
		{
			name:     "comment_bypass",
			ledgerID: "admin'--",
		},
		{
			name:     "hex_encoded",
			ledgerID: "0x31277F",
		},
		{
			name:     "quoted_injection",
			ledgerID: "'; DELETE FROM ledgers WHERE '1'='1",
		},
		{
			name:     "uuid_like_injection",
			ledgerID: "00000000-0000-0000-0000-000000000001' OR 1=1--",
		},
	}

	for _, tc := range sqlInjectionAttempts {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, body, err := onboard.Request(ctx, http.MethodGet, fmt.Sprintf("/v1/organizations/%s/ledgers/%s", orgID, tc.ledgerID), headers, nil)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			// Should get 400 (bad request) or 404 (not found) - never 200 with SQL error
			// Check that we're not getting database errors exposed
			if containsSQLErrorPatterns(body) {
				t.Errorf("SQL injection may have exposed database errors: %s", string(body))
			}
			// Valid responses are 400 or 404
			if code == http.StatusOK {
				// Check if the response is a valid empty list (safe) or something unexpected
				var list struct {
					Items []any `json:"items"`
				}
				if err := json.Unmarshal(body, &list); err != nil || len(list.Items) > 0 {
					t.Errorf("SQL injection attempt returned unexpected success: %s; body=%s", tc.ledgerID, string(body))
				}
			}
		})
	}
}

// TestIntegration_Security_InvalidUUIDFormat tests that API endpoints reject
// invalid UUID formats in path parameters.
func TestIntegration_Security_InvalidUUIDFormat(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	invalidUUIDs := []struct {
		name string
		id   string
	}{
		{
			name: "empty_string",
			id:   "",
		},
		{
			name: "too_short",
			id:   "12345",
		},
		{
			name: "too_long",
			id:   "00000000-0000-0000-0000-000000000001-extra",
		},
		{
			name: "wrong_format_no_dashes",
			id:   "00000000000000000000000000000001",
		},
		{
			name: "wrong_char_in_uuid",
			id:   "ZZZZZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZZZZZZZZZ",
		},
		{
			name: "special_chars",
			id:   "!@#$%^&*()_+-=[]{}|;':\",./<>?",
		},
		{
			name: "unicode_chars",
			id:   "\u0000\u0001\u0002",
		},
		{
			name: "newline_injection",
			id:   "00000000-0000-0000-0000-000000000001\nX-Injected-Header: value",
		},
		{
			name: "carriage_return_injection",
			id:   "00000000-0000-0000-0000-000000000001\r\nX-Injected: value",
		},
	}

	for _, tc := range invalidUUIDs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Skip empty string test as it changes the path structure
			if tc.id == "" {
				return
			}

			code, body, err := onboard.Request(ctx, http.MethodGet, "/v1/organizations/"+tc.id, headers, nil)
			if err != nil {
				// If the URL couldn't be created due to invalid control characters or URL escape sequences,
				// Go's net/url is protecting us at the URL parsing level - this is expected behavior
				if strings.Contains(err.Error(), "invalid control character") {
					t.Logf("URL correctly rejected by Go's net/url (control character protection): %q", tc.id)
					return
				}
				if strings.Contains(err.Error(), "invalid URL escape") {
					t.Logf("URL correctly rejected by Go's net/url (invalid URL escape): %q", tc.id)
					return
				}
				t.Fatalf("request failed: %v", err)
			}
			// Should get 400 (bad request) or 404 (not found)
			if code != http.StatusBadRequest && code != http.StatusNotFound && code != http.StatusUnauthorized {
				t.Errorf("expected status 400, 401, or 404 for invalid UUID %q, got %d; body=%s", tc.id, code, string(body))
			}
		})
	}
}

// TestIntegration_Security_HeaderInjection tests that header injection attempts
// through path parameters are rejected.
func TestIntegration_Security_HeaderInjection(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	headerInjectionAttempts := []struct {
		name string
		id   string
	}{
		{
			name: "crlf_header_injection",
			id:   "test%0d%0aX-Injected-Header:%20injected-value",
		},
		{
			name: "lf_header_injection",
			id:   "test%0aX-Injected-Header:%20injected-value",
		},
		{
			name: "cr_header_injection",
			id:   "test%0dX-Injected-Header:%20injected-value",
		},
	}

	for _, tc := range headerInjectionAttempts {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, _, err := onboard.Request(ctx, http.MethodGet, "/v1/organizations/"+tc.id, headers, nil)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			// Should get 400 (bad request) or 404 (not found) - never 200
			if code == http.StatusOK {
				t.Errorf("header injection attempt did not result in error: %s", tc.id)
			}
		})
	}
}

// ══════════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════════

// containsSensitivePatterns checks if response body contains patterns that might
// indicate sensitive file contents were exposed.
func containsSensitivePatterns(body []byte) bool {
	sensitivePatterns := []string{
		"root:", // /etc/passwd pattern
		"/bin/bash",
		"/bin/sh",
		"BEGIN RSA",
		"BEGIN PRIVATE KEY",
		"PRIVATE KEY",
		"password=",
		"secret=",
	}
	bodyStr := string(body)
	for _, pattern := range sensitivePatterns {
		if len(bodyStr) > 0 && contains(bodyStr, pattern) {
			return true
		}
	}
	return false
}

// containsSQLErrorPatterns checks if response body contains patterns that might
// indicate SQL errors were exposed.
func containsSQLErrorPatterns(body []byte) bool {
	sqlErrorPatterns := []string{
		"syntax error",
		"SQL syntax",
		"mysql",
		"postgresql",
		"sqlite",
		"ORA-",
		"SQLSTATE",
		"unterminated",
		"unexpected",
		"near \"",
		"at line",
	}
	bodyStr := string(body)
	for _, pattern := range sqlErrorPatterns {
		if len(bodyStr) > 0 && containsIgnoreCase(bodyStr, pattern) {
			return true
		}
	}
	return false
}

func requireAuthEnabled(t *testing.T) {
	enabled, err := pluginAuthEnabledFromEnvFiles()
	if err != nil {
		t.Fatalf("failed to read PLUGIN_AUTH_ENABLED from .env files: %v", err)
	}
	if enabled == nil {
		t.Skip("PLUGIN_AUTH_ENABLED not found in .env files; skipping cross-tenant auth tests")
	}
	if !*enabled {
		t.Skip("PLUGIN_AUTH_ENABLED=false in .env files; skipping cross-tenant auth tests")
	}
}

func pluginAuthEnabledFromEnvFiles() (*bool, error) {
	paths := []string{
		filepath.Join("components", "onboarding", ".env"),
		filepath.Join("components", "transaction", ".env"),
		filepath.Join("components", "crm", ".env"),
	}

	found := false
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		value, ok := readEnvKey(string(b), "PLUGIN_AUTH_ENABLED")
		if !ok {
			continue
		}
		found = true
		if strings.EqualFold(value, "true") {
			enabled := true
			return &enabled, nil
		}
	}

	if found {
		enabled := false
		return &enabled, nil
	}

	return nil, nil
}

func readEnvKey(contents, key string) (string, bool) {
	for _, line := range strings.Split(contents, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		if k != key {
			continue
		}
		return strings.TrimSpace(parts[1]), true
	}

	return "", false
}

// contains checks if s contains substr (case-sensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	sLower := toLower(s)
	substrLower := toLower(substr)
	return len(sLower) >= len(substrLower) && findSubstring(sLower, substrLower) >= 0
}

// findSubstring finds the index of substr in s, returns -1 if not found.
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// toLower converts a string to lowercase.
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}
