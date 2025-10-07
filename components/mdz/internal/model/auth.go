// Package model provides data models for the MDZ CLI internal layer.
// This file contains authentication-related models.
package model

// TokenResponse represents an OAuth token response from the authentication API.
//
// This struct maps to the OAuth 2.0 token response format, containing
// access tokens, refresh tokens, and token metadata.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}
