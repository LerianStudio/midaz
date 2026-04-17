// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"
)

// errMissingCredentials is returned when TEST_AUTH_USERNAME or TEST_AUTH_PASSWORD is not set.
var errMissingCredentials = errors.New("TEST_AUTH_USERNAME/TEST_AUTH_PASSWORD must be set when TEST_AUTH_URL is provided")

// errMissingAccessToken is returned when the auth response does not contain an access token.
var errMissingAccessToken = errors.New("auth response missing access token")

const authClientTimeout = 15 * time.Second

// AuthenticateFromEnv obtains a Bearer token using env vars and exports TEST_AUTH_HEADER.
// Inputs via env:
//
//	TEST_AUTH_URL: OAuth token endpoint (e.g., https://auth.../v1/login/oauth/access_token)
//	TEST_AUTH_USERNAME: username
//	TEST_AUTH_PASSWORD: password
//
// If TEST_AUTH_URL is empty, no-op. On success sets TEST_AUTH_HEADER to "Bearer <token>".
func AuthenticateFromEnv() error {
	authURL := os.Getenv("TEST_AUTH_URL")
	if authURL == "" {
		return nil
	}

	username := os.Getenv("TEST_AUTH_USERNAME")
	password := os.Getenv("TEST_AUTH_PASSWORD")

	if username == "" || password == "" {
		return errMissingCredentials
	}

	payload := map[string]string{
		"grantType": "password",
		"username":  username,
		"password":  password,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal auth payload: %w", err)
	}

	client := &http.Client{Timeout: authClientTimeout}

	ctx := context.Background()

	//nolint:gosec // G704: authURL is test-controlled configuration, not user input
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req) //nolint:gosec // G704: SSRF intentional in test helper
	if err != nil {
		return fmt.Errorf("execute auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody bytes.Buffer
		if _, readErr := errBody.ReadFrom(resp.Body); readErr != nil {
			return fmt.Errorf("auth request failed with status %d, and could not read body: %w", resp.StatusCode, readErr)
		}

		return fmt.Errorf("auth request failed: status=%d, body: %s", resp.StatusCode, errBody.String()) //nolint:err113 // dynamic error with context info
	}

	var out struct {
		AccessToken string `json:"accessToken"`
		Token       string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode auth response: %w", err)
	}

	token := out.AccessToken
	if token == "" {
		token = out.Token
	}

	if token == "" {
		return errMissingAccessToken
	}

	// Export for the duration of the process so helpers.AuthHeaders picks it up
	if err := os.Setenv("TEST_AUTH_HEADER", "Bearer "+token); err != nil {
		return fmt.Errorf("set TEST_AUTH_HEADER env var: %w", err)
	}

	return nil
}

// RunTestsWithAuth authenticates using env (if configured) and runs tests, failing fast on auth errors.
// Usage in each package's TestMain:
//
//	func TestMain(m *testing.M) { helpers.RunTestsWithAuth(m) }
func RunTestsWithAuth(m *testing.M) {
	if err := AuthenticateFromEnv(); err != nil {
		log.Fatalf("Failed to authenticate from environment: %v", err) //nolint:forbidigo,revive // log.Fatalf is required in TestMain for early exit
	}

	os.Exit(m.Run()) //nolint:forbidigo,revive // os.Exit is required in TestMain
}
