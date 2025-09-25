package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	body, _ := json.Marshal(payload)

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
		return fmt.Errorf("auth request failed: status=%d", resp.StatusCode)
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
