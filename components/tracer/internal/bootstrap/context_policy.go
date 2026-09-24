// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"strconv"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	in "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type contextPolicyConfig struct {
	CEL          cel.ContextAdapterConfig
	Policy       query.ContextPolicyConfig
	MaxBodyBytes int
}

func loadContextPolicyConfig(cfg *Config) (*contextPolicyConfig, error) {
	if !cfg.ContextPolicyAdminEnabled {
		return nil, nil
	}

	if !cfg.PluginAuthEnabled {
		return nil, fmt.Errorf("CONTEXT_POLICY_ADMIN_ENABLED requires PLUGIN_AUTH_ENABLED")
	}

	fraction, err := strconv.Atoi(cfg.ContextMaxFractionDigits)
	if err != nil || fraction < 0 {
		return nil, fmt.Errorf("CONTEXT_MAX_FRACTION_DIGITS must be explicitly set to a nonnegative integer")
	}

	bounds := tracercontract.Limits{MaxAccounts: cfg.ContextMaxAccounts, MaxEntries: cfg.ContextMaxEntries, MaxTextBytes: cfg.ContextMaxTextBytes, MaxIntegerDigits: cfg.ContextMaxIntegerDigits, MaxFractionDigits: fraction}
	if err := bounds.Validate(); err != nil {
		return nil, fmt.Errorf("invalid CONTEXT resource bounds: %w", err)
	}

	cost, err := strconv.ParseUint(cfg.ContextCELCostLimit, 10, 64)
	if err != nil || cost == 0 {
		return nil, fmt.Errorf("CONTEXT_CEL_COST_LIMIT must be explicitly set to a positive integer")
	}

	total, err := strconv.ParseUint(cfg.ContextCELTotalCostLimit, 10, 64)
	if err != nil || total == 0 {
		return nil, fmt.Errorf("CONTEXT_CEL_TOTAL_COST_LIMIT must be explicitly set to a positive integer")
	}

	if cfg.ContextMaxRules <= 0 || cfg.ContextMaxExpressionBytes <= 0 || cfg.ContextPolicyMaxBodyBytes <= 0 {
		return nil, fmt.Errorf("CONTEXT_MAX_RULES, CONTEXT_MAX_EXPRESSION_BYTES and CONTEXT_POLICY_MAX_BODY_BYTES must be positive")
	}

	return &contextPolicyConfig{CEL: cel.ContextAdapterConfig{Limits: bounds, CostLimit: cost, MaxExpressionBytes: cfg.ContextMaxExpressionBytes}, Policy: query.ContextPolicyConfig{MaxRules: cfg.ContextMaxRules, TotalCost: total}, MaxBodyBytes: cfg.ContextPolicyMaxBodyBytes}, nil
}

func initContextPolicyService(cfg *Config, conn pgdb.Connection, tx pgdb.TxBeginner, audit command.AuditEventRepository, clk clock.Clock) (in.ContextPolicyAdminService, error) {
	config, err := loadContextPolicyConfig(cfg)
	if err != nil {
		return nil, err
	}

	if config == nil {
		return nil, nil
	}

	engine, err := cel.NewContextAdapter(config.CEL)
	if err != nil {
		return nil, fmt.Errorf("create context CEL adapter: %w", err)
	}

	evaluator, err := query.NewContextPolicyEvaluator(engine, config.Policy)
	if err != nil {
		return nil, err
	}

	repo, err := postgres.NewContextPolicyRepository(conn, config.Policy.MaxRules)
	if err != nil {
		return nil, err
	}

	publisher, err := command.NewPublishContextPolicyCommand(repo, audit, tx, evaluator, clk)
	if err != nil {
		return nil, err
	}

	binder, err := command.NewBindContextPolicyCommand(repo, audit, tx, evaluator, clk)
	if err != nil {
		return nil, err
	}

	return services.NewContextPolicyService(publisher, binder, repo)
}
