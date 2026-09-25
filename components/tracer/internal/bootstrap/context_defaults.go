// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"strconv"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func applyContextDefaults(cfg *Config) {
	profile := tracercontract.DefaultResourceProfile()

	defaults := []struct {
		key    string
		target *int
		value  int
	}{
		{"CONTEXT_MAX_ACCOUNTS", &cfg.ContextMaxAccounts, profile.Facts.MaxAccounts},
		{"CONTEXT_MAX_ENTRIES", &cfg.ContextMaxEntries, profile.Facts.MaxEntries},
		{"CONTEXT_MAX_TEXT_BYTES", &cfg.ContextMaxTextBytes, profile.Facts.MaxTextBytes},
		{"CONTEXT_MAX_INTEGER_DIGITS", &cfg.ContextMaxIntegerDigits, profile.Facts.MaxIntegerDigits},
		{"CONTEXT_RESERVE_MAX_BODY_BYTES", &cfg.ContextReserveMaxBodyBytes, profile.MaxBodyBytes},
		{"CONTEXT_RESERVE_MAX_RESERVATIONS", &cfg.ContextReserveMaxReservations, profile.MaxReservations},
		{"CONTEXT_RESERVE_MAX_LIMITS", &cfg.ContextReserveMaxLimits, 256},
		{"CONTEXT_LIMIT_MAX_SCOPES", &cfg.ContextLimitMaxScopes, profile.Facts.MaxAccounts},
		{"CONTEXT_LIMIT_MAX_SCOPE_BYTES", &cfg.ContextLimitMaxScopeBytes, 65536},
		{"CONTEXT_LIMIT_MAX_BODY_BYTES", &cfg.ContextLimitMaxBodyBytes, profile.MaxBodyBytes},
		{"CONTEXT_POLICY_MAX_BODY_BYTES", &cfg.ContextPolicyMaxBodyBytes, profile.MaxBodyBytes},
		{"CONTEXT_MAX_RULES", &cfg.ContextMaxRules, 100},
		{"CONTEXT_MAX_EXPRESSION_BYTES", &cfg.ContextMaxExpressionBytes, 8192},
		{"CONTEXT_POLICY_CACHE_ENTRIES", &cfg.ContextPolicyCacheEntries, 1024},
		{"CONTEXT_POLICY_MAX_COMPILATIONS", &cfg.ContextPolicyMaxCompilations, 4},
	}
	for _, entry := range defaults {
		if _, exists := os.LookupEnv(entry.key); !exists && *entry.target == 0 {
			*entry.target = entry.value
		}
	}

	strings := []struct {
		key    string
		target *string
		value  string
	}{
		{"CONTEXT_MAX_FRACTION_DIGITS", &cfg.ContextMaxFractionDigits, strconv.Itoa(profile.Facts.MaxFractionDigits)},
		{"CONTEXT_CEL_COST_LIMIT", &cfg.ContextCELCostLimit, "1000000"},
		{"CONTEXT_CEL_TOTAL_COST_LIMIT", &cfg.ContextCELTotalCostLimit, "10000000"},
	}
	for _, entry := range strings {
		if _, exists := os.LookupEnv(entry.key); !exists && *entry.target == "" {
			*entry.target = entry.value
		}
	}
}
