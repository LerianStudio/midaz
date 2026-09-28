// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tracercontext"
	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// tracerTokenPrewarmTimeout bounds the boot-time wait for the first M2M token.
const tracerTokenPrewarmTimeout = 5 * time.Second

type contextTracerRuntime struct {
	coordinator *command.ContextTracerCoordinator
	close       func() error
}

// buildContextTracer wires the tracer reservation runtime when TRACER_BASE_URL
// is set and returns nil when it is not. With the integration on, any
// configuration it cannot honor refuses boot. minter, dialing plugin-auth at
// authHost, issues the M2M token the REST transport presents; the gRPC
// transport uses neither.
func buildContextTracer(cfg *Config, onboarding *libPostgres.Client, minter tracerclient.TokenMinter, authHost string, logger libLog.Logger) (_ *contextTracerRuntime, retErr error) {
	if cfg == nil {
		return nil, constant.ErrTracerContractUnavailable
	}

	if strings.TrimSpace(cfg.TracerBaseURL) == "" {
		logger.Log(context.Background(), libLog.LevelInfo, "Tracer reservation integration disabled (TRACER_BASE_URL unset)")

		return nil, nil
	}

	parsed, err := parseContextTracerConfig(cfg)
	if err != nil {
		return nil, err
	}

	if onboarding == nil {
		return nil, constant.ErrTracerContractUnavailable
	}

	facts, err := tracercontext.NewRepository(onboarding, parsed.client.Bounds, cfg.MultiTenantEnabled)
	if err != nil {
		return nil, err
	}

	loader, err := tracerclient.NewOfficialContextLoader(facts, parsed.client.Bounds)
	if err != nil {
		return nil, err
	}

	client, closeClient, tokens, err := buildContextTracerClient(cfg, parsed, minter, authHost)
	if err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil && closeClient != nil {
			_ = closeClient()
		}
	}()

	coordinator, err := command.NewContextTracerCoordinator(client, loader, parsed.coordinator, time.Now)
	if err != nil {
		return nil, err
	}

	logger.Log(context.Background(), libLog.LevelInfo, "Tracer reservation transport selected",
		libLog.String("transport", parsed.transport),
		libLog.String("integration_id", parsed.integrationID))

	if tokens != nil {
		prewarmTracerToken(logger, tokens)
	}

	return &contextTracerRuntime{coordinator: coordinator, close: closeClient}, nil
}

// buildContextTracerClient builds the transport parsed selected. The token
// source is returned only for REST, so the caller can pre-warm it.
func buildContextTracerClient(cfg *Config, parsed contextTracerRuntimeConfig, minter tracerclient.TokenMinter, authHost string) (command.ContextTracerClient, func() error, tracerclient.TokenSource, error) {
	baseURL := strings.TrimSpace(cfg.TracerBaseURL)

	switch parsed.transport {
	case tracerTransportGRPC:
		tlsConfig, err := buildSeamClientTLSConfig(cfg, seamServerName(baseURL))
		if err != nil {
			return nil, nil, nil, err
		}

		if tlsConfig == nil {
			return nil, nil, nil, fmt.Errorf("TRACER_TRANSPORT=grpc requires TRACER_TLS_MODE=mtls: %w", constant.ErrTracerContractUnavailable)
		}

		client, err := tracerclient.NewContextGRPCClient(stripURLScheme(baseURL), parsed.client, tracerclient.WithGRPCOperationTimeout(parsed.operationTimeout), tracerclient.WithGRPCDialOptions(grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))))
		if err != nil {
			return nil, nil, nil, err
		}

		return client, client.Close, nil, nil
	case tracerTransportREST:
		tlsConfig, err := buildRESTTracerTLSConfig(cfg, parsed.tlsMode, seamServerName(baseURL))
		if err != nil {
			return nil, nil, nil, err
		}

		tokens, err := buildTracerTokenSource(cfg, minter, authHost, parsed.tlsMode)
		if err != nil {
			return nil, nil, nil, err
		}

		client, err := tracerclient.NewContextHTTPClient(baseURL, parsed.client, tokens, tracerclient.WithOperationTimeout(parsed.operationTimeout), tracerclient.WithTLSConfig(tlsConfig))
		if err != nil {
			return nil, nil, nil, err
		}

		return client, nil, tokens, nil
	default:
		return nil, nil, nil, fmt.Errorf("unsupported tracer transport %q: %w", parsed.transport, constant.ErrTracerContractUnavailable)
	}
}

