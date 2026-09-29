// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"math"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	in "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

type contextReservationConfig struct {
	evaluation *contextPolicyConfig
	// unverifiedProducers is true only when plugin auth is disabled under an
	// explicit DEPLOYMENT_MODE=local: no caller is verified, and every HTTP
	// reservation is attributed to the ledger. producers maps token authorized
	// parties and certificate URIs onto the platform roster; under
	// multi-tenancy it maps certificate URIs only, and is nil when
	// TRACER_PLATFORM_PRODUCERS is unset.
	unverifiedProducers bool
	producers           *producerauth.Registry
	limits              postgres.ContextLimitRepositoryConfig
	admission           command.ReserveAdmissionConfig
	cache               query.CompiledPolicyCacheConfig
	maxBodyBytes        int
}

type contextReservationRuntime struct {
	handler               *in.ContextReservationHandler
	admission             *command.ReserveAdmissionCommand
	completion            *command.CompleteReserveOperationCommand
	reservationCompletion *command.CompleteReserveReservationCommand
	config                *contextReservationConfig
}

// reservationSurfaceEnabled reports whether the reservation surface is
// mounted. Under multi-tenancy it always is, and the tenant-manager decides
// per tenant: a tenant takes part only while it holds an active ledger
// association. In single-tenant mode TRACER_PLATFORM_PRODUCERS is the switch:
// without a producer roster the Tracer serves validations only.
func reservationSurfaceEnabled(cfg *Config) bool {
	return cfg != nil && (cfg.MultiTenantEnabled || strings.TrimSpace(cfg.TracerPlatformProducers) != "")
}

// loadContextReservationConfig validates the reservation runtime settings. It
// returns nil, nil when the reservation surface is disabled.
func loadContextReservationConfig(cfg *Config, logger libLog.Logger) (*contextReservationConfig, error) {
	if !reservationSurfaceEnabled(cfg) {
		if strings.TrimSpace(cfg.TracerGRPCPort) != "" {
			return nil, fmt.Errorf("TRACER_GRPC_PORT requires TRACER_PLATFORM_PRODUCERS: the gRPC listener serves only the reservation contract")
		}

		logger.Log(context.Background(), libLog.LevelInfo, "reservation integration disabled (TRACER_PLATFORM_PRODUCERS empty)")

		return nil, nil
	}

	if strings.TrimSpace(cfg.TracerGRPCPort) != "" && !strings.EqualFold(strings.TrimSpace(cfg.TracerTLSMode), tlsModeMTLS) {
		return nil, fmt.Errorf("TRACER_GRPC_PORT requires TRACER_TLS_MODE=mtls: the gRPC reservation seam identifies producers only by client certificate")
	}

	evaluation, err := loadContextEvaluationConfig(cfg)
	if err != nil {
		return nil, err
	}

	producers, err := loadPlatformProducers(cfg)
	if err != nil {
		return nil, err
	}

	if cfg.ContextReserveMaxBodyBytes <= 0 || cfg.ContextReserveMaxLimits <= 0 || cfg.ContextReserveMaxReservations <= 0 || cfg.ContextReserveMaxReservations > math.MaxInt32 || cfg.ContextPolicyCacheEntries <= 0 || cfg.ContextPolicyMaxCompilations <= 0 || cfg.ContextLimitMaxScopes <= 0 || cfg.ContextLimitMaxScopeBytes <= 0 {
		return nil, fmt.Errorf("context reservation body, limits, scopes, cache and compilation bounds must be positive")
	}

	longLived, err := parseReservationLongLivedTTLHours(cfg.ReservationLongLivedTTLHours)
	if err != nil {
		return nil, err
	}

	unverifiedProducers := producerVerificationDisabled(cfg)
	if unverifiedProducers {
		if !explicitLocalMode(cfg) {
			return nil, fmt.Errorf("reservations require PLUGIN_AUTH_ENABLED=true unless DEPLOYMENT_MODE=local: without the Access Manager no reservation caller is verified")
		}

		logger.Log(context.Background(), libLog.LevelWarn,
			"Reservation producer authentication is disabled (PLUGIN_AUTH_ENABLED=false, DEPLOYMENT_MODE=local): every HTTP reservation is attributed to the ledger producer")
	}

	facts := evaluation.CEL.Limits

	return &contextReservationConfig{
		evaluation: evaluation, unverifiedProducers: unverifiedProducers, producers: producers, maxBodyBytes: cfg.ContextReserveMaxBodyBytes,
		limits:    postgres.ContextLimitRepositoryConfig{MaxAccounts: facts.MaxAccounts, MaxLimits: cfg.ContextReserveMaxLimits, MaxScopes: cfg.ContextLimitMaxScopes, MaxScopeBytes: cfg.ContextLimitMaxScopeBytes},
		cache:     query.CompiledPolicyCacheConfig{MaxEntries: cfg.ContextPolicyCacheEntries, MaxCompilations: cfg.ContextPolicyMaxCompilations, SingleTenant: !cfg.MultiTenantEnabled},
		admission: command.ReserveAdmissionConfig{Plan: query.ContextReservationConfig{Facts: facts, MaxLimits: cfg.ContextReserveMaxLimits, MaxScopesPerLimit: cfg.ContextLimitMaxScopes, MaxReservations: cfg.ContextReserveMaxReservations}, MaxRules: cfg.ContextMaxRules, SingleTenant: !cfg.MultiTenantEnabled, MaxTimestampAge: model.MaxTimestampAge, ClockSkewTolerance: model.ClockSkewTolerance, ReservationLifetime: services.ReservationTTL, LongLivedLifetime: longLived},
	}, nil
}

