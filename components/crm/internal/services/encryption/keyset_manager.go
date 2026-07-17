// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"

	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"

	"go.opentelemetry.io/otel/attribute"
)

// KeysetUnwrapper defines the interface for unwrapping keysets.
// Compatible with KeysetWrapper from pkg/crypto/tink.
//
// mountPath is the resolved Vault Transit mount for the keyset. At unwrap time it
// is read from the keyset's stored KEKMountPath (the wrap-time mount), falling back
// to deriving it from the stored tenant only for legacy records, so reads are
// self-contained and independent of live config.
type KeysetUnwrapper interface {
	UnwrapKeyset(ctx context.Context, mountPath, keyName, wrappedKeyset string) ([]byte, error)
}

// CachedPrimitives holds the unwrapped AEAD and PRF primitives for an organization.
//
// PrimaryKeyID is the AEAD primary key ID (used for envelope marker formatting).
// PRFPrimaryKeyID is the PRF (search-token) keyset primary key ID, sourced from the
// stored HMACKeysetInfo.PrimaryKeyID, and is stamped on search tokens.
//
// LegacyHexTokenPRF is populated ONLY for migrated organizations whose persisted PRF
// keyset carries an imported legacy HMAC-SHA256 key; it computes the legacy-compatible
// hex-over-bare-value search token (byte-identical to the process-global indexed token).
// It is nil for envelope-only organizations. Consumed by legacy search-candidate
// generation (T-2.2.2).
type CachedPrimitives struct {
	AEAD              *tink.AEADPrimitive
	PRF               *tink.PRFPrimitive
	MultiKeyPRF       *tink.PRFMultiPrimitive
	LegacyHexTokenPRF *tink.LegacyPRFPrimitive
	PrimaryKeyID      uint32
	PRFPrimaryKeyID   uint32
	Version           uint32
	ExpiresAt         time.Time
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

	// MultiTenant enables per-tenant mount resolution for the legacy-record
	// fallback. When true, the tenant segment is appended and empty/"default"
	// tenants fail closed. When false (single-tenant), the flat base is used.
	MultiTenant bool
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
	baseMountPath string // Vault Transit base mount for per-tenant resolution
	multiTenant   bool
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
		multiTenant:   config.MultiTenant,
		cache:         make(map[string]*CachedPrimitives),
		fetching:      make(map[string]*sync.Mutex),
	}
}

// buildCacheKey constructs a tenant-scoped cache key prefix.
// Format: "tenantID:organizationID" to prevent cross-tenant cache collisions.
// Used by the protection-state resolver and as the prefix for version-scoped
// keyset cache invalidation.
func buildCacheKey(tenantID, organizationID string) string {
	return tenantID + ":" + organizationID
}

