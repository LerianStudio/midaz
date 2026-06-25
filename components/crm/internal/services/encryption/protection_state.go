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

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// statusNone is the protection status label used for legacy resolutions where no
// registry record participates: nil registry repository (KMS_VENDOR=none),
// registry-not-found, and nil-record. These organizations have no registry
// status, so "none" is emitted rather than a record status string.
const statusNone = "none"

// protectionStateCacheTTL bounds how long a resolved protection state is reused
// before the registry is consulted again. It mirrors the KeysetManager cache TTL
// so a multi-field alias search resolves the registry once per window instead of
// once per field. Successful provisioning invalidates the affected entry so
// legacy -> envelope transitions become visible immediately.
const protectionStateCacheTTL = 5 * time.Minute

// cachedProtectionState is a resolved ProtectionState with the status label it was
// resolved with (so the same mode/status telemetry can be replayed on a cache hit)
// and its expiry.
type cachedProtectionState struct {
	state       ProtectionState
	statusLabel string
	expiresAt   time.Time
}

func (c cachedProtectionState) isExpired() bool {
	return c.expiresAt.IsZero() || time.Now().After(c.expiresAt)
}

// ProtectionState contains the resolved encryption state for an organization.
type ProtectionState struct {
	// Mode indicates whether to use legacy or envelope encryption for new writes.
	Mode crypto.EncryptionMode
	// CanReadLegacy indicates whether legacy-encrypted data can be decrypted.
	CanReadLegacy bool
	// CurrentKeysetVersion is the keyset version for new encryptions (0 for legacy mode).
	CurrentKeysetVersion int
	// ReadableVersions lists the keyset versions whose marked ciphertext may be
	// decrypted. Decrypt fails closed when a marker's version is not in this set.
	// Empty/nil for legacy or unprovisioned organizations.
	ReadableVersions []int
	// OrganizationID is the resolved organization.
	OrganizationID string
	// TenantID is the resolved tenant (from registry record).
	TenantID string
}

// MustUseEnvelope returns true if the organization MUST use envelope encryption for new writes.
func (ps ProtectionState) MustUseEnvelope() bool {
	return ps.Mode == crypto.EncryptionModeEnvelope
}

// ProtectionStateResolver determines the encryption mode for an organization
// based on its registry state.
type ProtectionStateResolver struct {
	registryRepo mongoEncryption.RegistryRepository
	metrics      *protectionMetrics

	// Short-TTL cache keyed by "tenantID:organizationID" to avoid a live registry
	// FindOne on every field of a multi-field resolve. Mirrors the KeysetManager
	// cache pattern (mutex-guarded map, protectionStateCacheTTL window).
	mu    sync.RWMutex
	cache map[string]cachedProtectionState
}

// NewProtectionStateResolver creates a new resolver with the given registry repository.
// metrics is the nil-safe protection metrics seam; a nil value defaults to
// NewProtectionMetrics(nil) so emission is a no-op when telemetry is disabled.
func NewProtectionStateResolver(registryRepo mongoEncryption.RegistryRepository, metrics *protectionMetrics) *ProtectionStateResolver {
	if metrics == nil {
		metrics = NewProtectionMetrics(nil)
	}

	return &ProtectionStateResolver{
		registryRepo: registryRepo,
		metrics:      metrics,
		cache:        make(map[string]cachedProtectionState),
	}
}

// Resolve determines the protection state for an organization.
//
// Returns ProtectionState with Mode=Legacy if:
//   - Registry record not found (organization not provisioned)
//
// Returns ProtectionState with Mode=Envelope if:
//   - Registry record exists with "active" status
//
// Returns error if:
//   - Repository returns an unexpected error
//   - Reader is nil
func (r *ProtectionStateResolver) Resolve(ctx context.Context, organizationID string) (ProtectionState, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.protection.resolve_mode")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.organization_id", organizationID))

	cacheKey := buildCacheKey(ExtractTenantID(ctx), organizationID)

	// Fast path: replay a recently resolved state without touching the registry.
	// The same mode/status telemetry is re-emitted so observability is unchanged.
	if cached, ok := r.cacheGet(cacheKey); ok {
		span.SetAttributes(attribute.String("app.protection.resolved_mode", cached.state.Mode.String()))
		r.metrics.recordModeResolution(ctx, cached.state.Mode.String())
		r.metrics.recordStatus(ctx, cached.statusLabel)

		return cached.state, nil
	}

	// Nil registry repository indicates KMS_VENDOR=none (legacy-only mode).
	// Return legacy readable state to allow legacy encryption without envelope.
	if r.registryRepo == nil {
		return r.legacyState(ctx, span, cacheKey, organizationID), nil
	}

	record, err := r.registryRepo.Get(ctx, organizationID)
	if err != nil {
		if errors.Is(err, constant.ErrRegistryNotFound) {
			// Organization hasn't been provisioned for envelope encryption yet.
			// Default to legacy mode with legacy readable.
			return r.legacyState(ctx, span, cacheKey, organizationID), nil
		}

		// Unexpected repository error is not a resolved outcome: propagate
		// unchanged without emitting mode/status metrics.
		return ProtectionState{}, err
	}

	return r.resolveFromRecord(ctx, logger, span, cacheKey, record)
}

