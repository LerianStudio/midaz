// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAuthTokenGetter is a mock of AuthTokenGetter for tests
type mockAuthTokenGetter struct {
	getTokenFunc func(ctx context.Context, clientID, clientSecret string) (string, error)
}

func (m *mockAuthTokenGetter) GetApplicationToken(ctx context.Context, clientID, clientSecret string) (string, error) {
	if m.getTokenFunc != nil {
		return m.getTokenFunc(ctx, clientID, clientSecret)
	}
	return "test-token", nil
}

func TestNewMidazService(t *testing.T) {
	authClient := &middleware.AuthClient{}
	midazOnboardingURL := "https://midaz.example.com/"
	midazTransactionURL := "https://midaz-transaction.example.com/"

	service := NewMidazService(authClient, midazOnboardingURL, midazTransactionURL, "test-client-id", "test-client-secret", "v1.0.0")

	assert.NotNil(t, service)
	// NewMidazService strips trailing slashes; endpoints use a leading slash.
	assert.Equal(t, "https://midaz.example.com", service.MidazOnboardingURL)
	assert.Equal(t, "https://midaz-transaction.example.com", service.MidazTransactionURL)
	assert.Equal(t, "test-client-id", service.ClientID)
	assert.Equal(t, "test-client-secret", service.ClientSecret)
	assert.Equal(t, "v1.0.0", service.Version)
}