// buildRESTTracerTLSConfig returns nil (plaintext) outside mtls. Under mtls it
// verifies the tracer's server certificate against TRACER_TLS_CA_FILE and
// presents the ledger's client certificate only when TRACER_TLS_CERT_FILE or
// TRACER_TLS_KEY_FILE is set: REST identity is the bearer token.
func buildRESTTracerTLSConfig(cfg *Config, mode, serverName string) (*tls.Config, error) {
	if mode != tlsModeMTLS {
		return nil, nil
	}

	if strings.TrimSpace(cfg.TracerTLSCertFile) != "" || strings.TrimSpace(cfg.TracerTLSKeyFile) != "" {
		return buildClientMTLSConfig(cfg, serverName)
	}

	if strings.TrimSpace(cfg.TracerTLSCAFile) == "" {
		return nil, fmt.Errorf("TRACER_TLS_MODE=mtls requires TRACER_TLS_CA_FILE to verify the tracer: %w", constant.ErrTracerContractUnavailable)
	}

	rootCAs, err := loadCertPool(cfg.TracerTLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("load tracer server CA (TRACER_TLS_CA_FILE): %w", err)
	}

	return &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName, RootCAs: rootCAs}, nil
}

// buildTracerTokenSource refuses boot when the REST transport cannot mint an
// M2M token: plugin auth disabled or without a host (the minter would return an
// empty token), empty client credentials, or, under DEPLOYMENT_MODE=saas, a
// cleartext plugin-auth address outside an explicit mesh. authHost is the
// plugin-auth address after service discovery. Only variable names are
// reported, never a value.
func buildTracerTokenSource(cfg *Config, minter tracerclient.TokenMinter, authHost, tlsMode string) (tracerclient.TokenSource, error) {
	if !cfg.AuthEnabled {
		return nil, fmt.Errorf("TRACER_TRANSPORT=rest authenticates to Tracer with an M2M token minted by plugin-auth, but PLUGIN_AUTH_ENABLED=false; enable plugin auth or use TRACER_TRANSPORT=grpc with TRACER_TLS_MODE=mtls: %w", constant.ErrTracerContractUnavailable)
	}

	if strings.TrimSpace(authHost) == "" {
		return nil, fmt.Errorf("TRACER_TRANSPORT=rest authenticates to Tracer with an M2M token minted by plugin-auth, but PLUGIN_AUTH_HOST is empty and service discovery resolved no plugin-auth address: %w", constant.ErrTracerContractUnavailable)
	}

	if err := ValidateSaaSTracerAuthTLS(cfg.DeploymentMode, authHost, tlsMode); err != nil {
		return nil, err
	}

	var missing []string

	if strings.TrimSpace(cfg.IDPM2MClientID) == "" {
		missing = append(missing, "IDP_M2M_CLIENT_ID")
	}

	if cfg.IDPM2MClientSecret == "" {
		missing = append(missing, "IDP_M2M_CLIENT_SECRET")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("TRACER_TRANSPORT=rest authenticates to Tracer with an M2M token, but %s is empty: %w", strings.Join(missing, " and "), constant.ErrTracerContractUnavailable)
	}

	if minter == nil {
		return nil, fmt.Errorf("TRACER_TRANSPORT=rest requires the auth client to mint M2M tokens: %w", constant.ErrTracerContractUnavailable)
	}

	return tracerclient.NewM2MTokenSource(minter, cfg.IDPM2MClientID, cfg.IDPM2MClientSecret, time.Now)
}

// prewarmTracerToken mints the first M2M token at boot so the first
// transaction does not pay for it. It is best-effort: an identity provider that
// is not reachable yet must not keep the ledger from starting. A failed mint
// starts the token source's 5s mint pause; requests during it fail fast with
// 0536, which the fail posture handles, and the first request after it
// retries the mint.
func prewarmTracerToken(logger libLog.Logger, tokens tracerclient.TokenSource) {
	ctx, cancel := context.WithTimeout(context.Background(), tracerTokenPrewarmTimeout)
	defer cancel()

	if _, err := tokens.Token(ctx); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Tracer M2M token could not be minted at boot; a reservation after the mint pause will retry", libLog.Err(err))
	}
}
