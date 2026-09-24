// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"math"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	in "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

type contextReservationConfig struct {
	evaluation   *contextPolicyConfig
	identity     *seamidentity.Resolver
	limits       postgres.ContextLimitRepositoryConfig
	admission    command.ReserveAdmissionConfig
	cache        query.CompiledPolicyCacheConfig
	maxBodyBytes int
}

type contextReservationRuntime struct {
	handler               *in.ContextReservationHandler
	identity              *seamidentity.Resolver
	admission             *command.ReserveAdmissionCommand
	completion            *command.CompleteReserveOperationCommand
	reservationCompletion *command.CompleteReserveReservationCommand
	config                *contextReservationConfig
}

func loadContextReservationConfig(cfg *Config) (*contextReservationConfig, error) {
	if !cfg.ContextReserveEnabled {
		return nil, nil
	}

	if cfg.TracerTLSMode != "mtls" {
		return nil, fmt.Errorf("CONTEXT_RESERVE_ENABLED requires native TRACER_TLS_MODE=mtls")
	}

	evaluation, err := loadContextEvaluationConfig(cfg)
	if err != nil {
		return nil, err
	}

	identity, err := loadContextProducerIdentity(cfg, evaluation.CEL.Limits)
	if err != nil {
		return nil, err
	}

	if cfg.ContextReserveMaxBodyBytes <= 0 || cfg.ContextReserveMaxLimits <= 0 || cfg.ContextReserveMaxReservations <= 0 || cfg.ContextReserveMaxReservations > math.MaxInt32 || cfg.ContextPolicyCacheEntries <= 0 || cfg.ContextPolicyMaxCompilations <= 0 || cfg.ContextLimitMaxScopes <= 0 || cfg.ContextLimitMaxScopeBytes <= 0 {
		return nil, fmt.Errorf("context reservation body, limits, scopes, cache and compilation bounds must be positive")
	}

	lifetime, err := parseReservationLongLivedTTLHours(cfg.ReservationLongLivedTTLHours)
	if err != nil {
		return nil, err
	}

	facts := evaluation.CEL.Limits

	return &contextReservationConfig{
		evaluation: evaluation, identity: identity, maxBodyBytes: cfg.ContextReserveMaxBodyBytes,
		limits:    postgres.ContextLimitRepositoryConfig{MaxAccounts: facts.MaxAccounts, MaxLimits: cfg.ContextReserveMaxLimits, MaxScopes: cfg.ContextLimitMaxScopes, MaxScopeBytes: cfg.ContextLimitMaxScopeBytes, MaxTextBytes: facts.MaxTextBytes},
		cache:     query.CompiledPolicyCacheConfig{MaxEntries: cfg.ContextPolicyCacheEntries, MaxCompilations: cfg.ContextPolicyMaxCompilations, SingleTenant: !cfg.MultiTenantEnabled},
		admission: command.ReserveAdmissionConfig{Plan: query.ContextReservationConfig{Facts: facts, MaxLimits: cfg.ContextReserveMaxLimits, MaxScopesPerLimit: cfg.ContextLimitMaxScopes, MaxReservations: cfg.ContextReserveMaxReservations}, MaxRules: cfg.ContextMaxRules, SingleTenant: !cfg.MultiTenantEnabled, MaxTimestampAge: model.MaxTimestampAge, ClockSkewTolerance: model.ClockSkewTolerance, ReservationLifetime: lifetime},
	}, nil
}

func initContextReservation(cfg *Config, conn pgdb.Connection, tx pgdb.TxBeginner, audit command.AuditEventRepository, capacity *postgres.UsageReservationRepository, clk clock.Clock) (*contextReservationRuntime, error) {
	config, err := loadContextReservationConfig(cfg)
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

	return &contextReservationRuntime{handler: handler, identity: config.identity, admission: admission, completion: completion, reservationCompletion: reservationCompletion, config: config}, nil
}
