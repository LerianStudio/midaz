// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/hashicorp/vault/api"
)

// Client provides authenticated access to HashiCorp Vault Transit secrets engine.
// It supports both AppRole and Token authentication methods, with automatic
// re-authentication on token expiry for AppRole auth.
type Client struct {
	config     Config
	vaultAPI   *api.Client
	mu         sync.RWMutex
	isLoggedIn bool
}

// NewClient creates a new Vault client with the given configuration.
// The client is not authenticated until Login is called or an operation is performed.
func NewClient(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid vault config: %w", err)
	}

	vaultCfg := api.DefaultConfig()
	vaultCfg.Address = cfg.Addr

	vaultAPI, err := api.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	return &Client{
		config:   cfg,
		vaultAPI: vaultAPI,
	}, nil
}

// Login authenticates to Vault using the configured auth method.
// For AppRole: performs login API call with role_id and secret_id - validates credentials immediately.
// For Token: sets the provided token directly - NO validation occurs until first operation.
//
// This method is called automatically when needed. For AppRole, calling explicitly
// at startup validates credentials early. For Token auth, validation is deferred
// to the first Encrypt/Decrypt call.
func (c *Client) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.loginLocked(ctx)
}

// loginLocked performs authentication based on the configured method.
// Caller must hold c.mu.
func (c *Client) loginLocked(ctx context.Context) error {
	method := c.config.EffectiveAuthMethod()

	switch method {
	case AuthMethodAppRole:
		return c.loginAppRole(ctx)
	case AuthMethodToken:
		return c.loginToken()
	default:
		return fmt.Errorf("unsupported auth method: %s", method)
	}
}

// loginAppRole authenticates using AppRole credentials.
func (c *Client) loginAppRole(ctx context.Context) error {
	data := map[string]any{
		"role_id":   c.config.RoleID,
		"secret_id": c.config.SecretID,
	}

	resp, err := c.vaultAPI.Logical().WriteWithContext(ctx, "auth/approle/login", data)
	if err != nil {
		return fmt.Errorf("approle login failed: %w", err)
	}

	if resp == nil || resp.Auth == nil {
		return fmt.Errorf("approle login returned empty auth response")
	}

	c.vaultAPI.SetToken(resp.Auth.ClientToken)
	c.isLoggedIn = true

	return nil
}

// loginToken sets the provided token directly without validation.
// Unlike AppRole auth, token auth does not make a Vault API call here.
// The token is only validated on the first actual Vault operation (e.g., Encrypt/Decrypt).
// Invalid or expired tokens will cause errors at operation time, not at login time.
func (c *Client) loginToken() error {
	c.vaultAPI.SetToken(c.config.Token)
	c.isLoggedIn = true

	return nil
}

// ensureAuthenticated ensures the client is logged in.
// If not logged in, it performs authentication using the configured method.
func (c *Client) ensureAuthenticated(ctx context.Context) error {
	c.mu.RLock()
	loggedIn := c.isLoggedIn
	c.mu.RUnlock()

	if loggedIn {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.isLoggedIn {
		return nil
	}

	return c.loginLocked(ctx)
}

// reAuthenticate forces a new login. Called when a 403 error is received.
// For Token auth, this simply re-sets the same token (which may not help
// if the token itself is expired/revoked).
func (c *Client) reAuthenticate(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.isLoggedIn = false

	return c.loginLocked(ctx)
}

// isPermissionDenied checks if the error is a 403 permission denied error.
func isPermissionDenied(err error) bool {
	if err == nil {
		return false
	}

	var respErr *api.ResponseError
	if errors.As(err, &respErr) {
		return respErr.StatusCode == http.StatusForbidden
	}

	return false
}

// Close releases any resources held by the client.
// After Close is called, the client should not be used.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.isLoggedIn = false
	c.vaultAPI.ClearToken()

	return nil
}

// AuthMethod returns the effective authentication method being used.
func (c *Client) AuthMethod() AuthMethod {
	return c.config.EffectiveAuthMethod()
}

// IsAuthenticated returns true if the client has successfully authenticated.
// This is a fast, local check without any network calls.
// For Token auth, this returns true after Login() is called (token set).
// For AppRole auth, this returns true after successful AppRole login.
func (c *Client) IsAuthenticated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.isLoggedIn
}

// Address returns the configured Vault server address.
func (c *Client) Address() string {
	return c.config.Addr
}

// HealthCheck verifies Vault server availability by calling the sys/health endpoint.
// This is an unauthenticated endpoint that returns the health status of the Vault server.
// Returns nil if Vault is healthy (initialized and unsealed), error otherwise.
func (c *Client) HealthCheck(ctx context.Context) error {
	health, err := c.vaultAPI.Sys().HealthWithContext(ctx)
	if err != nil {
		return fmt.Errorf("vault health check failed: %w", err)
	}

	if !health.Initialized {
		return fmt.Errorf("vault is not initialized")
	}

	if health.Sealed {
		return fmt.Errorf("vault is sealed")
	}

	return nil
}

// EnsureTransitMount creates a Transit secrets-engine mount at mountPath.
// It is idempotent: a mount already present at mountPath is treated as success.
// mountPath must be non-empty.
func (c *Client) EnsureTransitMount(ctx context.Context, mountPath string) error {
	if mountPath == "" {
		return fmt.Errorf("vault: empty mount path")
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("vault: context error before transit mount: %w", err)
	}

	if err := c.ensureAuthenticated(ctx); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("vault: context error before transit mount: %w", err)
	}

	err := c.vaultAPI.Sys().MountWithContext(ctx, mountPath, &api.MountInput{Type: "transit"})
	if err != nil {
		if isMountAlreadyExists(err) {
			return nil
		}

		return fmt.Errorf("vault: enable transit mount %q: %w", mountPath, err)
	}

	return nil
}

// isMountAlreadyExists reports whether err indicates that the targeted mount
// path is already in use. Vault surfaces this as an HTTP 400 *api.ResponseError
// with a body containing "path is already in use". Matching the conflict closes
// the TOCTOU window between two concurrent mount attempts.
func isMountAlreadyExists(err error) bool {
	if err == nil {
		return false
	}

	var respErr *api.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == http.StatusBadRequest {
		return strings.Contains(strings.ToLower(err.Error()), "already in use")
	}

	return false
}

// logical returns the Vault logical client for Transit operations.
func (c *Client) logical() *api.Logical {
	return c.vaultAPI.Logical()
}
