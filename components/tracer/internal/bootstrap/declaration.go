// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/LerianStudio/lib-auth/v4/auth/declaration"
	authMiddleware "github.com/LerianStudio/lib-auth/v4/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
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
// byte-identical to today. buildDeclarationPublisher is fail-open and its disabled
// path returns before the minter is dereferenced, so passing a nil minter is safe.
func wireDeclarationPublisher(cfg *Config, authHost string, logger libLog.Logger) []func() {
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
// It is fail-open from construction onward: RI is optional and MUST NOT block or
// crash boot.
//   - DeclarationEnabled=false: returns nil immediately — no publisher, no
//     goroutine, no runnable registered.
//   - Flag on but IDP_HOST / IDP_M2M_CLIENT_ID / IDP_M2M_CLIENT_SECRET empty: a
//     single pre-flight Warn names the empty env vars, then construction proceeds;
//     declaration.New rejects the empty IdentityAddr/credentials, so the publisher
//     is Warn-skipped. The service still serves.
//   - PluginAuthEnabled=false with RI on: the M2M mint yields an empty token, so
//     the background publish fails-open (Warn) inside the publisher; boot is
//     unaffected.
//   - Server-side BOLA rejection arrives as a *declaration.PublishError on the
//     async publish path; it is Warn-only and never blocks boot.
//
// The secret VALUE is NEVER logged, span-attached, or serialized. The pre-flight
// Warn reports only the NAMES of empty env vars (names are not secrets). Field
// names are chosen to avoid lib-observability redaction tokens (secret, credential,
// client_id, key), which would otherwise blank the field value to [REDACTED].
//
// authClient is taken as the declaration.TokenMinter interface (satisfied by
// *middleware.AuthClient) so it is stubbable in tests; the disabled path returns
// before it is dereferenced, so callers may pass nil there.
func buildDeclarationPublisher(cfg *Config, authClient declaration.TokenMinter, logger libLog.Logger) []func() {
	if !cfg.DeclarationEnabled {
		return nil
	}

	warnIncompleteDeclarationConfig(cfg, logger)

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
		logger.Log(context.Background(), libLog.LevelWarn,
			"skipping RI declaration publisher: construction failed (fail-open, serving continues)",
			libLog.String("declaration_slug", "tracer"),
			libLog.Err(err))

		return nil
	}

	// FailFast is false, so Start never blocks and never returns a publish
	// error; the initial publish runs in the publisher's own recovered
	// goroutine. A non-nil error here is not expected, but is treated as
	// fail-open all the same.
	stop, err := publisher.Start(context.Background())
	if err != nil {
		logger.Log(context.Background(), libLog.LevelWarn,
			"skipping RI declaration publisher: start failed (fail-open, serving continues)",
			libLog.String("declaration_slug", "tracer"),
			libLog.Err(err))

		return nil
	}

	return []func(){stop}
}

// warnIncompleteDeclarationConfig emits a single structured Warn when RI is enabled
// but one or more required IdP settings are empty. It logs the NAMES of the empty
// env vars only — never any value, and never the secret. The field name avoids
// redaction tokens so the value survives the zap redactor.
func warnIncompleteDeclarationConfig(cfg *Config, logger libLog.Logger) {
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
		return
	}

	logger.Log(context.Background(), libLog.LevelWarn,
		"RI declaration enabled but IdP configuration is incomplete; publisher will fail-open (serving continues)",
		libLog.String("empty_idp_declaration_env", strings.Join(missing, ",")))
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
