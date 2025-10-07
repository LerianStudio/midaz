// Package rest provides REST API client implementations for the MDZ CLI.
// This file contains authentication-related REST operations.
package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/LerianStudio/midaz/v3/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"
)

// Auth implements authentication operations via REST API.
//
// This struct provides OAuth authentication methods for the CLI,
// supporting both username/password and authorization code flows.
type Auth struct {
	Factory *factory.Factory
}

// AuthenticateWithCredentials authenticates using username and password (OAuth password grant).
//
// This method implements the OAuth 2.0 password grant flow:
// 1. Constructs form data with credentials and client info
// 2. POSTs to OAuth token endpoint
// 3. Parses and returns access token response
//
// Parameters:
//   - username: User's username
//   - password: User's password
//
// Returns:
//   - *model.TokenResponse: Access token and metadata
//   - error: Authentication or HTTP error
func (r *Auth) AuthenticateWithCredentials(
	username, password string,
) (*model.TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", r.Factory.Env.ClientID)
	data.Set("client_secret", r.Factory.Env.ClientSecret)
	data.Set("username", username)
	data.Set("password", password)

	resp, err := http.PostForm(
		r.Factory.Env.URLAPIAuth+"/api/login/oauth/access_token", data)
	if err != nil {
		return nil, errors.New("error request: " + err.Error())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("error when reading the answer: " + err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var tokenResponse model.TokenResponse

	if err = json.Unmarshal(body, &tokenResponse); err != nil {
		return nil, errors.New("error processing the answer: " + err.Error())
	}

	return &tokenResponse, nil
}

// ExchangeToken exchanges an authorization code for an access token (OAuth authorization code grant).
//
// This method implements the OAuth 2.0 authorization code flow:
// 1. Constructs form data with authorization code and client info
// 2. POSTs to OAuth token endpoint
// 3. Parses and returns access token response
//
// Used for browser-based authentication flow.
//
// Parameters:
//   - code: Authorization code from OAuth callback
//
// Returns:
//   - *model.TokenResponse: Access token and metadata
//   - error: Authentication or HTTP error
func (r *Auth) ExchangeToken(code string) (*model.TokenResponse, error) {
	redirectURI := "http://localhost:9000/callback"

	data := url.Values{}
	data.Set("client_id", r.Factory.Env.ClientID)
	data.Set("client_secret", r.Factory.Env.ClientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	resp, err := http.PostForm(
		r.Factory.Env.URLAPIAuth+"/api/login/oauth/access_token", data)
	if err != nil {
		return nil, errors.New("request error: " + err.Error())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("request error: " + err.Error())
	}

	var tokenResponse model.TokenResponse

	if err = json.Unmarshal(body, &tokenResponse); err != nil {
		return nil, errors.New("error processing the answer: " + err.Error())
	}

	return &tokenResponse, nil
}

// NewAuth creates a new Auth instance.
//
// Parameters:
//   - f: Factory with HTTP client and environment configuration
//
// Returns:
//   - *Auth: Initialized Auth instance
func NewAuth(f *factory.Factory) *Auth {
	return &Auth{f}
}
