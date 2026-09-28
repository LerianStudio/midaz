// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"strconv"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func initContextLimitDefinitionPolicy(cfg *Config) (*command.ContextLimitDefinitionPolicy, error) {
	if !cfg.ContextReserveEnabled {
		return nil, nil
	}

	facts, err := loadContextFactBounds(cfg)
	if err != nil {
		return nil, err
	}

	return command.NewContextLimitDefinitionPolicy(facts, cfg.ContextLimitMaxScopes, cfg.ContextLimitMaxScopeBytes)
}

func loadContextFactBounds(cfg *Config) (tracercontract.Limits, error) {
	fraction, err := strconv.Atoi(cfg.ContextMaxFractionDigits)
	if err != nil || fraction < tracercontract.MinimumResourceProfileFractionDigits {
		return tracercontract.Limits{}, fmt.Errorf("CONTEXT_MAX_FRACTION_DIGITS must be at least %d", tracercontract.MinimumResourceProfileFractionDigits)
	}

	bounds := tracercontract.Limits{MaxAccounts: cfg.ContextMaxAccounts, MaxEntries: cfg.ContextMaxEntries, MaxTextBytes: cfg.ContextMaxTextBytes, MaxIntegerDigits: cfg.ContextMaxIntegerDigits, MaxFractionDigits: fraction}
	if err := bounds.Validate(); err != nil {
		return tracercontract.Limits{}, fmt.Errorf("invalid CONTEXT resource bounds: %w", err)
	}

	return bounds, nil
}
