// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import (
	"context"
	"encoding/base64"
	"fmt"
)

// Encrypt encrypts plaintext using the Vault Transit secrets engine.
// The keyName should follow the convention: org/{organization_id}
// Keys are auto-created on first use if the AppRole has create permissions.
//
// Returns the ciphertext in Vault's format: vault:v1:base64-encoded-ciphertext
func (c *Client) Encrypt(ctx context.Context, keyName string, plaintext []byte) (string, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	ciphertext, err := c.encryptInternal(ctx, keyName, plaintext)
	if err != nil {
		// Re-authenticate and retry once on permission denied (token expired)
		if isPermissionDenied(err) {
			if reAuthErr := c.reAuthenticate(ctx); reAuthErr != nil {
				return "", fmt.Errorf("re-authentication failed: %w", reAuthErr)
			}

			return c.encryptInternal(ctx, keyName, plaintext)
		}

		return "", err
	}

	return ciphertext, nil
}

// encryptInternal performs the actual encryption call to Vault.
func (c *Client) encryptInternal(ctx context.Context, keyName string, plaintext []byte) (string, error) {
	path := fmt.Sprintf("%s/encrypt/%s", c.config.MountPath, keyName)

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
// The ciphertext must be in Vault's format: vault:v1:base64-encoded-ciphertext
//
// Returns the original plaintext bytes.
func (c *Client) Decrypt(ctx context.Context, keyName string, ciphertext string) ([]byte, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	plaintext, err := c.decryptInternal(ctx, keyName, ciphertext)
	if err != nil {
		// Re-authenticate and retry once on permission denied (token expired)
		if isPermissionDenied(err) {
			if reAuthErr := c.reAuthenticate(ctx); reAuthErr != nil {
				return nil, fmt.Errorf("re-authentication failed: %w", reAuthErr)
			}

			return c.decryptInternal(ctx, keyName, ciphertext)
		}

		return nil, err
	}

	return plaintext, nil
}

// decryptInternal performs the actual decryption call to Vault.
func (c *Client) decryptInternal(ctx context.Context, keyName string, ciphertext string) ([]byte, error) {
	path := fmt.Sprintf("%s/decrypt/%s", c.config.MountPath, keyName)

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
