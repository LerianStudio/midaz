// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	in "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type limitAssetConfig struct {
	facts        tracercontract.Limits
	identity     *seamidentity.Resolver
	repository   postgres.ContextLimitRepositoryConfig
	maxBodyBytes int
}

func loadLimitAssetConfig(cfg *Config) (*limitAssetConfig, error) {
	if !cfg.ContextLimitAdminEnabled {
		return nil, nil
	}

	if !cfg.PluginAuthEnabled || cfg.TracerTLSMode != "mtls" {
		return nil, fmt.Errorf("CONTEXT_LIMIT_ADMIN_ENABLED requires PLUGIN_AUTH_ENABLED and native TRACER_TLS_MODE=mtls")
	}

	facts, err := loadContextFactBounds(cfg)
	if err != nil {
		return nil, err
	}

	if cfg.ContextLimitMaxScopes <= 0 || cfg.ContextLimitMaxScopeBytes <= 0 || cfg.ContextLimitMaxBodyBytes <= 0 {
		return nil, fmt.Errorf("CONTEXT_LIMIT_MAX_SCOPES, CONTEXT_LIMIT_MAX_SCOPE_BYTES and CONTEXT_LIMIT_MAX_BODY_BYTES must be positive")
	}
	// Operator configuration has a fixed boot-time size cap, separate from the
	// measured financial request envelope. Never decode an unbounded env value.
	if len(cfg.ContextProducerBindings) == 0 || len(cfg.ContextProducerBindings) > 65536 {
		return nil, fmt.Errorf("CONTEXT_PRODUCER_BINDINGS must contain 1 to 65536 bytes of JSON")
	}

	var bindings []seamidentity.Binding

	decoder := json.NewDecoder(bytes.NewBufferString(cfg.ContextProducerBindings))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&bindings); err != nil {
		return nil, fmt.Errorf("decode CONTEXT_PRODUCER_BINDINGS: %w", err)
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("CONTEXT_PRODUCER_BINDINGS must contain one JSON array")
	}

	identity, err := seamidentity.NewResolver(bindings, facts.MaxTextBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid CONTEXT_PRODUCER_BINDINGS: %w", err)
	}

	return &limitAssetConfig{
		facts: facts, identity: identity, maxBodyBytes: cfg.ContextLimitMaxBodyBytes,
		// Administration locks one limit by ID; it never lists admission candidates.
		repository: postgres.ContextLimitRepositoryConfig{MaxAccounts: facts.MaxAccounts, MaxLimits: 1, MaxScopes: cfg.ContextLimitMaxScopes, MaxScopeBytes: cfg.ContextLimitMaxScopeBytes, MaxTextBytes: facts.MaxTextBytes},
	}, nil
}

func loadContextFactBounds(cfg *Config) (tracercontract.Limits, error) {
	fraction, err := strconv.Atoi(cfg.ContextMaxFractionDigits)
	if err != nil || fraction < 0 {
		return tracercontract.Limits{}, fmt.Errorf("CONTEXT_MAX_FRACTION_DIGITS must be explicitly set to a nonnegative integer")
	}

	bounds := tracercontract.Limits{MaxAccounts: cfg.ContextMaxAccounts, MaxEntries: cfg.ContextMaxEntries, MaxTextBytes: cfg.ContextMaxTextBytes, MaxIntegerDigits: cfg.ContextMaxIntegerDigits, MaxFractionDigits: fraction}
	if err := bounds.Validate(); err != nil {
		return tracercontract.Limits{}, fmt.Errorf("invalid CONTEXT resource bounds: %w", err)
	}

	return bounds, nil
}

func initLimitAssetAdmin(cfg *Config, tx pgdb.TxBeginner, audit command.AuditEventRepository, clk clock.Clock) (*in.LimitAssetHandler, error) {
	config, err := loadLimitAssetConfig(cfg)
	if err != nil || config == nil {
		return nil, err
	}

	repo, err := postgres.NewContextLimitRepository(config.repository)
	if err != nil {
		return nil, err
	}

	binder, err := command.NewBindLimitAssetCommand(repo, audit, tx, clk, command.LimitAssetBindingConfig{Facts: config.facts, MaxScopes: config.repository.MaxScopes, SingleTenant: !cfg.MultiTenantEnabled})
	if err != nil {
		return nil, err
	}

	return in.NewLimitAssetHandler(binder, config.identity, config.facts, config.maxBodyBytes)
}
