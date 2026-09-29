// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"math"
	"strings"

	libAuth "github.com/LerianStudio/lib-auth/v5/auth/middleware"
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
	// m2m verifies producer access tokens on the HTTP routes; producers maps
	// token authorized parties and certificate URIs onto the platform roster.
	m2m       *libAuth.M2MAuthenticator
	keySource libAuth.KeySource
	// unverifiedProducers is true only when DEPLOYMENT_MODE=local disabled
	// token verification; every reservation is then attributed to the ledger.
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

// reservationSurfaceEnabled reports whether TRACER_PLATFORM_PRODUCERS names a
// producer roster, which is what enables the reservation surface. Without one
// the Tracer serves validations only.
func reservationSurfaceEnabled(cfg *Config) bool {
	return cfg != nil && strings.TrimSpace(cfg.TracerPlatformProducers) != ""
}

// loadContextReservationConfig validates the reservation runtime settings and
// builds the producer credential verifiers. It returns nil, nil when the
// reservation surface is disabled. On success the caller owns the returned
// config and must close it when the process stops.
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

	m2m, keySource, err := loadContextM2MAuthenticator(cfg, logger)
	if err != nil {
		return nil, err
	}

	unverifiedProducers := producerVerificationDisabled(cfg)
	if unverifiedProducers {
		logger.Log(context.Background(), libLog.LevelWarn,
			"Reservation producer authentication is disabled (DEPLOYMENT_MODE=local): every reservation is attributed to the ledger producer without a token")
	}

	facts := evaluation.CEL.Limits

	return &contextReservationConfig{
		evaluation: evaluation, m2m: m2m, keySource: keySource, unverifiedProducers: unverifiedProducers, producers: producers, maxBodyBytes: cfg.ContextReserveMaxBodyBytes,
		limits:    postgres.ContextLimitRepositoryConfig{MaxAccounts: facts.MaxAccounts, MaxLimits: cfg.ContextReserveMaxLimits, MaxScopes: cfg.ContextLimitMaxScopes, MaxScopeBytes: cfg.ContextLimitMaxScopeBytes},
		cache:     query.CompiledPolicyCacheConfig{MaxEntries: cfg.ContextPolicyCacheEntries, MaxCompilations: cfg.ContextPolicyMaxCompilations, SingleTenant: !cfg.MultiTenantEnabled},
		admission: command.ReserveAdmissionConfig{Plan: query.ContextReservationConfig{Facts: facts, MaxLimits: cfg.ContextReserveMaxLimits, MaxScopesPerLimit: cfg.ContextLimitMaxScopes, MaxReservations: cfg.ContextReserveMaxReservations}, MaxRules: cfg.ContextMaxRules, SingleTenant: !cfg.MultiTenantEnabled, MaxTimestampAge: model.MaxTimestampAge, ClockSkewTolerance: model.ClockSkewTolerance, ReservationLifetime: services.ReservationTTL, LongLivedLifetime: longLived},
	}, nil
}

// initContextReservation builds the reservation runtime, or returns nil when
// the reservation surface is disabled.
func initContextReservation(cfg *Config, conn pgdb.Connection, tx pgdb.TxBeginner, audit command.AuditEventRepository, capacity *postgres.UsageReservationRepository, clk clock.Clock, logger libLog.Logger) (runtime *contextReservationRuntime, err error) {
	config, err := loadContextReservationConfig(cfg, logger)
	if err != nil || config == nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			err = closeOnFailure(err, config)
		}
	}()

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
// mapping, and multi-tenancy needs verified producer tokens, because an
// unverified caller could otherwise name any tenant.
func loadPlatformProducers(cfg *Config) (*producerauth.Registry, error) {
	producers, err := producerauth.ParsePlatformProducers(cfg.TracerPlatformProducers)
	if err != nil {
		return nil, fmt.Errorf("invalid TRACER_PLATFORM_PRODUCERS: %w", err)
	}

	if strings.TrimSpace(cfg.TracerGRPCPort) != "" && !producers.HasCertificateMappings() {
		return nil, fmt.Errorf("TRACER_GRPC_PORT requires a certUri in TRACER_PLATFORM_PRODUCERS: the gRPC reservation seam identifies producers only by client certificate")
	}

	if cfg.MultiTenantEnabled && producerVerificationDisabled(cfg) {
		return nil, fmt.Errorf("MULTI_TENANT_ENABLED=true requires producer token verification: DEPLOYMENT_MODE=local attributes every reservation to the ledger without a token, so any caller could name any tenant")
	}

	return producers, nil
}

// loadContextM2MAuthenticator builds the verifier for producer access tokens.
// Verification is disabled only when DEPLOYMENT_MODE is explicitly local, and
// then no key source is started and neither CONTEXT_M2M_JWKS_URL nor
// CONTEXT_M2M_ISSUER is read; an unset or any other mode verifies tokens and
// requires both.
func loadContextM2MAuthenticator(cfg *Config, logger libLog.Logger) (*libAuth.M2MAuthenticator, libAuth.KeySource, error) {
	issuer := strings.TrimSpace(cfg.ContextM2MIssuer)

	if producerVerificationDisabled(cfg) {
		m2m, err := libAuth.NewM2MAuthenticatorWithKeySource(nil, issuer, false, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("build reservation producer token verifier: %w", err)
		}

		return m2m, nil, nil
	}

	jwksURL := strings.TrimSpace(cfg.ContextM2MJWKSURL)
	if jwksURL == "" {
		return nil, nil, fmt.Errorf("CONTEXT_M2M_JWKS_URL is required to verify reservation producer tokens unless DEPLOYMENT_MODE=local")
	}

	if issuer == "" {
		return nil, nil, fmt.Errorf("CONTEXT_M2M_ISSUER is required unless DEPLOYMENT_MODE=local")
	}

	source, err := libAuth.NewJWKSKeySource(libAuth.JWKSConfig{URL: jwksURL, Logger: logger})
	if err != nil {
		return nil, nil, fmt.Errorf("invalid CONTEXT_M2M_JWKS_URL: %w", err)
	}

	m2m, err := libAuth.NewM2MAuthenticatorWithKeySource(source, issuer, true, logger)
	if err != nil {
		return nil, nil, closeKeySourceOnFailure(fmt.Errorf("build reservation producer token verifier: %w", err), source)
	}

	return m2m, source, nil
}

// producerVerificationDisabled reports whether the deployment explicitly runs
// in local mode, the only mode that may skip producer token verification. An
// unset DEPLOYMENT_MODE does not qualify.
func producerVerificationDisabled(cfg *Config) bool {
	return cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.DeploymentMode), "local")
}

// close stops the JWKS refresher, when one was started.
func (c *contextReservationConfig) close() error {
	if c == nil || c.keySource == nil {
		return nil
	}

	return c.keySource.Close()
}

func closeOnFailure(cause error, config *contextReservationConfig) error {
	if closeErr := config.close(); closeErr != nil {
		return fmt.Errorf("%w (closing JWKS key source: %w)", cause, closeErr)
	}

	return cause
}

func closeKeySourceOnFailure(cause error, source libAuth.KeySource) error {
	if closeErr := source.Close(); closeErr != nil {
		return fmt.Errorf("%w (closing JWKS key source: %w)", cause, closeErr)
	}

	return cause
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