func TestMidazService_GetAccountFromMidazByAlias(t *testing.T) {
	ctx := context.Background()
	creditAccount := "test-account"
	organizationID := "org-123"
	ledgerID := "ledger-456"

	tests := []struct {
		name            string
		setupServer     func() *httptest.Server
		authTokenGetter AuthTokenGetter
		wantErr         bool
		expectedErrCode string
		description     string
	}{
		{
			name: "Success - Account found (StatusOK)",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/organizations/org-123/ledgers/ledger-456/accounts/alias/test-account", r.URL.Path)
					assert.Contains(t, r.Header.Get("Authorization"), "Bearer")
					w.WriteHeader(http.StatusOK)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     false,
			description: "Should return nil when account is found",
		},
		{
			name: "Success - Account found (StatusCreated)",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     false,
			description: "Should return nil for any non-error status code",
		},
		{
			name: "Error - GetApplicationToken fails",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
					return "", errors.New("token error")
				},
			},
			wantErr:         true,
			expectedErrCode: constant.ErrAccessMidaz.Error(),
			description:     "Should return error when GetApplicationToken fails",
		},
		{
			name: "Error - HTTP request fails (connection error)",
			setupServer: func() *httptest.Server {
				// Create server and immediately close it to simulate connection error
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				server.Close()
				return server
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:         true,
			expectedErrCode: constant.ErrAccessMidaz.Error(),
			description:     "Should return error when HTTP request fails",
		},
		{
			name: "Error - StatusForbidden",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:         true,
			expectedErrCode: constant.ErrForbiddenAccessMidaz.Error(),
			description:     "Should return error for StatusForbidden",
		},
		{
			name: "Error - StatusUnauthorized",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:         true,
			expectedErrCode: constant.ErrForbiddenAccessMidaz.Error(),
			description:     "Should return error for StatusUnauthorized",
		},
		{
			name: "Error - StatusInternalServerError",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:         true,
			expectedErrCode: constant.ErrAccessMidaz.Error(),
			description:     "Should return error for StatusInternalServerError",
		},
		{
			name: "Error - StatusNotFound with empty body (routing error)",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:         true,
			expectedErrCode: constant.ErrMidazRouteNotFound.Error(),
			description:     "Should return ErrMidazRouteNotFound when 404 body is empty (routing misconfiguration)",
		},
		{
			name: "Error - StatusNotFound with Midaz error JSON (account not found)",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"code":"0014","title":"Account Not Found","message":"account not found"}`))
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:         true,
			expectedErrCode: constant.ErrFindAccountOnMidaz.Error(),
			description:     "Should return ErrFindAccountOnMidaz when 404 body is a valid Midaz error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer func() {
				if server != nil {
					server.Close()
				}
			}()

			midazURL := ""
			if server != nil {
				midazURL = server.URL
			} else {
				// For connection error test, use closed server URL
				midazURL = "http://localhost:0/" // Invalid port to force error
			}

			service := &MidazService{
				authTokenGetter:    tt.authTokenGetter,
				client:             &http.Client{Timeout: 30 * time.Second},
				MidazOnboardingURL: midazURL,
				ClientID:           "test-client-id",
				ClientSecret:       "test-client-secret",
				Version:            "v1.0.0",
			}

			err := service.GetAccountFromMidazByAlias(ctx, creditAccount, organizationID, ledgerID)

			if tt.wantErr {
				assert.Error(t, err, tt.description)
				if tt.expectedErrCode != "" {
					switch e := err.(type) {
					case pkg.ValidationError:
						assert.Equal(t, tt.expectedErrCode, e.Code, "error code should match")
					case pkg.ForbiddenError:
						assert.Equal(t, tt.expectedErrCode, e.Code, "error code should match")
					case pkg.UnprocessableOperationError:
						assert.Equal(t, tt.expectedErrCode, e.Code, "error code should match")
					case pkg.EntityNotFoundError:
						assert.Equal(t, tt.expectedErrCode, e.Code, "error code should match")
					default:
						assert.Contains(t, err.Error(), tt.expectedErrCode, "error should contain expected code")
					}
				}
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

// TestNewMidazService_HasConfiguredHTTPClient verifies that NewMidazService
// creates a MidazService with a dedicated HTTP client (not http.DefaultClient),
// with a timeout configured, and with connection pooling via a custom Transport.
func TestNewMidazService_HasConfiguredHTTPClient(t *testing.T) {
	authClient := &middleware.AuthClient{}
	service := NewMidazService(authClient, "https://midaz.example.com/", "https://midaz-transaction.example.com/", "cid", "csecret", "v1.0.0")

	require.NotNil(t, service, "MidazService must not be nil")

	// The service must expose a configured HTTP client field
	require.NotNil(t, service.client, "MidazService must have a non-nil HTTP client")

	// Client must NOT be the default client (which has Timeout == 0)
	assert.NotEqual(t, http.DefaultClient, service.client, "Must not use http.DefaultClient")

	// Client must have a non-zero timeout
	defaultTimeout := 30 * time.Second
	assert.Equal(t, defaultTimeout, service.client.Timeout, "HTTP client timeout must be 30s by default")

	// Client must have a custom transport with connection pooling
	transport, ok := service.client.Transport.(*http.Transport)
	require.True(t, ok, "HTTP client transport must be *http.Transport")
	assert.Greater(t, transport.MaxIdleConns, 0, "MaxIdleConns must be configured")
	assert.Greater(t, transport.MaxIdleConnsPerHost, 0, "MaxIdleConnsPerHost must be configured")
	assert.Greater(t, int64(transport.IdleConnTimeout), int64(0), "IdleConnTimeout must be configured")
}

// TestNewMidazServiceWithAuthGetter verifies that NewMidazServiceWithAuthGetter
// creates a MidazService using a custom AuthTokenGetter instead of the default
// authClientAdapter.  This constructor is used in multi-tenant mode where a
// TenantAwareAuthGetter resolves per-tenant M2M credentials.
func TestNewMidazServiceWithAuthGetter(t *testing.T) {
	t.Parallel()

	customGetter := &mockAuthTokenGetter{
		getTokenFunc: func(_ context.Context, clientID, _ string) (string, error) {
			return "custom-token-" + clientID, nil
		},
	}

	service := NewMidazServiceWithAuthGetter(
		customGetter,
		"https://midaz.example.com/",
		"https://midaz-transaction.example.com/",
		"m2m-cid", "m2m-csecret", "v2.0.0",
	)

	require.NotNil(t, service)
	assert.Equal(t, "https://midaz.example.com", service.MidazOnboardingURL)
	assert.Equal(t, "https://midaz-transaction.example.com", service.MidazTransactionURL)
	assert.Equal(t, "m2m-cid", service.ClientID)
	assert.Equal(t, "m2m-csecret", service.ClientSecret)
	assert.Equal(t, "v2.0.0", service.Version)

	// Verify custom getter is used
	token, err := service.authTokenGetter.GetApplicationToken(context.Background(), "test-cid", "test-csecret")
	require.NoError(t, err)
	assert.Equal(t, "custom-token-test-cid", token, "must use the custom AuthTokenGetter")
}

// TestNewMidazServiceWithAuthGetter_BackwardCompatible verifies that the
// original NewMidazService constructor still produces valid services by
// delegating to NewMidazServiceWithAuthGetter internally.
func TestNewMidazServiceWithAuthGetter_BackwardCompatible(t *testing.T) {
	t.Parallel()

	authClient := &middleware.AuthClient{}

	svc := NewMidazService(authClient, "https://midaz.example.com/", "https://midaz-transaction.example.com/", "cid", "csecret", "v1.0.0")

	require.NotNil(t, svc)
	assert.Equal(t, "https://midaz.example.com", svc.MidazOnboardingURL)
	assert.Equal(t, "cid", svc.ClientID)
	assert.Equal(t, "csecret", svc.ClientSecret)
}

// TestMidazService_RespectsContextCancellation verifies that the HTTP request
// respects context cancellation, which requires NewRequestWithContext usage.
func TestMidazService_RespectsContextCancellation(t *testing.T) {
	// Create a server that delays long enough to be cancelled
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until context is cancelled or 5s passes
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// Create a context that is already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	service := &MidazService{
		authTokenGetter: &mockAuthTokenGetter{
			getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
				return "test-token", nil
			},
		},
		MidazOnboardingURL: server.URL,
		ClientID:           "test-client-id",
		ClientSecret:       "test-client-secret",
		Version:            "v1.0.0",
		client:             &http.Client{Timeout: 30 * time.Second},
	}

	err := service.GetAccountFromMidazByAlias(ctx, "test-account", "org-123", "ledger-456")

	// With a cancelled context, the request must fail
	assert.Error(t, err, "Request with cancelled context must return error")
}

// TestMidazService_GetAccountDetailsByAlias verifies all scenarios for the
// GetAccountDetailsByAlias method that retrieves a full Account struct from Midaz.
func TestMidazService_GetAccountDetailsByAlias(t *testing.T) {
	t.Parallel()

	segmentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	portfolioID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	happyPathBody := pkg.Account{
		ID:          "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Alias:       "test-alias",
		SegmentID:   &segmentID,
		PortfolioID: &portfolioID,
		Status: &pkg.AccountStatus{
			Code:        "active",
			Description: "Account is active",
		},
		Type: "deposit",
	}

	happyPathJSON, err := json.Marshal(happyPathBody)
	require.NoError(t, err, "Failed to marshal happy path body")

	tests := []struct {
		name            string
		setupServer     func() *httptest.Server
		authTokenGetter AuthTokenGetter
		wantErr         bool
		errType         string
		wantAccount     *pkg.Account
		description     string
	}{
		{
			name: "Success - returns Account with all fields populated",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/organizations/org-123/ledgers/ledger-456/accounts/alias/test-alias", r.URL.Path)
					assert.Contains(t, r.Header.Get("Authorization"), "Bearer")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(happyPathJSON)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     false,
			wantAccount: &happyPathBody,
			description: "Should return Account with all fields populated when Midaz responds 200",
		},
		{
			name: "Error - StatusNotFound with empty body (routing error)",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrMidazRouteNotFound.Error(),
			description: "Should return ErrMidazRouteNotFound when 404 body is empty (routing misconfiguration)",
		},
		{
			name: "Error - StatusNotFound with Midaz error JSON (account not found)",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"code":"0014","title":"Account Not Found","message":"account not found"}`))
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrFindAccountOnMidaz.Error(),
			description: "Should return ErrFindAccountOnMidaz when 404 body is a valid Midaz error",
		},
		{
			name: "Error - StatusForbidden returns ForbiddenError",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrForbiddenAccessMidaz.Error(),
			description: "Should return ForbiddenError on 403",
		},
		{
			name: "Error - StatusUnauthorized returns ForbiddenError",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrForbiddenAccessMidaz.Error(),
			description: "Should return ForbiddenError on 401",
		},
		{
			name: "Error - StatusInternalServerError returns error",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrAccessMidaz.Error(),
			description: "Should return error mapped from ErrAccessMidaz on 500",
		},
		{
			name: "Error - auth token failure returns error",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "", errors.New("auth service unavailable")
				},
			},
			wantErr:     true,
			errType:     constant.ErrAccessMidaz.Error(),
			description: "Should return error when GetApplicationToken fails",
		},
		{
			name: "Error - malformed JSON response returns error",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("{invalid json"))
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrFindAccountOnMidaz.Error(),
			description: "Should return error when response body is invalid JSON despite 200 status",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := tt.setupServer()
			defer server.Close()

			service := &MidazService{
				authTokenGetter:    tt.authTokenGetter,
				client:             &http.Client{Timeout: 30 * time.Second},
				MidazOnboardingURL: server.URL,
				ClientID:           "test-client-id",
				ClientSecret:       "test-client-secret",
				Version:            "v1.0.0",
			}

			account, err := service.GetAccountDetailsByAlias(context.Background(), "org-123", "ledger-456", "test-alias")

			if tt.wantErr {
				require.Error(t, err, tt.description)
				assert.Nil(t, account, "account must be nil on error")

				if tt.errType != "" {
					switch e := err.(type) {
					case pkg.ValidationError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case pkg.ForbiddenError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case pkg.UnprocessableOperationError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case pkg.EntityNotFoundError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					default:
						assert.Fail(t, "unexpected error type: %T", err)
					}
				}
			} else {
				require.NoError(t, err, tt.description)
				require.NotNil(t, account, "account must not be nil on success")
				assert.Equal(t, tt.wantAccount.ID, account.ID)
				assert.Equal(t, tt.wantAccount.Alias, account.Alias)
				assert.Equal(t, tt.wantAccount.Type, account.Type)
				require.NotNil(t, account.SegmentID)
				assert.Equal(t, *tt.wantAccount.SegmentID, *account.SegmentID)
				require.NotNil(t, account.PortfolioID)
				assert.Equal(t, *tt.wantAccount.PortfolioID, *account.PortfolioID)
				require.NotNil(t, account.Status)
				assert.Equal(t, tt.wantAccount.Status.Code, account.Status.Code)
			}
		})
	}
}

