// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"

	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"

	"go.opentelemetry.io/otel/attribute"
)

// KeysetUnwrapper defines the interface for unwrapping keysets.
// Compatible with KeysetWrapper from pkg/crypto/tink.
//
// mountPath is the resolved Vault Transit mount for the keyset's tenant; it is
// derived from the stored keyset tenant at unwrap time so reads are self-contained.
type KeysetUnwrapper interface {
	UnwrapKeyset(ctx context.Context, mountPath, keyName, wrappedKeyset string) ([]byte, error)
}

// CachedPrimitives holds the unwrapped AEAD and MAC primitives for an organization.
type CachedPrimitives struct {
	AEAD         *tink.AEADPrimitive
	MAC          *tink.MACPrimitive
	MultiKeyMAC  *tink.MACMultiPrimitive
	PrimaryKeyID uint32
	ExpiresAt    time.Time
}

// IsExpired returns true if the cached primitives have expired.
func (cp *CachedPrimitives) IsExpired() bool {
	return cp.ExpiresAt.IsZero() || time.Now().After(cp.ExpiresAt)
}

// KeysetManagerConfig holds configuration for KeysetManager.
type KeysetManagerConfig struct {
	CacheTTL time.Duration // Default: 5 minutes

	// BaseMountPath is the Vault Transit base mount used to resolve per-tenant
	// sub-mounts at unwrap time. Empty defaults to "transit" so single-tenant
	// deployments are unaffected.
	BaseMountPath string
}

// DefaultKeysetManagerConfig returns the default configuration.
func DefaultKeysetManagerConfig() KeysetManagerConfig {
	return KeysetManagerConfig{
		CacheTTL: 5 * time.Minute,
	}
}

// KeysetManager retrieves and caches unwrapped Tink primitives for organizations.
// It handles the KMS unwrap operation and caches results to minimize KMS calls.
// Uses per-tenant-organization mutexes to prevent cache stampede when multiple concurrent
// requests attempt to fetch the same organization's keyset.
//
// KeysetManager auto-provisions organizations on first access if their keyset is not
// found (lazy provisioning). Tenant ID for provisioning is obtained from context via
// tmcore.GetTenantIDContext - callers must ensure tenant ID is set in context.
//
// Cache keys are scoped by tenant to prevent cross-tenant cache collisions.
// Format: "tenantID:organizationID"
type KeysetManager struct {
	keysetRepo    mongoEncryption.KeysetRepository
	unwrapper     KeysetUnwrapper
	provisioner   ProvisioningService // Required: enables lazy provisioning on first access
	metrics       *protectionMetrics
	cacheTTL      time.Duration
	baseMountPath string                       // Vault Transit base mount for per-tenant resolution
	cache         map[string]*CachedPrimitives // Key: "tenantID:organizationID"
	mu            sync.RWMutex

	// Per-tenant-organization locks to prevent concurrent fetches for the same tenant+org.
	// This avoids cache stampede without blocking unrelated tenant-organizations.
	fetchMu  sync.Mutex
	fetching map[string]*sync.Mutex // Key: "tenantID:organizationID"
}

// NewKeysetManager creates a new keyset manager with the given dependencies.
// The provisioner enables lazy provisioning for organizations without existing keysets.
// Tenant ID for provisioning is obtained from context - callers must ensure it is set.
// metrics is the nil-safe protection metrics seam; a nil value defaults to
// NewProtectionMetrics(nil) so emission is a no-op when telemetry is disabled.
func NewKeysetManager(
	keysetRepo mongoEncryption.KeysetRepository,
	unwrapper KeysetUnwrapper,
	provisioner ProvisioningService,
	config KeysetManagerConfig,
	metrics *protectionMetrics,
) *KeysetManager {
	ttl := config.CacheTTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	mountPath := config.BaseMountPath
	if mountPath == "" {
		mountPath = "transit"
	}

	if metrics == nil {
		metrics = NewProtectionMetrics(nil)
	}

	return &KeysetManager{
		keysetRepo:    keysetRepo,
		unwrapper:     unwrapper,
		provisioner:   provisioner,
		metrics:       metrics,
		cacheTTL:      ttl,
		baseMountPath: mountPath,
		cache:         make(map[string]*CachedPrimitives),
		fetching:      make(map[string]*sync.Mutex),
	}
}

// buildCacheKey constructs a tenant-scoped cache key.
// Format: "tenantID:organizationID" to prevent cross-tenant cache collisions.
func buildCacheKey(tenantID, organizationID string) string {
	return tenantID + ":" + organizationID
}

// getOrgLock returns the mutex for a specific tenant-organization combination.
// Creates a new mutex if one doesn't exist for the tenant-organization.
func (km *KeysetManager) getOrgLock(cacheKey string) *sync.Mutex {
	km.fetchMu.Lock()
	defer km.fetchMu.Unlock()

	lock, ok := km.fetching[cacheKey]
	if !ok {
		lock = &sync.Mutex{}
		km.fetching[cacheKey] = lock
	}

	return lock
}

