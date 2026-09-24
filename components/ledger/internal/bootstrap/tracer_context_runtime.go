// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tracercontext"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tracerobligation"
	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type contextTracerDependencies struct {
	metricsFactory *metrics.MetricsFactory
	onboarding     *libPostgres.Client
	transaction    *libPostgres.Client
	catalog        tracerRecoveryCatalog
	resolver       tracerRecoveryPoolResolver
	service        string
	logger         libLog.Logger
}

type contextTracerRuntime struct {
	coordinator *command.ContextTracerCoordinator
	worker      *TracerRecoveryWorker
	close       func() error
}

func buildContextTracer(cfg *Config, deps contextTracerDependencies) (_ *contextTracerRuntime, retErr error) {
	if cfg == nil {
		return nil, constant.ErrTracerContractUnavailable
	}

	if !cfg.TracerContextEnabled {
		return nil, nil
	}

	parsed, err := parseContextTracerConfig(cfg, deps.service)
	if err != nil {
		return nil, err
	}

	if deps.onboarding == nil || deps.transaction == nil || deps.logger == nil {
		return nil, constant.ErrTracerContractUnavailable
	}

	facts, err := tracercontext.NewRepository(deps.onboarding, parsed.client.Bounds, cfg.MultiTenantEnabled)
	if err != nil {
		return nil, err
	}

	loader, err := tracerclient.NewOfficialContextLoader(facts, parsed.client.Namespace, parsed.client.Bounds)
	if err != nil {
		return nil, err
	}

	journal, err := tracerobligation.NewRepository(deps.transaction, parsed.coordinator.Facts, cfg.MultiTenantEnabled, parsed.recovery.MaxBatch)
	if err != nil {
		return nil, err
	}

	client, closeClient, err := buildContextTracerClient(cfg, parsed)
	if err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil && closeClient != nil {
			_ = closeClient()
		}
	}()

	recovery, err := command.NewTracerRecoveryProcessor(journal, client, journal, parsed.recovery, time.Now)
	if err != nil {
		return nil, err
	}

	recovery.MetricsFactory = deps.metricsFactory

	coordinator, err := command.NewContextTracerCoordinator(recovery, loader, parsed.coordinator)
	if err != nil {
		return nil, err
	}

	worker, err := NewTracerRecoveryWorker(coordinator, deps.catalog, deps.resolver, parsed.worker, deps.logger)
	if err != nil {
		return nil, err
	}

	return &contextTracerRuntime{coordinator: coordinator, worker: worker, close: closeClient}, nil
}

func buildContextTracerClient(cfg *Config, parsed contextTracerRuntimeConfig) (command.ContextTracerReserver, func() error, error) {
	baseURL := strings.TrimSpace(cfg.TracerBaseURL)

	tlsConfig, err := buildSeamClientTLSConfig(cfg, seamServerName(baseURL))
	if err != nil {
		return nil, nil, err
	}

	if tlsConfig == nil {
		return nil, nil, constant.ErrTracerContractUnavailable
	}

	transport := strings.ToLower(strings.TrimSpace(cfg.TracerTransport))
	switch transport {
	case "", tracerTransportGRPC:
		client, err := tracerclient.NewContextGRPCClient(stripURLScheme(baseURL), parsed.client, tracerclient.WithGRPCOperationTimeout(parsed.operationTimeout), tracerclient.WithGRPCDialOptions(grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))))
		if err != nil {
			return nil, nil, err
		}

		return client, client.Close, nil
	case tracerTransportREST:
		endpoint, err := url.Parse(baseURL)
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
			return nil, nil, fmt.Errorf("context REST tracer requires https: %w", constant.ErrTracerContractUnavailable)
		}

		client, err := tracerclient.NewContextHTTPClient(baseURL, parsed.client, tracerclient.WithOperationTimeout(parsed.operationTimeout), tracerclient.WithTLSConfig(tlsConfig))

		return client, nil, err
	default:
		return nil, nil, fmt.Errorf("unsupported context tracer transport: %w", constant.ErrTracerContractUnavailable)
	}
}

func combineTracerClosers(closers ...func() error) func() error {
	var active []func() error

	for _, closeClient := range closers {
		if closeClient != nil {
			active = append(active, closeClient)
		}
	}

	if len(active) == 0 {
		return nil
	}

	return func() error {
		var failures []error

		for _, closeClient := range active {
			if err := closeClient(); err != nil {
				failures = append(failures, err)
			}
		}

		return errors.Join(failures...)
	}
}
