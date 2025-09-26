package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"
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
		return fmt.Errorf("TEST_AUTH_USERNAME/TEST_AUTH_PASSWORD must be set when TEST_AUTH_URL is provided")
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

	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest(http.MethodPost, authURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody bytes.Buffer
		if _, readErr := errBody.ReadFrom(resp.Body); readErr != nil {
			return fmt.Errorf("auth request failed with status %d, and could not read body: %w", resp.StatusCode, readErr)
		}

		return fmt.Errorf("auth request failed: status=%d, body: %s", resp.StatusCode, errBody.String())
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
		return fmt.Errorf("auth response missing access token")
	}

	// Export for the duration of the process so helpers.AuthHeaders picks it up
	return os.Setenv("TEST_AUTH_HEADER", "Bearer "+token)
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
