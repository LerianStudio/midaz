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

const (
	authHTTPTimeout      = 15 * time.Second
	authMinStatusSuccess = 200
	authMaxStatusSuccess = 300
)

var (
	// ErrAuthCredentialsMissing indicates required auth credentials are not provided
	ErrAuthCredentialsMissing = errors.New("TEST_AUTH_USERNAME/TEST_AUTH_PASSWORD must be set when TEST_AUTH_URL is provided")
	// ErrAuthRequestFailed indicates the authentication request failed
	ErrAuthRequestFailed = errors.New("auth request failed")
	// ErrAuthTokenMissing indicates the auth response is missing an access token
	ErrAuthTokenMissing = errors.New("auth response missing access token")
)

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
		return ErrAuthCredentialsMissing
	}

	token, err := fetchAuthToken(authURL, username, password)
	if err != nil {
		return err
	}

	// Export for the duration of the process so helpers.AuthHeaders picks it up
	if err := os.Setenv("TEST_AUTH_HEADER", "Bearer "+token); err != nil {
		return fmt.Errorf("failed to set TEST_AUTH_HEADER: %w", err)
	}

	return nil
}

// fetchAuthToken performs the actual authentication request
func fetchAuthToken(authURL, username, password string) (string, error) {
	payload := map[string]string{
		"grantType": "password",
		"username":  username,
		"password":  password,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth payload: %w", err)
	}

	client := &http.Client{Timeout: authHTTPTimeout}
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < authMinStatusSuccess || resp.StatusCode >= authMaxStatusSuccess {
		var errBody bytes.Buffer
		if _, readErr := errBody.ReadFrom(resp.Body); readErr != nil {
			return "", fmt.Errorf("auth request failed with status %d, and could not read body: %w", resp.StatusCode, readErr)
		}

		return "", fmt.Errorf("%w: status=%d, body: %s", ErrAuthRequestFailed, resp.StatusCode, errBody.String())
	}

	return parseAuthToken(resp)
}

// parseAuthToken extracts the token from the auth response
func parseAuthToken(resp *http.Response) (string, error) {
	var out struct {
		AccessToken string `json:"accessToken"`
		Token       string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode auth response: %w", err)
	}

	token := out.AccessToken
	if token == "" {
		token = out.Token
	}

	if token == "" {
		return "", ErrAuthTokenMissing
	}

	return token, nil
}

// RunTestsWithAuth authenticates using env (if configured) and runs tests, failing fast on auth errors.
// Usage in each package's TestMain:
//
//	func TestMain(m *testing.M) { helpers.RunTestsWithAuth(m) }
func RunTestsWithAuth(m *testing.M) {
	if err := AuthenticateFromEnv(); err != nil {
		log.Fatalf("Failed to authenticate from environment: %v", err)
	}

	os.Exit(m.Run())
}
