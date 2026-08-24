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

	"github.com/LerianStudio/lib-auth/v3/auth/declaration"
	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libLog "github.com/LerianStudio/lib-observability/v2/log"

	ledgerembed "github.com/LerianStudio/midaz/v4/components/ledger"
)

// buildDeclarationPublishers wires the Responsibility-Inversion (RI) permission
// declaration publisher that pushes the embedded permissions manifest to the IdP
// at boot. It builds ONE publisher for the single slug the ledger binary owns —
// "midaz" (core ledger + embedded CRM + embedded fees) — and returns the stop()
// hooks the shutdown runnable drains on SIGTERM. The receiver (plugin-identity)
// enforces BOLA one-identity-one-slug, so fees and CRM are resources INSIDE midaz,
// not slugs of their own.
//
// It is fail-open from construction onward: RI is optional and MUST NOT block or
// crash boot.
//   - DeclarationEnabled=false: returns nil immediately — no publisher, no
//     goroutine, no runnable registered.
//   - Flag on but IDP_HOST / IDP_M2M_CLIENT_ID / IDP_M2M_CLIENT_SECRET empty: a
//     single pre-flight Warn names the empty env vars, then construction proceeds;
//     declaration.New rejects the empty IdentityAddr/credentials, so that publisher
//     is Warn-skipped. The service still serves.
//   - AuthEnabled=false with RI on: the M2M mint yields an empty token, so the
//     background publish fails-open (Warn) inside the publisher; boot is unaffected.
//   - Server-side BOLA rejection arrives as a *declaration.PublishError on the async
//     publish path; it is Warn-only and never blocks the boot.
//
// The secret VALUE is NEVER logged, span-attached, or serialized. The pre-flight
// Warn reports only the NAMES of empty env vars (names are not secrets). Field
// names are chosen to avoid lib-observability redaction tokens (secret, credential,
// client_id, key), which would otherwise blank the field value to [REDACTED].
//
// authClient is taken as the declaration.TokenMinter interface (satisfied by
// *middleware.AuthClient) so it is stubbable in tests; the disabled path returns
// before it is dereferenced, so callers may pass nil there.
func buildDeclarationPublishers(cfg *Config, authClient declaration.TokenMinter, logger libLog.Logger) []func() {
	if !cfg.DeclarationEnabled {
		return nil
	}

	warnIncompleteDeclarationConfig(cfg, logger)

	specs := []struct {
		slug     string
		manifest []byte
	}{
		{slug: "midaz", manifest: ledgerembed.MidazManifest},
	}

	stops := make([]func(), 0, len(specs))

	for _, spec := range specs {
		publisher, err := declaration.New(declaration.Config{
			Slug:         spec.slug,
			Manifest:     spec.manifest,
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
				libLog.String("declaration_slug", spec.slug),
				libLog.Err(err))

			continue
		}

		// FailFast is false, so Start never blocks and never returns a publish
		// error; the initial publish runs in the publisher's own recovered
		// goroutine. A non-nil error here is not expected, but is treated as
		// fail-open all the same.
		stop, err := publisher.Start(context.Background())
		if err != nil {
			logger.Log(context.Background(), libLog.LevelWarn,
				"skipping RI declaration publisher: start failed (fail-open, serving continues)",
				libLog.String("declaration_slug", spec.slug),
				libLog.Err(err))

			continue
		}

		stops = append(stops, stop)
	}

	return stops
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
		"RI declaration enabled but IdP configuration is incomplete; publishers will fail-open (serving continues)",
		libLog.String("empty_idp_declaration_env", strings.Join(missing, ",")))
}

// declarationPublisherRunnable adapts the RI declaration publishers' stop hooks to
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
