// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

// PackageCache is the narrow cache port the fee use case needs to memoize the
// per-(org,ledger) enabled-package set across transaction creates. It is a
// structural subset of the transaction RedisRepository, so the
// already-constructed transaction Redis repo satisfies it without a second
// client. A nil PackageCache on the UseCase disables caching (the bytes are
// always fetched from Mongo), exactly like a nil MetricsFactory disables metrics.
type PackageCache interface {
	// GetBytes returns the cached value for key, or (nil/empty, redis.Nil) on miss.
	GetBytes(ctx context.Context, key string) ([]byte, error)
	// SetBytes stores value under key with the given TTL. A zero TTL is persistent.
	SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Del removes key from the cache.
	Del(ctx context.Context, key string) error
}

// packageCacheNotFoundSentinel marks an (org,ledger) pair known to have zero
// enabled fee packages. Caching the absence stops the common zero-package tenant
// from hitting Mongo on every transaction create (negative caching).
var packageCacheNotFoundSentinel = []byte("NOT_FOUND")

// packageCacheSentinelTTL bounds how long a zero-package result is trusted before
// being re-verified against Mongo. SetBytes multiplies this by time.Second
// internally (mirroring the transaction-route cache), so 60 means 60 seconds.
const packageCacheSentinelTTL = time.Duration(60)

// packageCacheKey builds the Redis key for an (org,ledger) enabled-package set.
// Format: "fee_packages:{organizationID:ledgerID}" — the {…} hash-tag groups the
// two IDs onto one Redis Cluster slot, matching the idempotency key convention.
func packageCacheKey(organizationID, ledgerID uuid.UUID) string {
	var builder strings.Builder

	builder.Grow(88) // "fee_packages:{" + 2×UUID + ":" + "}"

	builder.WriteString("fee_packages")
	builder.WriteString(":")
	builder.WriteString("{")
	builder.WriteString(organizationID.String())
	builder.WriteString(":")
	builder.WriteString(ledgerID.String())
	builder.WriteString("}")

	return builder.String()
}

// UseCase is a struct to implement the services methods.
// Fields are private to enforce construction through NewUseCase,
// which validates that all required dependencies are provided.
type UseCase struct {
	// packageRepo provides an abstraction on top of the pack data source.
	packageRepo pack.Repository

	// resolver resolves account/transaction reads in-process via the ledger query layer.
	resolver feeshared.MidazResolver

	// defaultCurrency is the default currency for fee calculations
	defaultCurrency string

	// MetricsFactory emits the bounded domain_operations_total /
	// domain_operation_duration_ms metrics for every state-mutating fee
	// entrypoint via utils.RecordDomainOperation. Assigned at bootstrap; a nil
	// value is a no-op so the binary runs with telemetry disabled.
	MetricsFactory *metrics.MetricsFactory

	// PackageCache caches the per-(org,ledger) enabled-package set so a
	// non-skipped transaction create does not hit Mongo on every call (and the
	// common zero-package tenant is served from a NOT_FOUND sentinel). Assigned
	// at bootstrap from the shared transaction Redis repo; a nil value disables
	// caching and every CalculateFee fetches from Mongo. Invalidated on
	// create/update/delete of a package in the affected (org,ledger).
	PackageCache PackageCache

	// Streaming emits past-tense fee domain events; nil disables event emission.
	Streaming libStreaming.Emitter
}

// ErrNilPackageRepo is returned when a nil PackageRepo is provided to NewUseCase.
var ErrNilPackageRepo = errors.New("PackageRepo is required and cannot be nil")

// ErrNilResolver is returned when a nil MidazResolver is provided to NewUseCase.
var ErrNilResolver = errors.New("MidazResolver is required and cannot be nil")

// ErrEmptyDefaultCurrency is returned when an empty DefaultCurrency is provided to NewUseCase.
var ErrEmptyDefaultCurrency = errors.New("DefaultCurrency is required and cannot be empty")

// NewUseCase creates a new UseCase with validated dependencies.
// Returns an error if any required dependency is nil or empty.
func NewUseCase(packageRepo pack.Repository, resolver feeshared.MidazResolver, defaultCurrency string) (*UseCase, error) {
	if packageRepo == nil {
		return nil, ErrNilPackageRepo
	}

	if resolver == nil {
		return nil, ErrNilResolver
	}

	if defaultCurrency == "" {
		return nil, ErrEmptyDefaultCurrency
	}

	return &UseCase{
		packageRepo:     packageRepo,
		resolver:        resolver,
		defaultCurrency: defaultCurrency,
	}, nil
}

