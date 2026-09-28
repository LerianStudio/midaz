// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// tracerIntegrationRoster names the applications the Tracer accepts as a
// reservation producer. The ledger's integration id is its APPLICATION_NAME.
var tracerIntegrationRoster = []string{ApplicationName}

type contextTracerRuntimeConfig struct {
	client           tracer.ContextClientConfig
	coordinator      command.ContextTracerConfig
	operationTimeout time.Duration
	transport        string
	tlsMode          string
	integrationID    string
}

func parseContextTracerConfig(cfg *Config) (contextTracerRuntimeConfig, error) {
	endpoint, err := validateContextTracerEndpoint(cfg)
	if err != nil {
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

	endpoint.client = client
	endpoint.coordinator = command.ContextTracerConfig{Bounds: bounds, MaxReservations: client.MaxReservations, AdmissionTimeout: timeout}
	endpoint.operationTimeout = timeout

	return endpoint, nil
}

// validateContextTracerEndpoint applies the boot rules of the integration:
// gRPC identifies the ledger by its client certificate and therefore requires
// native mtls; REST identifies it by an M2M token and accepts any TLS mode,
// with https required under mtls and, in SaaS, an http URL only behind an
// explicit mesh.
func validateContextTracerEndpoint(cfg *Config) (contextTracerRuntimeConfig, error) {
	if cfg == nil || strings.TrimSpace(cfg.TracerBaseURL) == "" {
		return contextTracerRuntimeConfig{}, fmt.Errorf("tracer integration requires TRACER_BASE_URL: %w", constant.ErrTracerContractUnavailable)
	}

	integrationID, err := resolveTracerIntegrationID(cfg.ApplicationName)
	if err != nil {
		return contextTracerRuntimeConfig{}, err
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.TracerTLSMode))
	if mode != "" && mode != tlsModeMesh && mode != tlsModeMTLS {
		return contextTracerRuntimeConfig{}, fmt.Errorf("invalid TRACER_TLS_MODE %q: expected %q or %q: %w", cfg.TracerTLSMode, tlsModeMTLS, tlsModeMesh, constant.ErrTracerContractUnavailable)
	}

	transport := strings.ToLower(strings.TrimSpace(cfg.TracerTransport))
	if transport == "" {
		transport = tracerTransportGRPC
	}

	switch transport {
	case tracerTransportGRPC:
		if mode != tlsModeMTLS {
			return contextTracerRuntimeConfig{}, fmt.Errorf("TRACER_TRANSPORT=grpc identifies the ledger by its client certificate and requires TRACER_TLS_MODE=mtls; use TRACER_TRANSPORT=rest behind a mesh: %w", constant.ErrTracerContractUnavailable)
		}
	case tracerTransportREST:
		if err := validateRESTTracerURL(cfg.TracerBaseURL, mode); err != nil {
			return contextTracerRuntimeConfig{}, err
		}

		if err := ValidateSaaSTracerTLS(cfg.DeploymentMode, cfg.TracerBaseURL, mode); err != nil {
			return contextTracerRuntimeConfig{}, err
		}
	default:
		return contextTracerRuntimeConfig{}, fmt.Errorf("invalid TRACER_TRANSPORT %q: expected %q or %q: %w", cfg.TracerTransport, tracerTransportGRPC, tracerTransportREST, constant.ErrTracerContractUnavailable)
	}

	if cfg.TracerTimeoutMs < mmodel.TracerTimeoutMsMin || cfg.TracerTimeoutMs > mmodel.TracerTimeoutMsMax {
		return contextTracerRuntimeConfig{}, fmt.Errorf("TRACER_TIMEOUT_MS must be explicit and in the ledger supported range: %w", constant.ErrTracerContractUnavailable)
	}

	return contextTracerRuntimeConfig{transport: transport, tlsMode: mode, integrationID: integrationID}, nil
}

// resolveTracerIntegrationID returns the ledger's integration id. An unset
// APPLICATION_NAME is the ledger's own application name.
func resolveTracerIntegrationID(applicationName string) (string, error) {
	integrationID := strings.TrimSpace(applicationName)
	if integrationID == "" {
		integrationID = ApplicationName
	}

	if !slices.Contains(tracerIntegrationRoster, integrationID) {
		return "", fmt.Errorf("APPLICATION_NAME %q is not a Tracer reservation producer; expected one of %q: %w", integrationID, tracerIntegrationRoster, constant.ErrTracerContractUnavailable)
	}

	return integrationID, nil
}

func validateRESTTracerURL(baseURL, mode string) error {
	endpoint, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return fmt.Errorf("TRACER_TRANSPORT=rest requires an http(s) TRACER_BASE_URL: %w", constant.ErrTracerContractUnavailable)
	}

	if mode == tlsModeMTLS && endpoint.Scheme != "https" {
		return fmt.Errorf("TRACER_TRANSPORT=rest with TRACER_TLS_MODE=mtls requires an https TRACER_BASE_URL: %w", constant.ErrTracerContractUnavailable)
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
