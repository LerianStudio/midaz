// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type contextTracerRuntimeConfig struct {
	client           tracer.ContextClientConfig
	coordinator      command.ContextTracerConfig
	operationTimeout time.Duration
}

func parseContextTracerConfig(cfg *Config) (contextTracerRuntimeConfig, error) {
	if err := validateContextTracerEndpoint(cfg); err != nil {
		return contextTracerRuntimeConfig{}, err
	}

	bounds, err := parseContextTracerBounds(cfg)
	if err != nil {
		return contextTracerRuntimeConfig{}, err
	}

	client := tracer.ContextClientConfig{Bounds: bounds, MaxBodyBytes: cfg.TracerContextMaxBodyBytes, MaxReservations: cfg.TracerContextMaxReservations}
	if err := client.Validate(); err != nil {
		return contextTracerRuntimeConfig{}, err
	}

	timeout := time.Duration(cfg.TracerTimeoutMs) * time.Millisecond

	return contextTracerRuntimeConfig{client: client, coordinator: command.ContextTracerConfig{Bounds: bounds, MaxReservations: client.MaxReservations, AdmissionTimeout: timeout}, operationTimeout: timeout}, nil
}

func validateContextTracerEndpoint(cfg *Config) error {
	if cfg == nil || strings.TrimSpace(cfg.TracerBaseURL) == "" || strings.ToLower(strings.TrimSpace(cfg.TracerTLSMode)) != "mtls" {
		return fmt.Errorf("context tracer requires TRACER_BASE_URL and native mtls: %w", constant.ErrTracerContractUnavailable)
	}

	if cfg.TracerTimeoutMs < mmodel.TracerTimeoutMsMin || cfg.TracerTimeoutMs > mmodel.TracerTimeoutMsMax {
		return fmt.Errorf("TRACER_TIMEOUT_MS must be explicit and in the ledger supported range: %w", constant.ErrTracerContractUnavailable)
	}

	return nil
}

func parseContextTracerBounds(cfg *Config) (tracercontract.Limits, error) {
	fraction, err := strconv.Atoi(cfg.TracerContextMaxFractionDigits)
	if err != nil || fraction < 0 {
		return tracercontract.Limits{}, fmt.Errorf("TRACER_CONTEXT_MAX_FRACTION_DIGITS must be explicit and nonnegative: %w", constant.ErrTracerContractUnavailable)
	}

	return tracercontract.Limits{MaxAccounts: cfg.TracerContextMaxAccounts, MaxEntries: cfg.TracerContextMaxEntries, MaxTextBytes: cfg.TracerContextMaxTextBytes, MaxIntegerDigits: cfg.TracerContextMaxIntegerDigits, MaxFractionDigits: fraction}, nil
}