// GetPrimitives retrieves the AEAD, MAC, and MACMulti primitives for an organization.
// Returns cached primitives if available and not expired.
// Otherwise, fetches from repository, unwraps via KMS, caches, and returns.
//
// Returns:
//   - AEAD primitive for encryption/decryption
//   - MAC primitive for search token generation (primary key only)
//   - MACMulti primitive for search token generation across all keys (for key rotation)
//   - Primary AEAD key ID for envelope marker formatting
//
// Uses per-tenant-organization mutexes to deduplicate concurrent requests for the same
// tenant-organization, preventing cache stampede while allowing concurrent fetches for
// different tenant-organizations.
//
// Cache keys are scoped by tenant ID to prevent cross-tenant cache collisions.
func (km *KeysetManager) GetPrimitives(ctx context.Context, organizationID string) (*tink.AEADPrimitive, *tink.MACPrimitive, *tink.MACMultiPrimitive, uint32, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, 0, err
	}

	// Extract tenant ID for cache key scoping
	tenantID := ExtractTenantID(ctx)
	cacheKey := buildCacheKey(tenantID, organizationID)

	// Fast path: check cache with read lock
	km.mu.RLock()
	cached, ok := km.cache[cacheKey]
	km.mu.RUnlock()

	if ok && !cached.IsExpired() {
		// Fast-path cache hit: record exactly one hit.
		km.metrics.recordCache(ctx, "get_primitives", "hit")

		return cached.AEAD, cached.MAC, cached.MultiKeyMAC, cached.PrimaryKeyID, nil
	}

	// Cache miss or expired - acquire per-tenant-organization lock
	orgLock := km.getOrgLock(cacheKey)
	orgLock.Lock()
	defer orgLock.Unlock()

	// Double-check cache after acquiring lock (another goroutine may have fetched)
	km.mu.RLock()
	cached, ok = km.cache[cacheKey]
	km.mu.RUnlock()

	if ok && !cached.IsExpired() {
		// Double-check cache hit: another goroutine populated the cache while we
		// waited on the lock. Record exactly one hit (no double-count with the
		// fast path, which already returned for the fast-path-hit case).
		km.metrics.recordCache(ctx, "get_primitives", "hit")

		return cached.AEAD, cached.MAC, cached.MultiKeyMAC, cached.PrimaryKeyID, nil
	}

	// Cache miss: we will fetch and cache. Record exactly one miss per fetch.
	km.metrics.recordCache(ctx, "get_primitives", "miss")

	// Fetch and cache while holding org lock
	primitives, err := km.fetchAndCache(ctx, cacheKey, organizationID)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	return primitives.AEAD, primitives.MAC, primitives.MultiKeyMAC, primitives.PrimaryKeyID, nil
}

