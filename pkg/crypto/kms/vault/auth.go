// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package vault provides a client for HashiCorp Vault Transit secrets engine.
// It supports envelope encryption operations with organization-scoped keys
// and automatic re-authentication on token expiry.
package vault

import (
	"fmt"
	"strings"
)

// AuthMethod represents the Vault authentication method.
type AuthMethod string

const (
	// AuthMethodAppRole uses AppRole authentication with role_id and secret_id.
	AuthMethodAppRole AuthMethod = "approle"

	// AuthMethodToken uses a pre-existing Vault token.
	AuthMethodToken AuthMethod = "token"
)

// String returns the string representation of the AuthMethod.
func (m AuthMethod) String() string {
	return string(m)
}

// IsValid returns true if the auth method is a supported value.
func (m AuthMethod) IsValid() bool {
	switch m {
	case AuthMethodAppRole, AuthMethodToken:
		return true
	default:
		return false
	}
}

// ParseAuthMethod parses a string into an AuthMethod.
// Returns an error if the string is not a valid auth method.
func ParseAuthMethod(s string) (AuthMethod, error) {
	normalized := AuthMethod(strings.ToLower(strings.TrimSpace(s)))
	if !normalized.IsValid() {
		return "", fmt.Errorf("invalid auth method %q: supported methods are %q, %q",
			s, AuthMethodAppRole, AuthMethodToken)
	}

	return normalized, nil
}

// ValidateAuthCredentials validates that the required credentials are present
// for the specified auth method. Returns an error listing missing credentials.
func ValidateAuthCredentials(method AuthMethod, roleID, secretID, token string) error {
	switch method {
	case AuthMethodAppRole:
		var missing []string

		if strings.TrimSpace(roleID) == "" {
			missing = append(missing, "role_id")
		}

		if strings.TrimSpace(secretID) == "" {
			missing = append(missing, "secret_id")
		}

		if len(missing) > 0 {
			return fmt.Errorf("approle auth requires: %s", strings.Join(missing, ", "))
		}

		return nil

	case AuthMethodToken:
		if strings.TrimSpace(token) == "" {
			return fmt.Errorf("token auth requires: token")
		}

		return nil

	default:
		return fmt.Errorf("unsupported auth method: %s", method)
	}
}
