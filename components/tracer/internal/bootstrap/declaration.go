// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libLog "github.com/LerianStudio/lib-observability/v4/log"

	tracerembed "github.com/LerianStudio/midaz/v4/components/tracer"
)

// wireDeclarationPublisher builds the RI permission-declaration publisher over
// the boot's own auth client — the one initHTTPServer wires — so publishing opens
// no second client and fires no second health probe.
//
// The error is the fail-closed configuration error described on
// buildDeclarationPublisher; the caller must abort boot on it.
func wireDeclarationPublisher(cfg *Config, authClient *authMiddleware.AuthClient, logger libLog.Logger) ([]func(), error) {
	return buildDeclarationPublisher(cfg, authClient, logger)
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
// DeclarationEnabled=false never fails the boot. While that flag exists it is the
// switch that says whether this deployment declares its permissions at all; when
// it is retired the validation becomes unconditional. The scope catalog is the
// exception: it is published whenever plugin auth is on (see buildScopePublisher),
// because the identity provider validates partner scopes against it. With plugin
// auth off as well, it returns (nil, nil) — no validation, no publisher, no
// goroutine.
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
		return buildScopePublisher(cfg, authClient, logger), nil
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
	missing := missingDeclarationConfig(cfg)
	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf(
		"IDP_DECLARATION_ENABLED=true but the required IdP configuration is empty: %s",
		strings.Join(missing, ","))
}

// missingDeclarationConfig names the required IdP settings that are empty.
func missingDeclarationConfig(cfg *Config) []string {
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

	return missing
}

// buildScopePublisher publishes the manifest's scope section alone, for a
// deployment whose permission declaration is off. It runs only when plugin auth is
// on: the catalog exists for the authorization the deployment enforces.
//
// It never fails the boot. The catalog is what the identity provider validates
// partner scope writes against; a deployment that cannot publish it keeps serving,
// and the reason is logged at ERROR. A SaaS deployment with a cleartext IDP_HOST is
// one such reason — publishing would ship the M2M credential unencrypted.
func buildScopePublisher(cfg *Config, authClient declaration.TokenMinter, logger libLog.Logger) []func() {
	if !cfg.PluginAuthEnabled {
		return nil
	}

	ctx := context.Background()

	notPublished := func(reason error) []func() {
		logger.Log(ctx, libLog.LevelError, "Scope catalog not published; partner scopes for tracer cannot be validated until it is",
			libLog.Err(reason))

		return nil
	}

	if missing := missingDeclarationConfig(cfg); len(missing) > 0 {
		return notPublished(fmt.Errorf("PLUGIN_AUTH_ENABLED=true but the required IdP configuration is empty: %s",
			strings.Join(missing, ",")))
	}

	if isSaaSMode(cfg.DeploymentMode) && idpSchemeIsCleartext(cfg.IDPHost) {
		return notPublished(errors.New(
			"DEPLOYMENT_MODE=saas: TLS required for the scope catalog publication but not configured (set IDP_HOST to an https:// URL)"))
	}

	publisher, err := declaration.New(declaration.Config{
		Slug:         "tracer",
		Manifest:     tracerembed.TracerManifest,
		IdentityAddr: cfg.IDPHost,
		Auth:         authClient,
		ClientID:     cfg.IDPM2MClientID,
		ClientSecret: cfg.IDPM2MClientSecret,
		Logger:       logger,
		ScopeOnly:    true,
	})
	if err != nil {
		return notPublished(err)
	}

	stop, err := publisher.Start(ctx)
	if err != nil {
		return notPublished(err)
	}

	return []func(){stop}
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

// wireAuthScope hands the embedded manifest's scope catalog to the auth client, so
// every tracer guard derives the rule, limit, validation or audit event it sends
// from its own route path. It must run BEFORE any route is registered: Authorize
// reads the catalog at registration, and a route registered first sends no
// instance at all.
func wireAuthScope(auth *authMiddleware.AuthClient) error {
	return declaration.WireScope(auth, tracerembed.TracerManifest)
}