// fetchAndCache fetches keyset from repository, unwraps via KMS, and caches the primitives.
// Caller MUST hold the per-tenant-organization lock before calling this method.
//
// When a keyset is not found and a Provisioner is configured, this method
// automatically provisions the organization before retrying the fetch.
//
// The cacheKey parameter is the tenant-scoped key (tenantID:organizationID).
// The organizationID parameter is needed for repository lookups and provisioning.
func (km *KeysetManager) fetchAndCache(ctx context.Context, cacheKey, organizationID string) (*CachedPrimitives, error) {
	// Check context before expensive KMS operations
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Fetch keyset from repository (repository extracts tenant from context)
	keyset, err := km.keysetRepo.Get(ctx, organizationID)
	if err != nil {
		// If keyset not found and provisioner available, try to auto-provision
		if km.provisioner != nil && (errors.Is(err, constant.ErrKeysetNotFound) || errors.Is(err, mmodel.ErrKeysetNotFound)) {
			if provErr := km.autoProvision(ctx, organizationID); provErr != nil {
				return nil, fmt.Errorf("auto-provision failed: %w", provErr)
			}

			// Retry fetch after provisioning
			keyset, err = km.keysetRepo.Get(ctx, organizationID)
			if err != nil {
				return nil, fmt.Errorf("failed to get keyset after provisioning: %w", err)
			}
		} else {
			return nil, err
		}
	}

	// Guard against nil keyset (repository returned nil without error)
	if keyset == nil {
		return nil, constant.ErrKeysetNotFound
	}

	// Check context before KMS unwrap
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Resolve the Vault Transit mount from the STORED keyset tenant (not ctx) so
	// reads are self-contained: flat base for single-tenant ("" or "default"),
	// per-tenant sub-mount otherwise. The same mount serves both AEAD and MAC.
	mount := resolveMount(km.baseMountPath, keyset.TenantID)

	// app.protection.mount_path is the resolved mount, not a secret.
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // NewTrackingFromContext returns 4 values; only the tracer is needed here

	ctx, span := tracer.Start(ctx, "service.protection.keyset_manager.unwrap")
	defer span.End()

	span.SetAttributes(attribute.String("app.protection.mount_path", mount))

	// Unwrap AEAD keyset. Provider is "vault" today (envelope encryption is
	// Vault-only); this becomes dynamic when other KMS providers land.
	// Provider operation timing is recorded even when the unwrap fails.
	aeadStart := time.Now()
	aeadBytes, err := km.unwrapper.UnwrapKeyset(ctx, mount, keyset.KEKPath, keyset.WrappedKeyset)
	km.metrics.recordProviderOperation(ctx, providerOperationUnwrap, providerVault, time.Since(aeadStart).Milliseconds())

	if err != nil {
		km.metrics.recordProviderFailure(ctx, providerOperationUnwrap, errorCodeUnwrapAEADFailed)
		libOpenTelemetry.HandleSpanError(span, "failed to unwrap AEAD keyset", err)

		// Wrap with %w so callers can errors.Is(vault.ErrMountNotFound) (fail-closed).
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

	// Unwrap MAC keyset using the same resolved mount. Provider operation timing
	// is recorded even on failure.
	macStart := time.Now()
	macBytes, err := km.unwrapper.UnwrapKeyset(ctx, mount, keyset.KEKPath, keyset.WrappedHMACKeyset)
	km.metrics.recordProviderOperation(ctx, providerOperationUnwrap, providerVault, time.Since(macStart).Milliseconds())

	if err != nil {
		km.metrics.recordProviderFailure(ctx, providerOperationUnwrap, errorCodeUnwrapMACFailed)
		libOpenTelemetry.HandleSpanError(span, "failed to unwrap MAC keyset", err)

		// Wrap with %w so callers can errors.Is(vault.ErrMountNotFound) (fail-closed).
		return nil, fmt.Errorf("failed to unwrap MAC keyset: %w", err)
	}

	// Deserialize MAC keyset to get handle for creating both primitives
	macHandle, err := tink.DeserializeMACKeyset(macBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize MAC keyset: %w", err)
	}

	// Create MAC primitive from handle
	macPrimitive, err := tink.NewMACPrimitive(macHandle)
	if err != nil {
		return nil, fmt.Errorf("failed to create MAC primitive: %w", err)
	}

	// Create multi-key MAC primitive for search operations (strict mode: error fails the operation)
	multiKeyMAC, err := tink.NewMACMultiPrimitive(macHandle)
	if err != nil {
		return nil, fmt.Errorf("failed to create multi-key MAC primitive: %w", err)
	}

	// Build cached primitives
	cached := &CachedPrimitives{
		AEAD:         aeadPrimitive,
		MAC:          macPrimitive,
		MultiKeyMAC:  multiKeyMAC,
		PrimaryKeyID: keyset.KeysetInfo.PrimaryKeyID,
		ExpiresAt:    time.Now().Add(km.cacheTTL),
	}

	// Cache the primitives with write lock using tenant-scoped key
	km.mu.Lock()
	km.cache[cacheKey] = cached
	km.mu.Unlock()

	return cached, nil
}

// autoProvision provisions an organization using the injected provisioner.
// Tenant ID is extracted from context, defaulting to "default" for single-tenant mode.
func (km *KeysetManager) autoProvision(ctx context.Context, organizationID string) error {
	if km.provisioner == nil {
		return fmt.Errorf("provisioner not configured")
	}

	// Use ExtractTenantID which defaults to "default" for single-tenant mode
	tenantID := ExtractTenantID(ctx)

	_, err := km.provisioner.Provision(ctx, ProvisionInput{
		TenantID:       tenantID,
		OrganizationID: organizationID,
		Actor:          "system:auto-provision",
		Reason:         "Auto-provisioned on first encrypted field access",
	})

	return err
}

// InvalidateCacheForTenant removes the cached primitives for a specific tenant-organization.
// Call this after key rotation or when keyset is updated.
// Also removes the per-tenant-organization mutex to prevent unbounded map growth.
func (km *KeysetManager) InvalidateCacheForTenant(tenantID, organizationID string) {
	cacheKey := buildCacheKey(tenantID, organizationID)

	km.mu.Lock()
	delete(km.cache, cacheKey)
	km.mu.Unlock()

	// Clean up per-tenant-org mutex to prevent unbounded growth
	km.fetchMu.Lock()
	delete(km.fetching, cacheKey)
	km.fetchMu.Unlock()
}

// InvalidateCache removes the cached primitives for an organization in the default tenant.
// For multi-tenant environments, use InvalidateCacheForTenant instead.
// Call this after key rotation or when keyset is updated.
// Also removes the per-organization mutex to prevent unbounded map growth.
func (km *KeysetManager) InvalidateCache(organizationID string) {
	km.InvalidateCacheForTenant("default", organizationID)
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
