// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// KeysetReader defines the interface for reading organization keysets.
// Compatible with the KeysetRepository in the MongoDB adapter.
type KeysetReader interface {
	Get(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error)
}

// KeysetUnwrapper defines the interface for unwrapping keysets.
// Compatible with KeysetWrapper from pkg/crypto/tink.
type KeysetUnwrapper interface {
	UnwrapKeyset(ctx context.Context, keyName string, wrappedKeyset string) ([]byte, error)
}

// CachedPrimitives holds the unwrapped AEAD and MAC primitives for an organization.
type CachedPrimitives struct {
	AEAD      *tink.AEADPrimitive
	MAC       *tink.MACPrimitive
	ExpiresAt time.Time
}

// IsExpired returns true if the cached primitives have expired.
func (cp *CachedPrimitives) IsExpired() bool {
	return cp.ExpiresAt.IsZero() || time.Now().After(cp.ExpiresAt)
}

// KeysetManagerConfig holds configuration for KeysetManager.
type KeysetManagerConfig struct {
	CacheTTL time.Duration // Default: 5 minutes
}

// DefaultKeysetManagerConfig returns the default configuration.
func DefaultKeysetManagerConfig() KeysetManagerConfig {
	return KeysetManagerConfig{
		CacheTTL: 5 * time.Minute,
	}
}

// KeysetManager retrieves and caches unwrapped Tink primitives for organizations.
// It handles the KMS unwrap operation and caches results to minimize KMS calls.
type KeysetManager struct {
	keysetReader KeysetReader
	unwrapper    KeysetUnwrapper
	cacheTTL     time.Duration
	cache        map[string]*CachedPrimitives
	mu           sync.RWMutex
}

// NewKeysetManager creates a new keyset manager with the given dependencies.
func NewKeysetManager(
	keysetReader KeysetReader,
	unwrapper KeysetUnwrapper,
	config KeysetManagerConfig,
) *KeysetManager {
	ttl := config.CacheTTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	return &KeysetManager{
		keysetReader: keysetReader,
		unwrapper:    unwrapper,
		cacheTTL:     ttl,
		cache:        make(map[string]*CachedPrimitives),
	}
}

// GetPrimitives retrieves the AEAD and MAC primitives for an organization.
// Returns cached primitives if available and not expired.
// Otherwise, fetches from repository, unwraps via KMS, caches, and returns.
func (km *KeysetManager) GetPrimitives(ctx context.Context, organizationID string) (*tink.AEADPrimitive, *tink.MACPrimitive, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// Check cache with read lock
	km.mu.RLock()
	cached, ok := km.cache[organizationID]
	km.mu.RUnlock()

	if ok && !cached.IsExpired() {
		return cached.AEAD, cached.MAC, nil
	}

	// Cache miss or expired - fetch and unwrap
	return km.fetchAndCache(ctx, organizationID)
}

// fetchAndCache fetches keyset from repository, unwraps, and caches the primitives.
func (km *KeysetManager) fetchAndCache(ctx context.Context, organizationID string) (*tink.AEADPrimitive, *tink.MACPrimitive, error) {
	// Check context before expensive KMS operations
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// Fetch keyset from repository
	keyset, err := km.keysetReader.Get(ctx, organizationID)
	if err != nil {
		return nil, nil, err
	}

	// Check context before KMS unwrap
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// Unwrap AEAD keyset
	aeadBytes, err := km.unwrapper.UnwrapKeyset(ctx, keyset.KEKPath, keyset.WrappedKeyset)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unwrap AEAD keyset: %w", err)
	}

	// Parse AEAD keyset into primitive
	aeadPrimitive, err := tink.ParseAEADKeyset(aeadBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse AEAD keyset: %w", err)
	}

	// Check context before second KMS unwrap
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// Unwrap MAC keyset
	macBytes, err := km.unwrapper.UnwrapKeyset(ctx, keyset.KEKPath, keyset.WrappedHMACKeyset)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unwrap MAC keyset: %w", err)
	}

	// Parse MAC keyset into primitive
	macPrimitive, err := tink.ParseMACKeyset(macBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse MAC keyset: %w", err)
	}

	// Cache the primitives with write lock
	km.mu.Lock()
	km.cache[organizationID] = &CachedPrimitives{
		AEAD:      aeadPrimitive,
		MAC:       macPrimitive,
		ExpiresAt: time.Now().Add(km.cacheTTL),
	}
	km.mu.Unlock()

	return aeadPrimitive, macPrimitive, nil
}

// InvalidateCache removes the cached primitives for an organization.
// Call this after key rotation or when keyset is updated.
func (km *KeysetManager) InvalidateCache(organizationID string) {
	km.mu.Lock()
	defer km.mu.Unlock()

	delete(km.cache, organizationID)
}

// ClearCache removes all cached primitives.
func (km *KeysetManager) ClearCache() {
	km.mu.Lock()
	defer km.mu.Unlock()

	km.cache = make(map[string]*CachedPrimitives)
}
