// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
// For AppRole: performs login with role_id and secret_id.
// For Token: sets the provided token directly.
//
// This method is called automatically when needed, but can be called
// explicitly to verify credentials at startup.
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

// loginToken sets the provided token directly.
// Token auth does not perform a login call - it assumes the token is valid.
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

// MountPath returns the configured Transit secrets engine mount path.
func (c *Client) MountPath() string {
	return c.config.MountPath
}

// AuthMethod returns the effective authentication method being used.
func (c *Client) AuthMethod() AuthMethod {
	return c.config.EffectiveAuthMethod()
}

// logical returns the Vault logical client for Transit operations.
func (c *Client) logical() *api.Logical {
	return c.vaultAPI.Logical()
}
