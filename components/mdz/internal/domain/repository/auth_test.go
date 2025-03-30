package repository_test

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestAuthInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuth := repository.NewMockAuth(ctrl)
	
	// Test interface compliance by using the mock
	var _ repository.Auth = mockAuth
}

func TestAuth_AuthenticateWithCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuth := repository.NewMockAuth(ctrl)
	
	testCases := []struct {
		name           string
		username       string
		password       string
		mockSetup      func()
		expectedResult *model.TokenResponse
		expectedError  error
	}{
		{
			name:     "success",
			username: "test@example.com",
			password: "password123",
			mockSetup: func() {
				expectedToken := &model.TokenResponse{
					AccessToken:  "access_token_123",
					IDToken:      "id_token_123",
					RefreshToken: "refresh_token_123",
					TokenType:    "Bearer",
					ExpiresIn:    3600,
					Scope:        "openid profile email",
				}
				mockAuth.EXPECT().
					AuthenticateWithCredentials("test@example.com", "password123").
					Return(expectedToken, nil)
			},
			expectedResult: &model.TokenResponse{
				AccessToken:  "access_token_123",
				IDToken:      "id_token_123",
				RefreshToken: "refresh_token_123",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				Scope:        "openid profile email",
			},
			expectedError: nil,
		},
		{
			name:     "invalid_credentials",
			username: "test@example.com",
			password: "wrong_password",
			mockSetup: func() {
				mockAuth.EXPECT().
					AuthenticateWithCredentials("test@example.com", "wrong_password").
					Return(nil, errors.New("invalid credentials"))
			},
			expectedResult: nil,
			expectedError:  errors.New("invalid credentials"),
		},
		{
			name:     "server_error",
			username: "test@example.com",
			password: "password123",
			mockSetup: func() {
				mockAuth.EXPECT().
					AuthenticateWithCredentials("test@example.com", "password123").
					Return(nil, errors.New("server error"))
			},
			expectedResult: nil,
			expectedError:  errors.New("server error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			
			result, err := mockAuth.AuthenticateWithCredentials(tc.username, tc.password)
			
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
			
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestAuth_ExchangeToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuth := repository.NewMockAuth(ctrl)
	
	testCases := []struct {
		name           string
		code           string
		mockSetup      func()
		expectedResult *model.TokenResponse
		expectedError  error
	}{
		{
			name: "success",
			code: "authorization_code_123",
			mockSetup: func() {
				expectedToken := &model.TokenResponse{
					AccessToken:  "access_token_123",
					IDToken:      "id_token_123",
					RefreshToken: "refresh_token_123",
					TokenType:    "Bearer",
					ExpiresIn:    3600,
					Scope:        "openid profile email",
				}
				mockAuth.EXPECT().
					ExchangeToken("authorization_code_123").
					Return(expectedToken, nil)
			},
			expectedResult: &model.TokenResponse{
				AccessToken:  "access_token_123",
				IDToken:      "id_token_123",
				RefreshToken: "refresh_token_123",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				Scope:        "openid profile email",
			},
			expectedError: nil,
		},
		{
			name: "invalid_code",
			code: "invalid_code",
			mockSetup: func() {
				mockAuth.EXPECT().
					ExchangeToken("invalid_code").
					Return(nil, errors.New("invalid authorization code"))
			},
			expectedResult: nil,
			expectedError:  errors.New("invalid authorization code"),
		},
		{
			name: "server_error",
			code: "authorization_code_123",
			mockSetup: func() {
				mockAuth.EXPECT().
					ExchangeToken("authorization_code_123").
					Return(nil, errors.New("server error"))
			},
			expectedResult: nil,
			expectedError:  errors.New("server error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			
			result, err := mockAuth.ExchangeToken(tc.code)
			
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
			
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
