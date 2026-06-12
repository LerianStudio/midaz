// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import (
	"fmt"
	"strings"
)

// Config holds the configuration for connecting to HashiCorp Vault.
// Callers are responsible for populating this struct from their own
// environment variables or configuration sources.
//
// The config supports two authentication methods:
//   - AppRole: Uses RoleID and SecretID for machine-to-machine auth
//   - Token: Uses a pre-existing Vault token
//
// Set AuthMethod to specify which credentials to use. The appropriate
// credentials for the selected method must be provided.
type Config struct {
	// Addr is the Vault server address (e.g., "https://vault.example.com:8200").
	Addr string

	// AuthMethod specifies which authentication method to use.
	// Supported values: "approle", "token". Defaults to "approle" if empty.
	AuthMethod AuthMethod

	// RoleID is the AppRole role ID for authentication.
	// Required when AuthMethod is "approle".
	RoleID string

	// SecretID is the AppRole secret ID for authentication.
	// Required when AuthMethod is "approle".
	SecretID string

	// Token is the Vault token for authentication.
	// Required when AuthMethod is "token".
	Token string
}

// Validate checks that all required fields are present and non-empty.
// Validates base configuration and auth-method-specific credentials.
// Returns an error describing validation failures.
func (c Config) Validate() error {
	var missing []string

	if strings.TrimSpace(c.Addr) == "" {
		missing = append(missing, "addr")
	}

	if len(missing) > 0 {
		return fmt.Errorf("vault config missing required fields: %s", strings.Join(missing, ", "))
	}

	// Determine effective auth method (default to approle)
	method := c.AuthMethod
	if method == "" {
		method = AuthMethodAppRole
	}

	if !method.IsValid() {
		return fmt.Errorf("invalid auth method %q: supported methods are %q, %q",
			c.AuthMethod, AuthMethodAppRole, AuthMethodToken)
	}

	// Validate auth-specific credentials
	if err := ValidateAuthCredentials(method, c.RoleID, c.SecretID, c.Token); err != nil {
		return fmt.Errorf("vault auth validation failed: %w", err)
	}

	return nil
}

// EffectiveAuthMethod returns the auth method to use.
// Defaults to AuthMethodAppRole if AuthMethod is empty.
func (c Config) EffectiveAuthMethod() AuthMethod {
	if c.AuthMethod == "" {
		return AuthMethodAppRole
	}

	return c.AuthMethod
}

// IsEmpty returns true if all configuration fields are empty or contain only whitespace.
func (c Config) IsEmpty() bool {
	return strings.TrimSpace(c.Addr) == "" &&
		strings.TrimSpace(c.RoleID) == "" &&
		strings.TrimSpace(c.SecretID) == "" &&
		strings.TrimSpace(c.Token) == ""
}
