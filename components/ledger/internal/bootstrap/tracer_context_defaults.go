// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"strconv"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// Only absent variables receive defaults. Explicit zero/empty values retain
// their validation semantics. Fractional digits below the shared technical
// floor fail activation instead of truncating Ledger fee arithmetic.
func applyTracerContextDefaults(cfg *Config) {
	profile := tracercontract.DefaultResourceProfile()

	defaults := []struct {
		key    string
		target *int
		value  int
	}{
		{"TRACER_TIMEOUT_MS", &cfg.TracerTimeoutMs, 250},
		{"TRACER_CONTEXT_MAX_ACCOUNTS", &cfg.TracerContextMaxAccounts, profile.Facts.MaxAccounts},
		{"TRACER_CONTEXT_MAX_ENTRIES", &cfg.TracerContextMaxEntries, profile.Facts.MaxEntries},
		{"TRACER_CONTEXT_MAX_TEXT_BYTES", &cfg.TracerContextMaxTextBytes, profile.Facts.MaxTextBytes},
		{"TRACER_CONTEXT_MAX_INTEGER_DIGITS", &cfg.TracerContextMaxIntegerDigits, profile.Facts.MaxIntegerDigits},
		{"TRACER_CONTEXT_MAX_BODY_BYTES", &cfg.TracerContextMaxBodyBytes, profile.MaxBodyBytes},
		{"TRACER_CONTEXT_MAX_RESERVATIONS", &cfg.TracerContextMaxReservations, profile.MaxReservations},
	}
	for _, entry := range defaults {
		if _, exists := os.LookupEnv(entry.key); !exists && *entry.target == 0 {
			*entry.target = entry.value
		}
	}

	if _, exists := os.LookupEnv("TRACER_CONTEXT_MAX_FRACTION_DIGITS"); !exists && cfg.TracerContextMaxFractionDigits == "" {
		cfg.TracerContextMaxFractionDigits = strconv.Itoa(profile.Facts.MaxFractionDigits)
	}
}