// cacheGet returns a non-expired cached protection state for the key, if present.
func (r *ProtectionStateResolver) cacheGet(cacheKey string) (cachedProtectionState, bool) {
	r.mu.RLock()
	cached, ok := r.cache[cacheKey]
	r.mu.RUnlock()

	if !ok || cached.isExpired() {
		return cachedProtectionState{}, false
	}

	return cached, true
}

// cacheStore records a resolved protection state with its status label under the key.
func (r *ProtectionStateResolver) cacheStore(cacheKey, statusLabel string, state ProtectionState) {
	r.mu.Lock()
	r.cache[cacheKey] = cachedProtectionState{
		state:       state,
		statusLabel: statusLabel,
		expiresAt:   time.Now().Add(protectionStateCacheTTL),
	}
	r.mu.Unlock()
}

// Invalidate removes a cached protection state for a tenant and organization.
func (r *ProtectionStateResolver) Invalidate(tenantID, organizationID string) {
	cacheKey := buildCacheKey(tenantID, organizationID)

	r.mu.Lock()
	delete(r.cache, cacheKey)
	r.mu.Unlock()
}

// legacyState builds the legacy ProtectionState and emits the resolved-outcome
// telemetry for the legacy branches that carry no registry record (nil repo,
// not-found, nil-record): resolved_mode=legacy, mode metric=legacy, status=none.
func (r *ProtectionStateResolver) legacyState(ctx context.Context, span trace.Span, cacheKey, organizationID string) ProtectionState {
	mode := crypto.EncryptionModeLegacy

	span.SetAttributes(attribute.String("app.protection.resolved_mode", mode.String()))
	r.metrics.recordModeResolution(ctx, mode.String())
	r.metrics.recordStatus(ctx, statusNone)

	state := ProtectionState{
		Mode:                 mode,
		CanReadLegacy:        true,
		CurrentKeysetVersion: 0,
		OrganizationID:       organizationID,
		TenantID:             "",
	}

	r.cacheStore(cacheKey, statusNone, state)

	return state
}

// resolveFromRecord maps a registry record to a ProtectionState.
// Returns legacy state if record is nil (organization not provisioned).
func (r *ProtectionStateResolver) resolveFromRecord(ctx context.Context, logger libLog.Logger, span trace.Span, cacheKey string, record *mmodel.OrganizationRegistryRecord) (ProtectionState, error) {
	// Guard against nil record (repository returned nil without error).
	// No registry record participates, so status=none (see statusNone).
	if record == nil {
		return r.legacyState(ctx, span, cacheKey, ""), nil
	}

	// Registry record exists → organization is provisioned for envelope encryption
	if record.Status == mmodel.RegistryStatusActive {
		mode := crypto.EncryptionModeEnvelope

		span.SetAttributes(attribute.String("app.protection.resolved_mode", mode.String()))
		r.metrics.recordModeResolution(ctx, mode.String())
		// Active path emits the registry record status string ("active").
		r.metrics.recordStatus(ctx, string(record.Status))

		state := ProtectionState{
			Mode:                 mode,
			CanReadLegacy:        record.LegacyReadable,
			CurrentKeysetVersion: record.CurrentVersion,
			ReadableVersions:     record.ReadableVersions,
			OrganizationID:       record.OrganizationID,
			TenantID:             record.TenantID,
		}

		r.cacheStore(cacheKey, string(record.Status), state)

		return state, nil
	}

	// Unknown status - treat as error to avoid silent misconfiguration.
	// Record the span error and a status=unknown counter, but return the
	// ORIGINAL error unchanged.
	err := fmt.Errorf("unknown registry status: %s", record.Status)
	libOpenTelemetry.HandleSpanError(span, "unknown registry status", err)
	r.metrics.recordStatus(ctx, "unknown")

	// Operator-actionable misconfiguration: warn with the failing status only
	// (never PII/values). Org id is already on the span.
	logger.Log(ctx, libLog.LevelWarn, "unknown registry status", libLog.String("registry_status", string(record.Status)))

	return ProtectionState{}, err
}