// initContextReservation builds the reservation runtime, or returns nil when
// the reservation surface is disabled.
func initContextReservation(cfg *Config, conn pgdb.Connection, tx pgdb.TxBeginner, audit command.AuditEventRepository, capacity *postgres.UsageReservationRepository, clk clock.Clock, logger libLog.Logger) (*contextReservationRuntime, error) {
	config, err := loadContextReservationConfig(cfg, logger)
	if err != nil || config == nil {
		return nil, err
	}

	engine, err := cel.NewContextAdapter(config.evaluation.CEL)
	if err != nil {
		return nil, err
	}

	evaluator, err := query.NewContextPolicyEvaluator(engine, config.evaluation.Policy)
	if err != nil {
		return nil, err
	}

	policies, err := postgres.NewContextPolicyRepository(conn, config.evaluation.Policy.MaxRules)
	if err != nil {
		return nil, err
	}

	resolver, err := query.NewResolveContextPolicyQuery(policies, config.evaluation.Policy.MaxRules)
	if err != nil {
		return nil, err
	}

	compiled, err := query.NewCompiledContextPolicyQuery(resolver, evaluator, config.cache)
	if err != nil {
		return nil, err
	}

	decisions, err := postgres.NewReserveDecisionRepository(conn, config.admission.MaxRules, config.admission.Plan.MaxReservations)
	if err != nil {
		return nil, err
	}

	limits, err := postgres.NewContextLimitRepository(config.limits)
	if err != nil {
		return nil, err
	}

	operations := postgres.NewReserveOperationRepository()

	admission, err := command.NewReserveAdmissionCommand(command.ReserveAdmissionDependencies{Decisions: decisions, Operations: operations, Capacity: capacity, Limits: limits, Policies: compiled, Evaluator: evaluator, Audit: audit, Transactions: tx}, clk, config.admission)
	if err != nil {
		return nil, err
	}

	completion, err := command.NewCompleteReserveOperationCommand(operations, decisions, capacity, audit, tx, clk, command.ReserveCompletionConfig{SingleTenant: config.admission.SingleTenant, MaxRules: config.admission.MaxRules, MaxReservations: config.admission.Plan.MaxReservations})
	if err != nil {
		return nil, err
	}

	reservationCompletion, err := command.NewCompleteReserveReservationCommand(decisions, completion, !cfg.MultiTenantEnabled)
	if err != nil {
		return nil, err
	}

	handler, err := in.NewContextReservationHandler(admission, completion, reservationCompletion, config.admission.Plan.Facts, config.maxBodyBytes, config.admission.Plan.MaxReservations)
	if err != nil {
		return nil, err
	}

	return &contextReservationRuntime{handler: handler, admission: admission, completion: completion, reservationCompletion: reservationCompletion, config: config}, nil
}