// TestMidazService_CountTransactionsByRoute verifies all scenarios for the
// CountTransactionsByRoute method that counts transactions on the Midaz transaction service.
func TestMidazService_CountTransactionsByRoute(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	routeID := "33333333-3333-3333-3333-333333333333"
	startDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	params := CountParams{
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Route:          routeID,
		Status:         "APPROVED",
		StartDate:      startDate,
		EndDate:        endDate,
	}

	tests := []struct {
		name            string
		setupServer     func() *httptest.Server
		authTokenGetter AuthTokenGetter
		wantErr         bool
		errType         string
		wantCount       int64
		description     string
	}{
		{
			name: "Success - returns transaction count",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Contains(t, r.URL.Path, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/metrics/count")
					assert.Equal(t, http.MethodHead, r.Method)
					assert.Equal(t, routeID, r.URL.Query().Get("route"))
					assert.Equal(t, "APPROVED", r.URL.Query().Get("status"))
					assert.Equal(t, startDate.Format(time.RFC3339), r.URL.Query().Get("start_date"))
					assert.Equal(t, endDate.Format(time.RFC3339), r.URL.Query().Get("end_date"))
					assert.Contains(t, r.Header.Get("Authorization"), "Bearer")

					w.Header().Set("X-Total-Count", "42")
					w.WriteHeader(http.StatusNoContent)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     false,
			wantCount:   42,
			description: "Should return totalCount from X-Total-Count header when Midaz responds 204",
		},
		{
			name: "Error - auth token failure",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "", errors.New("auth service unavailable")
				},
			},
			wantErr:     true,
			errType:     constant.ErrMidazQueryFailed.Error(),
			description: "Should return error when GetApplicationToken fails",
		},
		{
			name: "Error - StatusForbidden",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrForbiddenAccessMidaz.Error(),
			description: "Should return ForbiddenError on 403",
		},
		{
			name: "Error - StatusUnauthorized",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrForbiddenAccessMidaz.Error(),
			description: "Should return ForbiddenError on 401",
		},
		{
			name: "Error - StatusInternalServerError",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrAccessMidaz.Error(),
			description: "Should return error on 500",
		},
		{
			name: "Error - connection failure",
			setupServer: func() *httptest.Server {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				server.Close()

				return server
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrMidazQueryFailed.Error(),
			description: "Should return error when HTTP connection fails",
		},
		{
			name: "Error - malformed JSON response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("{invalid json"))
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			wantErr:     true,
			errType:     constant.ErrMidazQueryFailed.Error(),
			description: "Should return error on invalid JSON response",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := tt.setupServer()
			defer server.Close()

			service := &MidazService{
				authTokenGetter:     tt.authTokenGetter,
				client:              &http.Client{Timeout: 30 * time.Second},
				MidazOnboardingURL:  "https://onboarding.example.com/",
				MidazTransactionURL: server.URL + "/v1",
				ClientID:            "test-client-id",
				ClientSecret:        "test-client-secret",
				Version:             "v1.0.0",
			}

			count, err := service.CountTransactionsByRoute(context.Background(), params)

			if tt.wantErr {
				require.Error(t, err, tt.description)

				if tt.errType != "" {
					switch e := err.(type) {
					case pkg.HTTPError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case pkg.ForbiddenError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case pkg.ValidationError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case *pkg.HTTPError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case pkg.UnprocessableOperationError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					default:
						assert.Fail(t, "unexpected error type: %T", err)
					}
				}
			} else {
				require.NoError(t, err, tt.description)
				assert.Equal(t, tt.wantCount, count)
			}
		})
	}
}