// PackageRepo returns the package repository dependency.
func (uc *UseCase) PackageRepo() pack.Repository {
	return uc.packageRepo
}

// Resolver returns the in-process Midaz resolver dependency.
func (uc *UseCase) Resolver() feeshared.MidazResolver {
	return uc.resolver
}

// DefaultCurrency returns the default currency for fee calculations.
func (uc *UseCase) DefaultCurrency() string {
	return uc.defaultCurrency
}

// findPackagesCached returns the enabled fee packages for (org,ledger), serving
// them from PackageCache when present and falling back to Mongo on a miss. It is
// the cache-aside front for packageRepo.FindByOrganizationIDAndLedgerID:
//
//   - A NOT_FOUND sentinel hit returns an empty slice with no Mongo round-trip,
//     so the common zero-package tenant is never charged a query per create.
//   - A populated hit decodes the cached JSON. Corrupted cache data falls back to
//     Mongo rather than failing the request.
//   - On a miss, Mongo is queried and the result re-cached: a non-empty set is
//     stored persistently (TTL 0, invalidated on package mutation), an empty set
//     as the bounded NOT_FOUND sentinel.
//
// Cache read/write failures NEVER fail the request: they are logged at Warn,
// span-recorded, and the path degrades to a direct Mongo fetch. A nil
// PackageCache disables caching entirely.
func (uc *UseCase) findPackagesCached(
	ctx context.Context,
	logger libLog.Logger,
	span trace.Span,
	organizationID, ledgerID uuid.UUID,
) ([]*pack.Package, error) {
	if uc.PackageCache == nil {
		return uc.packageRepo.FindByOrganizationIDAndLedgerID(ctx, organizationID, ledgerID)
	}

	key := packageCacheKey(organizationID, ledgerID)

	cached, err := uc.PackageCache.GetBytes(ctx, key)
	if err != nil && !errors.Is(err, redis.Nil) {
		logger.Log(ctx, libLog.LevelWarn, "Failed to read fee package cache, falling back to Mongo", libLog.Err(err))
	}

	if err == nil && len(cached) > 0 {
		if bytes.Equal(cached, packageCacheNotFoundSentinel) {
			return []*pack.Package{}, nil
		}

		var packages []*pack.Package
		if decodeErr := json.Unmarshal(cached, &packages); decodeErr != nil {
			libOpentelemetry.HandleSpanError(span, "Corrupted fee package cache, falling back to Mongo", decodeErr)
			logger.Log(ctx, libLog.LevelWarn, "Corrupted fee package cache, falling back to Mongo", libLog.Err(decodeErr))
		} else {
			return packages, nil
		}
	}

	packages, err := uc.packageRepo.FindByOrganizationIDAndLedgerID(ctx, organizationID, ledgerID)
	if err != nil {
		return nil, err
	}

	uc.writePackageCache(ctx, logger, key, packages)

	return packages, nil
}

// writePackageCache stores the freshly-fetched package set: a non-empty set as
// persistent JSON (invalidated on mutation), an empty set as the bounded
// NOT_FOUND sentinel. Write failures are logged at Warn and swallowed — the
// cache is an optimization, never a correctness dependency.
func (uc *UseCase) writePackageCache(ctx context.Context, logger libLog.Logger, key string, packages []*pack.Package) {
	if len(packages) == 0 {
		if setErr := uc.PackageCache.SetBytes(ctx, key, packageCacheNotFoundSentinel, packageCacheSentinelTTL); setErr != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to store fee package not-found sentinel", libLog.Err(setErr))
		}

		return
	}

	encoded, encErr := json.Marshal(packages)
	if encErr != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to encode fee package cache", libLog.Err(encErr))

		return
	}

	if setErr := uc.PackageCache.SetBytes(ctx, key, encoded, 0); setErr != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to store fee package cache", libLog.Err(setErr))
	}
}

// invalidatePackageCache removes the cached package set for (org,ledger) after a
// package mutation. A nil PackageCache or a Del failure is logged at Warn and
// otherwise ignored: a stale entry self-heals at the sentinel TTL, and the
// mutation itself has already committed to Mongo.
func (uc *UseCase) invalidatePackageCache(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID) {
	if uc.PackageCache == nil {
		return
	}

	if err := uc.PackageCache.Del(ctx, packageCacheKey(organizationID, ledgerID)); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to invalidate fee package cache", libLog.Err(err))
	}
}

// segmentIDToString maps an optional segment UUID to an optional string.
func segmentIDToString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}

	s := id.String()

	return &s
}

// enableOrFalse dereferences an optional enable flag, defaulting to false.
func enableOrFalse(enable *bool) bool {
	return enable != nil && *enable
}
