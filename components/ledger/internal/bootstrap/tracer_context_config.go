// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type contextTracerRuntimeConfig struct {
	client           tracer.ContextClientConfig
	coordinator      command.ContextTracerConfig
	recovery         command.TracerRecoveryConfig
	worker           TracerRecoveryWorkerConfig
	operationTimeout time.Duration
}

func parseContextTracerConfig(cfg *Config, service string) (contextTracerRuntimeConfig, error) {
	if err := validateContextTracerEndpoint(cfg); err != nil {
		return contextTracerRuntimeConfig{}, err
	}

	bounds, err := parseContextTracerBounds(cfg)
	if err != nil {
		return contextTracerRuntimeConfig{}, err
	}

	client := tracer.ContextClientConfig{Namespace: cfg.TracerAssetNamespace, Bounds: bounds, MaxBodyBytes: cfg.TracerContextMaxBodyBytes, MaxReservations: cfg.TracerContextMaxReservations}
	if err := client.Validate(); err != nil {
		return contextTracerRuntimeConfig{}, err
	}

	recovery, err := parseTracerRecoveryConfig(cfg)
	if err != nil {
		return contextTracerRuntimeConfig{}, err
	}

	worker, err := parseTracerRecoveryWorkerConfig(cfg, service, recovery.RetryInterval)
	if err != nil {
		return contextTracerRuntimeConfig{}, err
	}

	return contextTracerRuntimeConfig{client: client, coordinator: command.ContextTracerConfig{Facts: tracerreservation.Config{Bounds: bounds, MaxBodyBytes: client.MaxBodyBytes}, MaxReservations: client.MaxReservations, AdmissionTimeout: time.Duration(cfg.TracerTimeoutMs) * time.Millisecond}, recovery: recovery, worker: worker, operationTimeout: time.Duration(cfg.TracerTimeoutMs) * time.Millisecond}, nil
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

func parseTracerRecoveryConfig(cfg *Config) (command.TracerRecoveryConfig, error) {
	interval, err := tracerRecoveryDuration(cfg.TracerRecoveryIntervalMs)
	if err != nil {
		return command.TracerRecoveryConfig{}, fmt.Errorf("TRACER_RECOVERY_INTERVAL_MS: %w", err)
	}

	attempt, err := tracerRecoveryDuration(cfg.TracerRecoveryAttemptTimeoutMs)
	if err != nil {
		return command.TracerRecoveryConfig{}, fmt.Errorf("TRACER_RECOVERY_ATTEMPT_TIMEOUT_MS: %w", err)
	}

	recovery := command.TracerRecoveryConfig{IntegrationID: cfg.TracerIntegrationID, Namespace: cfg.TracerAssetNamespace, SingleTenant: !cfg.MultiTenantEnabled, MaxBatch: cfg.TracerRecoveryBatchSize, RetryInterval: interval, AttemptTimeout: attempt}

	recovery.MaxRetryInterval, err = tracerRecoveryDuration(cfg.TracerRecoveryMaxRetryIntervalMs)
	if err != nil {
		return command.TracerRecoveryConfig{}, fmt.Errorf("TRACER_RECOVERY_MAX_RETRY_INTERVAL_MS: %w", err)
	}

	if err := recovery.Validate(); err != nil {
		return command.TracerRecoveryConfig{}, err
	}

	return recovery, nil
}

func parseTracerRecoveryWorkerConfig(cfg *Config, service string, interval time.Duration) (TracerRecoveryWorkerConfig, error) {
	cycle, err := tracerRecoveryDuration(cfg.TracerRecoveryCycleTimeoutMs)
	if err != nil {
		return TracerRecoveryWorkerConfig{}, fmt.Errorf("TRACER_RECOVERY_CYCLE_TIMEOUT_MS: %w", err)
	}

	tenant, err := tracerRecoveryDuration(cfg.TracerRecoveryTenantTimeoutMs)
	if err != nil {
		return TracerRecoveryWorkerConfig{}, fmt.Errorf("TRACER_RECOVERY_TENANT_TIMEOUT_MS: %w", err)
	}

	worker := TracerRecoveryWorkerConfig{MultiTenant: cfg.MultiTenantEnabled, Service: service, Interval: interval, CycleTimeout: cycle, TenantTimeout: tenant, MaxTenants: cfg.TracerRecoveryMaxTenants, MaxCatalogTenants: cfg.TracerRecoveryMaxCatalogTenants}
	if worker.MaxTenants <= 0 || worker.MaxCatalogTenants < worker.MaxTenants || (worker.MultiTenant && strings.TrimSpace(service) == "") {
		return TracerRecoveryWorkerConfig{}, constant.ErrTracerContractUnavailable
	}

	return worker, nil
}

func tracerRecoveryDuration(milliseconds int) (time.Duration, error) {
	if milliseconds <= 0 || int64(milliseconds) > math.MaxInt64/int64(time.Millisecond) {
		return 0, constant.ErrTracerContractUnavailable
	}

	return time.Duration(milliseconds) * time.Millisecond, nil
}