// loadPlatformProducers parses the producer roster and checks it against the
// transports and tenancy it must serve: a gRPC listener needs a certificate
// mapping, and multi-tenancy needs a verified caller, because an unverified
// one could otherwise name any tenant. Multi-tenant HTTP reservations
// identify the producer by the platform claims the tenant-manager writes onto
// its token, so under multi-tenancy the roster is optional, serves gRPC only,
// and refuses a clientId it would never consult.
func loadPlatformProducers(cfg *Config) (*producerauth.Registry, error) {
	if cfg.MultiTenantEnabled && producerVerificationDisabled(cfg) {
		return nil, fmt.Errorf("MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true for reservations: without the Access Manager every reservation is attributed to the ledger, so any caller could name any tenant")
	}

	grpcEnabled := strings.TrimSpace(cfg.TracerGRPCPort) != ""

	if cfg.MultiTenantEnabled && strings.TrimSpace(cfg.TracerPlatformProducers) == "" {
		if grpcEnabled {
			return nil, fmt.Errorf("TRACER_GRPC_PORT requires a certUri in TRACER_PLATFORM_PRODUCERS: the gRPC reservation seam identifies producers only by client certificate")
		}

		return nil, nil
	}

	producers, err := producerauth.ParsePlatformProducers(cfg.TracerPlatformProducers)
	if err != nil {
		return nil, fmt.Errorf("invalid TRACER_PLATFORM_PRODUCERS: %w", err)
	}

	if cfg.MultiTenantEnabled && producers.HasClientIDMappings() {
		return nil, fmt.Errorf("MULTI_TENANT_ENABLED=true refuses clientId entries in TRACER_PLATFORM_PRODUCERS: multi-tenant HTTP reservations identify the producer by the platform claims the tenant-manager writes onto its token, never by a configured client id; keep only certUri entries, which serve gRPC")
	}

	if grpcEnabled && !producers.HasCertificateMappings() {
		return nil, fmt.Errorf("TRACER_GRPC_PORT requires a certUri in TRACER_PLATFORM_PRODUCERS: the gRPC reservation seam identifies producers only by client certificate")
	}

	return producers, nil
}

// producerVerificationDisabled reports whether the HTTP reservation routes
// run without a verified caller: with plugin auth disabled the route guard
// authorizes nothing.
func producerVerificationDisabled(cfg *Config) bool {
	return cfg != nil && !cfg.PluginAuthEnabled
}

// explicitLocalMode reports whether DEPLOYMENT_MODE is explicitly local, the
// only mode that may serve reservations without a verified caller. An unset
// DEPLOYMENT_MODE does not qualify.
func explicitLocalMode(cfg *Config) bool {
	return cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.DeploymentMode), "local")
}

// initReserveOperationExpiry builds the reaper's expiry of decision-owned
// operations whether or not the reservation surface is enabled: capacity held
// by an earlier decision must still return when its TTL elapses.
func initReserveOperationExpiry(cfg *Config, conn pgdb.Connection, tx pgdb.TxBeginner, audit command.AuditEventRepository, capacity *postgres.UsageReservationRepository) (workers.ReserveOperationExpirer, error) {
	decisions, err := postgres.NewReserveDecisionRepository(conn, cfg.ContextMaxRules, cfg.ContextReserveMaxReservations)
	if err != nil {
		return nil, err
	}

	expiry, err := command.NewExpireReserveOperationCommand(postgres.NewReserveOperationRepository(), decisions, capacity, audit, tx,
		command.ReserveCompletionConfig{SingleTenant: !cfg.MultiTenantEnabled, MaxRules: cfg.ContextMaxRules, MaxReservations: cfg.ContextReserveMaxReservations})
	if err != nil {
		return nil, err
	}

	return expiry, nil
}