// TestMidazService_ListAccounts verifies all scenarios for the
// ListAccounts method that retrieves paginated accounts from Midaz onboarding service.
func TestMidazService_ListAccounts(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	segmentID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	portfolioID := uuid.MustParse("55555555-5555-5555-5555-555555555555")

	accountSegID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	accountPortID := uuid.MustParse("55555555-5555-5555-5555-555555555555")

	happyPathItems := []pkg.Account{
		{
			ID:          "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Alias:       "account-1",
			SegmentID:   &accountSegID,
			PortfolioID: &accountPortID,
			Status: &pkg.AccountStatus{
				Code:        "active",
				Description: "Active account",
			},
			Type: "deposit",
		},
		{
			ID:    "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			Alias: "account-2",
			Type:  "deposit",
		},
	}

	tests := []struct {
		name            string
		setupServer     func() *httptest.Server
		authTokenGetter AuthTokenGetter
		filters         AccountFilters
		page            int
		limit           int
		wantErr         bool
		errType         string
		wantPage        *AccountPage
		description     string
	}{
		{
			name: "Success - returns account page with segment filter",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Contains(t, r.URL.Path, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts")
					assert.Equal(t, segmentID.String(), r.URL.Query().Get("segment_id"))
					assert.Equal(t, "10", r.URL.Query().Get("limit"))
					assert.Equal(t, "1", r.URL.Query().Get("page"))
					assert.Contains(t, r.Header.Get("Authorization"), "Bearer")

					resp := map[string]any{
						"items": happyPathItems,
						"page":  1,
						"limit": 10,
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(resp)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			filters: AccountFilters{
				SegmentID: &segmentID,
			},
			page:    1,
			limit:   10,
			wantErr: false,
			wantPage: &AccountPage{
				Items: happyPathItems,
				Page:  1,
				Limit: 10,
			},
			description: "Should return account page when Midaz responds 200 with segment filter",
		},
		{
			name: "Success - returns account page with portfolio filter",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, portfolioID.String(), r.URL.Query().Get("portfolio_id"))
					assert.Empty(t, r.URL.Query().Get("segment_id"))

					resp := map[string]any{
						"items": happyPathItems,
						"page":  1,
						"limit": 10,
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(resp)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			filters: AccountFilters{
				PortfolioID: &portfolioID,
			},
			page:    1,
			limit:   10,
			wantErr: false,
			wantPage: &AccountPage{
				Items: happyPathItems,
				Page:  1,
				Limit: 10,
			},
			description: "Should return account page when Midaz responds 200 with portfolio filter",
		},
		{
			name: "Success - no filters",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Empty(t, r.URL.Query().Get("segment_id"))
					assert.Empty(t, r.URL.Query().Get("portfolio_id"))

					resp := map[string]any{
						"items": []pkg.Account{},
						"page":  1,
						"limit": 10,
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(resp)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			filters: AccountFilters{},
			page:    1,
			limit:   10,
			wantErr: false,
			wantPage: &AccountPage{
				Items: []pkg.Account{},
				Page:  1,
				Limit: 10,
			},
			description: "Should return empty page when no filters and no accounts",
		},
		{
			name: "Error - auth token failure",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "", errors.New("auth service unavailable")
				},
			},
			filters:     AccountFilters{},
			page:        1,
			limit:       10,
			wantErr:     true,
			errType:     constant.ErrMidazQueryFailed.Error(),
			description: "Should return error when GetApplicationToken fails",
		},
		{
			name: "Error - StatusForbidden",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			filters:     AccountFilters{},
			page:        1,
			limit:       10,
			wantErr:     true,
			errType:     constant.ErrForbiddenAccessMidaz.Error(),
			description: "Should return ForbiddenError on 403",
		},
		{
			name: "Error - StatusInternalServerError",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			filters:     AccountFilters{},
			page:        1,
			limit:       10,
			wantErr:     true,
			errType:     constant.ErrAccessMidaz.Error(),
			description: "Should return error on 500",
		},
		{
			name: "Error - malformed JSON response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("{invalid json"))
				}))
			},
			authTokenGetter: &mockAuthTokenGetter{
				getTokenFunc: func(_ context.Context, _, _ string) (string, error) {
					return "test-token", nil
				},
			},
			filters:     AccountFilters{},
			page:        1,
			limit:       10,
			wantErr:     true,
			errType:     constant.ErrMidazQueryFailed.Error(),
			description: "Should return error on invalid JSON response",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := tt.setupServer()
			defer server.Close()

			service := &MidazService{
				authTokenGetter:     tt.authTokenGetter,
				client:              &http.Client{Timeout: 30 * time.Second},
				MidazOnboardingURL:  server.URL + "/v1",
				MidazTransactionURL: "https://transaction.example.com/",
				ClientID:            "test-client-id",
				ClientSecret:        "test-client-secret",
				Version:             "v1.0.0",
			}

			page, err := service.ListAccounts(context.Background(), orgID, ledgerID, tt.filters, tt.page, tt.limit)

			if tt.wantErr {
				require.Error(t, err, tt.description)
				assert.Nil(t, page, "page must be nil on error")

				if tt.errType != "" {
					switch e := err.(type) {
					case pkg.HTTPError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case pkg.ForbiddenError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case pkg.ValidationError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case *pkg.HTTPError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					case pkg.UnprocessableOperationError:
						assert.Equal(t, tt.errType, e.Code, "error code should match")
					default:
						assert.Fail(t, "unexpected error type: %T", err)
					}
				}
			} else {
				require.NoError(t, err, tt.description)
				require.NotNil(t, page, "page must not be nil on success")
				assert.Equal(t, tt.wantPage.Page, page.Page)
				assert.Equal(t, tt.wantPage.Limit, page.Limit)
				assert.Equal(t, len(tt.wantPage.Items), len(page.Items))
			}
		})
	}
}