// buildVersionedCacheKey constructs a version-scoped keyset cache key.
// Format: "tenantID:organizationID:version" so each keyset version is cached
// independently and routed-by-version reads do not collide.
func buildVersionedCacheKey(tenantID, organizationID string, version int) string {
	return tenantID + ":" + organizationID + ":" + strconv.Itoa(version)
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

// GetActivePrimitives retrieves the cached AEAD, PRF, and PRFMulti primitives for an
// organization's ACTIVE (highest-version) keyset. It is CACHE-FIRST: the active
// entry is keyed on the non-versioned cache key (tenantID:organizationID), so a
// fresh cache hit serves without touching the repository. On a miss it loads the
// active keyset via repo.GetActive and, when no keyset exists and a provisioner is
// configured, auto-provisions version 1 before retrying (lazy provisioning).
//
// The returned CachedPrimitives carries:
//   - AEAD primitive for encryption/decryption
//   - PRF primitive for search token generation (primary key only)
//   - MultiKeyPRF primitive for search token generation across all keys
//   - PrimaryKeyID, the primary AEAD key ID
//   - PRFPrimaryKeyID, the primary PRF key ID for search-token versioning
//   - Version, the loaded keyset version, stamped onto envelope markers
//
// Uses the per-key mutex to deduplicate concurrent loads/unwraps for the same
// tenant-organization. Cache keys are scoped by tenant to prevent cross-tenant
// collisions.
func (km *KeysetManager) GetActivePrimitives(ctx context.Context, organizationID string) (*CachedPrimitives, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tenantID := ExtractTenantID(ctx)
	cacheKey := buildCacheKey(tenantID, organizationID)

	return km.getOrUnwrap(ctx, cacheKey, func(ctx context.Context) (*mmodel.OrganizationKeyset, error) {
		return km.loadActiveKeyset(ctx, organizationID)
	})
}

// GetPrimitivesForVersion retrieves the cached primitives for an organization's
// keyset at an EXACT version. It is CACHE-FIRST: the entry is keyed on the
// version-scoped cache key (tenantID:organizationID:version), so a fresh cache hit
// serves without touching the repository. On a miss it loads via repo.GetByVersion
// and does NOT auto-provision; a missing version surfaces as ErrKeysetNotFound.
// Used on the read/decrypt path where the marker selects the version to route to.
func (km *KeysetManager) GetPrimitivesForVersion(ctx context.Context, organizationID string, version int) (*CachedPrimitives, error) {
	// Check context before any work
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tenantID := ExtractTenantID(ctx)
	cacheKey := buildVersionedCacheKey(tenantID, organizationID, version)

	return km.getOrUnwrap(ctx, cacheKey, func(ctx context.Context) (*mmodel.OrganizationKeyset, error) {
		keyset, err := km.keysetRepo.GetByVersion(ctx, organizationID, version)
		if err != nil {
			return nil, err
		}

		if keyset == nil {
			return nil, constant.ErrKeysetNotFound
		}

		return keyset, nil
	})
}

// loadActiveKeyset fetches the organization's active (highest-version) keyset,
// auto-provisioning version 1 on first access when a provisioner is configured.
func (km *KeysetManager) loadActiveKeyset(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	keyset, err := km.keysetRepo.GetActive(ctx, organizationID)
	if err != nil {
		// If keyset not found and provisioner available, try to auto-provision.
		if km.provisioner != nil && (errors.Is(err, constant.ErrKeysetNotFound) || errors.Is(err, mmodel.ErrKeysetNotFound)) {
			if provErr := km.autoProvision(ctx, organizationID); provErr != nil {
				return nil, fmt.Errorf("auto-provision failed: %w", provErr)
			}

			// Retry fetch after provisioning.
			keyset, err = km.keysetRepo.GetActive(ctx, organizationID)
			if err != nil {
				return nil, fmt.Errorf("failed to get keyset after provisioning: %w", err)
			}
		} else {
			return nil, err
		}
	}

	// Guard against nil keyset (repository returned nil without error).
	if keyset == nil {
		return nil, constant.ErrKeysetNotFound
	}

	return keyset, nil
}

// getOrUnwrap returns cached primitives for cacheKey when present and fresh,
// otherwise loads the keyset via the load closure, unwraps it via KMS, caches, and
// returns. It is CACHE-FIRST: the cache is consulted before load runs, so a fresh
// hit serves from cache without any repository call. The per-key mutex (keyed on
// cacheKey) deduplicates concurrent loads/unwraps without blocking unrelated keys.
func (km *KeysetManager) getOrUnwrap(ctx context.Context, cacheKey string, load func(context.Context) (*mmodel.OrganizationKeyset, error)) (*CachedPrimitives, error) {
	// Fast path: check cache with read lock.
	km.mu.RLock()
	cached, ok := km.cache[cacheKey]
	km.mu.RUnlock()

	if ok && !cached.IsExpired() {
		km.metrics.recordCache(ctx, "get_primitives", "hit")

		return cached, nil
	}

	// Cache miss or expired - acquire per-key lock.
	orgLock := km.getOrgLock(cacheKey)
	orgLock.Lock()
	defer orgLock.Unlock()

	// Double-check cache after acquiring lock (another goroutine may have unwrapped).
	km.mu.RLock()
	cached, ok = km.cache[cacheKey]
	km.mu.RUnlock()

	if ok && !cached.IsExpired() {
		km.metrics.recordCache(ctx, "get_primitives", "hit")

		return cached, nil
	}

	km.metrics.recordCache(ctx, "get_primitives", "miss")

	keyset, err := load(ctx)
	if err != nil {
		return nil, err
	}

	return km.unwrapAndCache(ctx, cacheKey, keyset)
}

// unwrapAndCache unwraps the keyset via KMS and caches the primitives under cacheKey.
// Caller MUST hold the per-tenant-organization-version lock before calling.
func (km *KeysetManager) unwrapAndCache(ctx context.Context, cacheKey string, keyset *mmodel.OrganizationKeyset) (*CachedPrimitives, error) {
	// Check context before KMS unwrap
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Prefer the stored KEKMountPath so reads are config-independent; fall back to
	// the shared engine for legacy records that have no stored mount.
	mount := keyset.KEKMountPath
	if mount == "" {
		// Fail closed in multi-tenant mode when the stored tenant is missing or the
		// reserved sentinel: such a record has no tenant-scoped key and must not read
		// against the shared engine.
		if km.multiTenant {
			tenantID := strings.Trim(keyset.TenantID, "/ \t")
			if tenantID == "" || tenantID == defaultTenantID {
				return nil, fmt.Errorf("multi-tenant keyset requires a real tenant id, got %q", keyset.TenantID)
			}
		}

		mount = km.baseMountPath
	}

	// app.protection.mount_path is the resolved mount, not a secret.
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled // NewTrackingFromContext returns 4 values; only the tracer is needed here

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

	// Unwrap PRF (search-token) keyset using the same resolved mount. It is stored
	// in the WrappedHMACKeyset slot for storage-format compatibility. Provider
	// operation timing is recorded even on failure.
	prfStart := time.Now()
	prfBytes, err := km.unwrapper.UnwrapKeyset(ctx, mount, keyset.KEKPath, keyset.WrappedHMACKeyset)
	km.metrics.recordProviderOperation(ctx, providerOperationUnwrap, providerVault, time.Since(prfStart).Milliseconds())

	if err != nil {
		km.metrics.recordProviderFailure(ctx, providerOperationUnwrap, errorCodeUnwrapPRFFailed)
		libOpenTelemetry.HandleSpanError(span, "failed to unwrap PRF keyset", err)

		// Wrap with %w so callers can errors.Is(vault.ErrMountNotFound) (fail-closed).
		return nil, fmt.Errorf("failed to unwrap PRF keyset: %w", err)
	}

	// Build the PRF primitives (primary, multi-key, and the optional legacy hex-token
	// primitive for migrated keysets) from the unwrapped PRF keyset.
	prfSet, err := buildPRFPrimitives(prfBytes, keyset.HMACKeysetInfo)
	if err != nil {
		return nil, err
	}

	// Build cached primitives. PRFPrimaryKeyID is sourced from the stored
	// HMACKeysetInfo primary key ID (the search-token keyset metadata).
	cached := &CachedPrimitives{
		AEAD:              aeadPrimitive,
		PRF:               prfSet.prf,
		MultiKeyPRF:       prfSet.multiKey,
		LegacyHexTokenPRF: prfSet.legacyHexToken,
		PrimaryKeyID:      keyset.KeysetInfo.PrimaryKeyID,
		PRFPrimaryKeyID:   keyset.HMACKeysetInfo.PrimaryKeyID,
		Version:           uint32(keyset.Version), // #nosec G115 -- keyset version is a small positive monotonic counter
		ExpiresAt:         time.Now().Add(km.cacheTTL),
	}

	// Cache the primitives with write lock using the tenant+version-scoped key
	km.mu.Lock()
	km.cache[cacheKey] = cached
	km.mu.Unlock()

	return cached, nil
}

// prfPrimitiveSet groups the PRF primitives derived from a single unwrapped PRF
// keyset: the primary-key primitive, the multi-key primitive (all enabled keys),
// and the optional legacy hex-token primitive (nil for envelope-only keysets).
type prfPrimitiveSet struct {
	prf            *tink.PRFPrimitive
	multiKey       *tink.PRFMultiPrimitive
	legacyHexToken *tink.LegacyPRFPrimitive
}

// buildPRFPrimitives deserializes the unwrapped PRF keyset once and builds all PRF
// primitives from the shared handle. The legacy hex-token primitive is populated
// ONLY when the stored PRF metadata flags an imported legacy HMAC key; envelope-only
// keysets leave it nil. It fails closed (wrapped error) when the metadata flags a
// legacy key but the handle has no enabled legacy entry, surfacing a
// provisioning/decoding bug rather than silently degrading search.
func buildPRFPrimitives(prfBytes []byte, info mmodel.KeysetInfo) (prfPrimitiveSet, error) {
	prfHandle, err := tink.DeserializePRFKeyset(prfBytes)
	if err != nil {
		return prfPrimitiveSet{}, fmt.Errorf("failed to deserialize PRF keyset: %w", err)
	}

	prfPrimitive, err := tink.NewPRFPrimitive(prfHandle)
	if err != nil {
		return prfPrimitiveSet{}, fmt.Errorf("failed to create PRF primitive: %w", err)
	}

	// Strict mode: a failure to build the multi-key primitive fails the operation.
	multiKeyPRF, err := tink.NewPRFMultiPrimitive(prfHandle)
	if err != nil {
		return prfPrimitiveSet{}, fmt.Errorf("failed to create multi-key PRF primitive: %w", err)
	}

	set := prfPrimitiveSet{prf: prfPrimitive, multiKey: multiKeyPRF}

	if hmacKeysetHasLegacyKey(info) {
		set.legacyHexToken, err = tink.NewLegacyPRFPrimitiveFromHandle(prfHandle)
		if err != nil {
			return prfPrimitiveSet{}, fmt.Errorf("failed to derive legacy PRF primitive for migrated keyset: %w", err)
		}
	}

	return set, nil
}

// hmacKeysetHasLegacyKey reports whether the stored PRF (HMAC) keyset metadata
// carries an imported legacy key. It mirrors tink.KeysetInfo.HasLegacyKey over the
// persisted mmodel representation, reusing tink.KeyType.IsLegacy for the label test.
func hmacKeysetHasLegacyKey(info mmodel.KeysetInfo) bool {
	for _, key := range info.Keys {
		if tink.KeyType(key.Type).IsLegacy() {
			return true
		}
	}

	return false
}

// autoProvision provisions an organization using the injected provisioner. The tenant
// is resolved via ResolveProvisionTenantID (rejects reserved "default" before
// provisioning, fail-closed); an empty context maps to the single-tenant sentinel.
func (km *KeysetManager) autoProvision(ctx context.Context, organizationID string) error {
	if km.provisioner == nil {
		return fmt.Errorf("provisioner not configured")
	}

	tenantID, err := ResolveProvisionTenantID(ctx)
	if err != nil {
		return err
	}

	_, err = km.provisioner.Provision(ctx, ProvisionInput{
		TenantID:       tenantID,
		OrganizationID: organizationID,
		Actor:          "system:auto-provision",
		Reason:         "Lazy migration: imported legacy key material on first encrypted field access",
		// Lazy provisioning migrates an existing organization: it imports the
		// legacy key material (envelope PRIMARY + legacy ENABLED).
		importLegacy: true,
	})

	return err
}

// InvalidateCacheForTenant removes the cached primitives for the ACTIVE keyset and
// ALL keyset versions of a specific tenant-organization. The active entry is cached
// under the exact key "tenantID:organizationID" (no version), while version entries
// are cached under the "tenantID:organizationID:version" prefix; both are removed.
// Call this after key rotation or when a keyset is updated. Also removes the matching
// per-key mutexes to prevent unbounded map growth.
func (km *KeysetManager) InvalidateCacheForTenant(tenantID, organizationID string) {
	activeKey := buildCacheKey(tenantID, organizationID)
	prefix := activeKey + ":"

	km.mu.Lock()
	delete(km.cache, activeKey)

	for key := range km.cache {
		if strings.HasPrefix(key, prefix) {
			delete(km.cache, key)
		}
	}
	km.mu.Unlock()

	// Clean up per-key mutexes (active + all versions) to prevent unbounded growth.
	km.fetchMu.Lock()
	delete(km.fetching, activeKey)

	for key := range km.fetching {
		if strings.HasPrefix(key, prefix) {
			delete(km.fetching, key)
		}
	}
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
