// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/LerianStudio/lib-auth/v4/auth/declaration"
	authMiddleware "github.com/LerianStudio/lib-auth/v4/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libLog "github.com/LerianStudio/lib-observability/v4/log"

	tracerembed "github.com/LerianStudio/midaz/v4/components/tracer"
)

// wireDeclarationPublisher builds the RI permission-declaration publisher over
// authHost — the same plugin-auth host initHTTPServer wires — keeping the
// initHTTPServer/finalizeStartup return signatures untouched.
//
// authMiddleware.NewAuthClient is NOT I/O-free: when PluginAuthEnabled is true
// and the address is non-empty it performs a synchronous GET {address}/health at
// construction, so the client is built ONLY when RI is enabled — otherwise the
// default-off path would fire a redundant second health probe (the first is in
// initHTTPServer) and then discard the client. Gating keeps the flag-off boot
// byte-identical to today. buildDeclarationPublisher's disabled path returns
// before the minter is dereferenced, so passing a nil minter is safe.
//
// The error is the fail-closed configuration error described on
// buildDeclarationPublisher; the caller must abort boot on it.
func wireDeclarationPublisher(cfg *Config, authHost string, logger libLog.Logger) ([]func(), error) {
	var declarationAuth declaration.TokenMinter
	if cfg.DeclarationEnabled {
		declarationAuth = authMiddleware.NewAuthClient(authHost, cfg.PluginAuthEnabled, logger)
	}

	return buildDeclarationPublisher(cfg, declarationAuth, logger)
}

// buildDeclarationPublisher wires the Responsibility-Inversion (RI) permission
// declaration publisher that pushes tracer's embedded permissions manifest to the
// IdP at boot. The tracer binary owns exactly ONE service slug — "tracer" — so
// this builds a single publisher and returns the stop() hook(s) the shutdown
// runnable drains on SIGTERM.
//
// FAILURE POLICY — the platform standard, split by failure class, identical to
// the ledger's and to lib-auth auth/declaration.WireFromEnv.
//
// CONFIGURATION ERROR -> FAIL CLOSED (returns an error; the caller aborts boot).
// Deterministic, never transient, always an operator or build defect:
//   - DeclarationEnabled=true with IDP_HOST / IDP_M2M_CLIENT_ID /
//     IDP_M2M_CLIENT_SECRET empty.
//   - declaration.New rejecting the embedded manifest (parse, validate, or a
//     slug that does not equal manifest.service) or the IdP address.
//
// A Warn here would take the pod green while nothing is declared — silent policy
// drift. Under a rolling update this is a stalled rollout, not an outage: the new
// pod never becomes ready and the previous ReplicaSet keeps serving.
//
// RUNTIME FAILURE -> FAIL OPEN (Warn inside the publisher; boot unaffected).
// Environmental, transient, and never a policy risk — what is already
// materialized in the IdP keeps enforcing:
//   - identity unreachable, 5xx, or a failing initial publish (FailFast=false).
//   - PluginAuthEnabled=false with RI on: the M2M mint yields an empty token and
//     the background publish Warns.
//   - Server-side BOLA rejection, arriving as a *declaration.PublishError on the
//     async publish path.
//
// DeclarationEnabled=false returns (nil, nil) immediately — no validation, no
// publisher, no goroutine. While that flag exists it is the switch that says
// whether this deployment is on RI at all; when it is retired the validation
// becomes unconditional (lmap #5163).
//
// The secret VALUE is NEVER logged, span-attached, or serialized. The pre-flight
// Warn reports only the NAMES of empty env vars (names are not secrets). Field
// names are chosen to avoid lib-observability redaction tokens (secret, credential,
// client_id, key), which would otherwise blank the field value to [REDACTED].
//
// authClient is taken as the declaration.TokenMinter interface (satisfied by
// *middleware.AuthClient) so it is stubbable in tests; the disabled path returns
// before it is dereferenced, so callers may pass nil there.
func buildDeclarationPublisher(cfg *Config, authClient declaration.TokenMinter, logger libLog.Logger) ([]func(), error) {
	if !cfg.DeclarationEnabled {
		return nil, nil
	}

	if err := validateDeclarationConfig(cfg); err != nil {
		return nil, err
	}

	publisher, err := declaration.New(declaration.Config{
		Slug:         "tracer",
		Manifest:     tracerembed.TracerManifest,
		IdentityAddr: cfg.IDPHost,
		Auth:         authClient,
		ClientID:     cfg.IDPM2MClientID,
		ClientSecret: cfg.IDPM2MClientSecret,
		Cache:        nil,
		Interval:     0,
		FailFast:     false,
		Logger:       logger,
	})
	if err != nil {
		// Construction only fails on the embedded manifest or the IdP address —
		// both deterministic, both configuration class.
		return nil, fmt.Errorf("build RI declaration publisher for slug %q: %w", "tracer", err)
	}

	// FailFast is false, so Start never blocks and never returns a publish
	// error; the initial publish runs in the publisher's own recovered
	// goroutine. A non-nil error here is therefore not an unreachable IdP — it
	// is a wiring defect, so it fails closed with the rest.
	stop, err := publisher.Start(context.Background())
	if err != nil {
		return nil, fmt.Errorf("start RI declaration publisher for slug %q: %w", "tracer", err)
	}

	return []func(){stop}, nil
}

// validateDeclarationConfig fails closed when RI is enabled but one or more
// required IdP settings are empty. The error names the empty env vars only —
// never any value, and never the secret.
func validateDeclarationConfig(cfg *Config) error {
	missing := make([]string, 0, 3)

	if cfg.IDPHost == "" {
		missing = append(missing, "IDP_HOST")
	}

	if cfg.IDPM2MClientID == "" {
		missing = append(missing, "IDP_M2M_CLIENT_ID")
	}

	if cfg.IDPM2MClientSecret == "" {
		missing = append(missing, "IDP_M2M_CLIENT_SECRET")
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf(
		"IDP_DECLARATION_ENABLED=true but the required IdP configuration is empty: %s",
		strings.Join(missing, ","))
}

// declarationPublisherRunnable adapts the RI declaration publisher's stop hooks to
// the libCommons.App interface. It blocks until SIGINT/SIGTERM and then invokes
// every publisher's stop() so each publisher's background loop is cancelled and
// drained before the process exits. It mirrors streamingProducerRunnable; it holds
// no logger because stop() returns nothing to log.
type declarationPublisherRunnable struct {
	stops []func()
}

// Run blocks until SIGINT/SIGTERM and then invokes every publisher stop hook. Each
// stop cancels the publisher's context and waits for its goroutine to finish, so
// no publisher goroutine outlives shutdown.
func (r *declarationPublisherRunnable) Run(_ *libCommons.Launcher) error {
	if r == nil || len(r.stops) == 0 {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	for _, s := range r.stops {
		if s != nil {
			s()
		}
	}

	return nil
}
