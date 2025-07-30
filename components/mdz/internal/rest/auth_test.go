package rest

import (
	"net/http"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestAuthenticateWithCredentials(t *testing.T) {
	tests := []struct {
		name           string
		username       string
		password       string
		mockResponse   string
		mockStatusCode int
		expectError    bool
		expectedToken  *model.TokenResponse
	}{
		{
			name:     "success",
			username: "testuser",
			password: "testpassword",
			mockResponse: `{
				"access_token": "mock_access_token",
				"id_token": "mock_id_token",
				"refresh_token": "mock_refresh_token",
				"token_type": "Bearer",
				"expires_in": 3600,
				"scope": "read write"
			}`,
			mockStatusCode: 200,
			expectError:    false,
			expectedToken: &model.TokenResponse{
				AccessToken:  "mock_access_token",
				IDToken:      "mock_id_token",
				RefreshToken: "mock_refresh_token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				Scope:        "read write",
			},
		},
		{
			name:           "invalid credentials",
			username:       "invaliduser",
			password:       "wrongpassword",
			mockResponse:   `{"error": "invalid_credentials"}`,
			mockStatusCode: 401,
			expectError:    true,
			expectedToken:  nil,
		},
		{
			name:           "server error",
			username:       "testuser",
			password:       "testpassword",
			mockResponse:   `{"error": "internal_server_error"}`,
			mockStatusCode: 500,
			expectError:    true,
			expectedToken:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			httpmock.RegisterResponder("POST", "https://mock-api.com/api/login/oauth/access_token",
				httpmock.NewStringResponder(tc.mockStatusCode, tc.mockResponse))

			factory := &factory.Factory{
				HTTPClient: &http.Client{},
				Env: &environment.Env{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
					URLAPIAuth:   "https://mock-api.com",
				},
			}

			authInstance := &Auth{
				Factory: factory,
			}

			token, err := authInstance.AuthenticateWithCredentials(tc.username, tc.password)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, token)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, token)
				assert.Equal(t, tc.expectedToken.AccessToken, token.AccessToken)
				assert.Equal(t, tc.expectedToken.IDToken, token.IDToken)
				assert.Equal(t, tc.expectedToken.RefreshToken, token.RefreshToken)
				assert.Equal(t, tc.expectedToken.TokenType, token.TokenType)
				assert.Equal(t, tc.expectedToken.ExpiresIn, token.ExpiresIn)
				assert.Equal(t, tc.expectedToken.Scope, token.Scope)
			}

			info := httpmock.GetCallCountInfo()
			assert.Equal(t, 1, info["POST https://mock-api.com/api/login/oauth/access_token"])
		})
	}
}

func TestExchangeToken(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		mockResponse   string
		mockStatusCode int
		expectError    bool
		expectedToken  *model.TokenResponse
	}{
		{
			name: "success",
			code: "valid_code",
			mockResponse: `{
				"access_token": "mock_access_token",
				"id_token": "mock_id_token",
				"refresh_token": "mock_refresh_token",
				"token_type": "Bearer",
				"expires_in": 3600,
				"scope": "read write"
			}`,
			mockStatusCode: 200,
			expectError:    false,
			expectedToken: &model.TokenResponse{
				AccessToken:  "mock_access_token",
				IDToken:      "mock_id_token",
				RefreshToken: "mock_refresh_token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				Scope:        "read write",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			httpmock.RegisterResponder("POST", "https://mock-api.com/api/login/oauth/access_token",
				httpmock.NewStringResponder(tc.mockStatusCode, tc.mockResponse))

			factory := &factory.Factory{
				HTTPClient: &http.Client{},
				Env: &environment.Env{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
					URLAPIAuth:   "https://mock-api.com",
				},
			}

			authInstance := &Auth{
				Factory: factory,
			}

			token, err := authInstance.ExchangeToken(tc.code)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, token)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, token)
				assert.Equal(t, tc.expectedToken.AccessToken, token.AccessToken)
				assert.Equal(t, tc.expectedToken.IDToken, token.IDToken)
				assert.Equal(t, tc.expectedToken.RefreshToken, token.RefreshToken)
				assert.Equal(t, tc.expectedToken.TokenType, token.TokenType)
				assert.Equal(t, tc.expectedToken.ExpiresIn, token.ExpiresIn)
				assert.Equal(t, tc.expectedToken.Scope, token.Scope)
			}

			info := httpmock.GetCallCountInfo()
			assert.Equal(t, 1, info["POST https://mock-api.com/api/login/oauth/access_token"])
		})
	}
}
