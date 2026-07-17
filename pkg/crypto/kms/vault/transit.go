// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/vault/api"
)

// Encrypt encrypts plaintext using the Vault Transit secrets engine.
// The mountPath selects the shared Transit engine for the request (e.g. "transit-mt"
// or "transit-st") and must be non-empty.
// The keyName carries the tenant scope (e.g. "tenant-x_org-123"); the resulting op
// path is "transit-mt/encrypt/tenant-x_org-123".
// Keys are auto-created on first use if the AppRole has create permissions.
//
// Returns the ciphertext in Vault's format: vault:v1:base64-encoded-ciphertext
func (c *Client) Encrypt(ctx context.Context, mountPath, keyName string, plaintext []byte) (string, error) {
	if mountPath == "" {
		return "", fmt.Errorf("vault: empty mount path for key %q", keyName)
	}

	if err := c.ensureAuthenticated(ctx); err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	ciphertext, err := c.encryptInternal(ctx, mountPath, keyName, plaintext)
	if err != nil {
		// Re-authenticate and retry once on permission denied (token expired)
		if isPermissionDenied(err) {
			if reAuthErr := c.reAuthenticate(ctx); reAuthErr != nil {
				return "", fmt.Errorf("re-authentication failed: %w", reAuthErr)
			}

			ciphertext, err = c.encryptInternal(ctx, mountPath, keyName, plaintext)
			if err != nil {
				return "", c.mapMountErr(mountPath, err)
			}

			return ciphertext, nil
		}

		// Fail closed when the Transit mount is absent: surface a typed error
		// instead of falling back to a different mount.
		return "", c.mapMountErr(mountPath, err)
	}

	return ciphertext, nil
}

// encryptInternal performs the actual encryption call to Vault.
func (c *Client) encryptInternal(ctx context.Context, mountPath, keyName string, plaintext []byte) (string, error) {
	path := fmt.Sprintf("%s/encrypt/%s", mountPath, keyName)

	data := map[string]any{
		"plaintext": base64.StdEncoding.EncodeToString(plaintext),
	}

	resp, err := c.logical().WriteWithContext(ctx, path, data)
	if err != nil {
		return "", fmt.Errorf("transit encrypt failed: %w", err)
	}

	if resp == nil || resp.Data == nil {
		return "", fmt.Errorf("transit encrypt returned empty response")
	}

	ciphertext, ok := resp.Data["ciphertext"].(string)
	if !ok || ciphertext == "" {
		return "", fmt.Errorf("transit encrypt response missing ciphertext")
	}

	return ciphertext, nil
}

// Decrypt decrypts ciphertext using the Vault Transit secrets engine.
// The mountPath selects the shared Transit engine for the request (e.g. "transit-mt"
// or "transit-st") and must be non-empty.
// The keyName carries the tenant scope (e.g. "tenant-x_org-123"); the resulting op
// path is "transit-mt/decrypt/tenant-x_org-123".
// The ciphertext must be in Vault's format: vault:v1:base64-encoded-ciphertext
//
// Returns the original plaintext bytes.
func (c *Client) Decrypt(ctx context.Context, mountPath, keyName, ciphertext string) ([]byte, error) {
	if mountPath == "" {
		return nil, fmt.Errorf("vault: empty mount path for key %q", keyName)
	}

	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	plaintext, err := c.decryptInternal(ctx, mountPath, keyName, ciphertext)
	if err != nil {
		// Re-authenticate and retry once on permission denied (token expired)
		if isPermissionDenied(err) {
			if reAuthErr := c.reAuthenticate(ctx); reAuthErr != nil {
				return nil, fmt.Errorf("re-authentication failed: %w", reAuthErr)
			}

			plaintext, err = c.decryptInternal(ctx, mountPath, keyName, ciphertext)
			if err != nil {
				return nil, c.mapMountErr(mountPath, err)
			}

			return plaintext, nil
		}

		// Fail closed when the Transit mount is absent: surface a typed error
		// instead of falling back to a different mount.
		return nil, c.mapMountErr(mountPath, err)
	}

	return plaintext, nil
}

// decryptInternal performs the actual decryption call to Vault.
func (c *Client) decryptInternal(ctx context.Context, mountPath, keyName, ciphertext string) ([]byte, error) {
	path := fmt.Sprintf("%s/decrypt/%s", mountPath, keyName)

	data := map[string]any{
		"ciphertext": ciphertext,
	}

	resp, err := c.logical().WriteWithContext(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("transit decrypt failed: %w", err)
	}

	if resp == nil || resp.Data == nil {
		return nil, fmt.Errorf("transit decrypt returned empty response")
	}

	plaintextB64, ok := resp.Data["plaintext"].(string)
	if !ok || plaintextB64 == "" {
		return nil, fmt.Errorf("transit decrypt response missing plaintext")
	}

	plaintext, err := base64.StdEncoding.DecodeString(plaintextB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode plaintext: %w", err)
	}

	return plaintext, nil
}

// mapMountErr translates a missing-Transit-mount error into the typed
// ErrMountNotFound (wrapped with the mount path for context), so callers can
// match it with errors.Is and fail closed. Non-mount errors pass through
// unchanged.
func (c *Client) mapMountErr(mountPath string, err error) error {
	if isMountNotFound(err) {
		return fmt.Errorf("%w: %s", ErrMountNotFound, mountPath)
	}

	return err
}

// isMountNotFound reports whether err indicates that the targeted Transit mount
// does not exist. A genuinely missing mount is an HTTP 404 *api.ResponseError
// whose body carries the route-not-found signal: Vault 1.21 returns
// "no handler for route ...\". route entry not found." for a missing mount.
//
// A 404 with body "unsupported path" is deliberately NOT classified here: on
// modern Vault that means the mount EXISTS but the requested sub-path is invalid
// (e.g. a Transit key name that violates Vault's naming rules), which must be
// surfaced as a generic error rather than masked as a missing mount. The
// trade-off is that pre-1.x builds, which returned "unsupported path" for a
// missing mount, are no longer classified as mount-not-found.
func isMountNotFound(err error) bool {
	if err == nil {
		return false
	}

	var respErr *api.ResponseError
	if !errors.As(err, &respErr) || respErr.StatusCode != http.StatusNotFound {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "no handler for route") ||
		strings.Contains(msg, "route entry not found")
}