// TestMidazService_GetAccountFromMidazByAlias_InvalidURL tests the case where
// the URL construction might cause issues
func TestMidazService_GetAccountFromMidazByAlias_InvalidURL(t *testing.T) {
	ctx := context.Background()
	creditAccount := "test-account"
	organizationID := "org-123"
	ledgerID := "ledger-456"

	// Test with a URL that will cause connection issues
	authTokenGetter := &mockAuthTokenGetter{
		getTokenFunc: func(ctx context.Context, clientID, clientSecret string) (string, error) {
			return "test-token", nil
		},
	}

	service := &MidazService{
		authTokenGetter:    authTokenGetter,
		client:             &http.Client{Timeout: 30 * time.Second},
		MidazOnboardingURL: "http://invalid-url-that-does-not-exist:9999",
		ClientID:           "test-client-id",
		ClientSecret:       "test-client-secret",
		Version:            "v1.0.0",
	}

	// This will fail at the HTTP request stage
	err := service.GetAccountFromMidazByAlias(ctx, creditAccount, organizationID, ledgerID)
	assert.Error(t, err)

	// Should return ErrAccessMidaz for connection/auth failures
	if validationErr, ok := err.(pkg.ValidationError); ok {
		assert.Equal(t, constant.ErrAccessMidaz.Error(), validationErr.Code,
			"Error should be ErrAccessMidaz for unreachable URL")
	}
}
