// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
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
// Uses per-organization mutexes to prevent cache stampede when multiple concurrent
// requests attempt to fetch the same organization's keyset.
type KeysetManager struct {
	keysetReader KeysetReader
	unwrapper    KeysetUnwrapper
	cacheTTL     time.Duration
	cache        map[string]*CachedPrimitives
	mu           sync.RWMutex

	// Per-organization locks to prevent concurrent fetches for the same org.
	// This avoids cache stampede without blocking unrelated organizations.
	fetchMu  sync.Mutex
	fetching map[string]*sync.Mutex
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
		fetching:     make(map[string]*sync.Mutex),
	}
}

// getOrgLock returns the mutex for a specific organization.
// Creates a new mutex if one doesn't exist for the organization.
func (km *KeysetManager) getOrgLock(organizationID string) *sync.Mutex {
	km.fetchMu.Lock()
	defer km.fetchMu.Unlock()

	lock, ok := km.fetching[organizationID]
	if !ok {
		lock = &sync.Mutex{}
		km.fetching[organizationID] = lock
	}

	return lock
}

// GetPrimitives retrieves the AEAD and MAC primitives for an organization.
// Returns cached primitives if available and not expired.
// Otherwise, fetches from repository, unwraps via KMS, caches, and returns.
//
// Uses per-organization mutexes to deduplicate concurrent requests for the same
// organization, preventing cache stampede while allowing concurrent fetches for
// different organizations.
func (km *KeysetManager) GetPrimitives(ctx context.Context, organizationID string) (*tink.AEADPrimitive, *tink.MACPrimitive, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// Fast path: check cache with read lock
	km.mu.RLock()
	cached, ok := km.cache[organizationID]
	km.mu.RUnlock()

	if ok && !cached.IsExpired() {
		return cached.AEAD, cached.MAC, nil
	}

	// Cache miss or expired - acquire per-organization lock
	orgLock := km.getOrgLock(organizationID)
	orgLock.Lock()
	defer orgLock.Unlock()

	// Double-check cache after acquiring lock (another goroutine may have fetched)
	km.mu.RLock()
	cached, ok = km.cache[organizationID]
	km.mu.RUnlock()

	if ok && !cached.IsExpired() {
		return cached.AEAD, cached.MAC, nil
	}

	// Fetch and cache while holding org lock
	primitives, err := km.fetchAndCache(ctx, organizationID)
	if err != nil {
		return nil, nil, err
	}

	return primitives.AEAD, primitives.MAC, nil
}

// fetchAndCache fetches keyset from repository, unwraps via KMS, and caches the primitives.
// Caller MUST hold the per-organization lock before calling this method.
func (km *KeysetManager) fetchAndCache(ctx context.Context, organizationID string) (*CachedPrimitives, error) {
	// Check context before expensive KMS operations
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Fetch keyset from repository
	keyset, err := km.keysetReader.Get(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	// Guard against nil keyset (repository returned nil without error)
	if keyset == nil {
		return nil, constant.ErrKeysetNotFound
	}

	// Check context before KMS unwrap
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Unwrap AEAD keyset
	aeadBytes, err := km.unwrapper.UnwrapKeyset(ctx, keyset.KEKPath, keyset.WrappedKeyset)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap AEAD keyset: %w", err)
	}

	// Parse AEAD keyset into primitive
	aeadPrimitive, err := tink.ParseAEADKeyset(aeadBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AEAD keyset: %w", err)
	}

	// Check context before second KMS unwrap
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Unwrap MAC keyset
	macBytes, err := km.unwrapper.UnwrapKeyset(ctx, keyset.KEKPath, keyset.WrappedHMACKeyset)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap MAC keyset: %w", err)
	}

	// Parse MAC keyset into primitive
	macPrimitive, err := tink.ParseMACKeyset(macBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MAC keyset: %w", err)
	}

	// Build cached primitives
	cached := &CachedPrimitives{
		AEAD:      aeadPrimitive,
		MAC:       macPrimitive,
		ExpiresAt: time.Now().Add(km.cacheTTL),
	}

	// Cache the primitives with write lock
	km.mu.Lock()
	km.cache[organizationID] = cached
	km.mu.Unlock()

	return cached, nil
}

// InvalidateCache removes the cached primitives for an organization.
// Call this after key rotation or when keyset is updated.
// Also removes the per-organization mutex to prevent unbounded map growth.
func (km *KeysetManager) InvalidateCache(organizationID string) {
	km.mu.Lock()
	delete(km.cache, organizationID)
	km.mu.Unlock()

	// Clean up per-org mutex to prevent unbounded growth
	km.fetchMu.Lock()
	delete(km.fetching, organizationID)
	km.fetchMu.Unlock()
}

// ClearCache removes all cached primitives and per-organization mutexes.
// This prevents unbounded memory growth from the fetching map.
func (km *KeysetManager) ClearCache() {
	km.mu.Lock()
	km.cache = make(map[string]*CachedPrimitives)
	km.mu.Unlock()

	// Clean up all per-org mutexes to prevent unbounded growth
	km.fetchMu.Lock()
	km.fetching = make(map[string]*sync.Mutex)
	km.fetchMu.Unlock()
}
