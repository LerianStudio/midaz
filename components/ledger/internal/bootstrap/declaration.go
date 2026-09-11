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
	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libLog "github.com/LerianStudio/lib-observability/v4/log"

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
// FAILURE POLICY — the platform standard, split by failure class. This mirrors
// lib-auth auth/declaration.WireFromEnv, which br-sisbajud, plugin-br-pix-switch
// and plugin-br-pix-indirect-btg already follow; the ledger used to be the one
// adopter that fail-opened on a bad configuration, and that divergence is what
// this shape removes.
//
// CONFIGURATION ERROR -> FAIL CLOSED (returns an error; the caller aborts boot).
// Deterministic, never transient, and always an operator or build defect:
//   - DeclarationEnabled=true with IDP_HOST / IDP_M2M_CLIENT_ID /
//     IDP_M2M_CLIENT_SECRET empty.
//   - declaration.New rejecting the embedded manifest (parse, validate, or a
//     slug that does not equal manifest.service) or the IdP address.
//
// Degrading these to a Warn is worse than a stalled rollout: the pod goes green
// and the operator believes RI is on while nothing is declared — silent policy
// drift. A rolling update turns this into a stalled rollout, not an outage: the
// new pod never becomes ready and the previous ReplicaSet keeps serving.
//
// RUNTIME FAILURE -> FAIL OPEN (Warn inside the publisher; boot unaffected).
// Environmental and transient, and never a policy risk, because whatever is
// already materialized in the IdP keeps enforcing — the declaration only
// converges it:
//   - identity unreachable, 5xx, or a failing initial publish (FailFast=false,
//     so Start does not block and the publish runs in the publisher's own
//     recovered goroutine).
//   - AuthEnabled=false with RI on: the M2M mint yields an empty token and the
//     background publish Warns.
//   - Server-side BOLA rejection, arriving as a *declaration.PublishError on the
//     async publish path.
//
// DeclarationEnabled=false returns (nil, nil) immediately — no validation, no
// publisher, no goroutine, no runnable. While that flag still exists it is the
// switch that says whether this deployment is on RI at all, so a deployment that
// has not adopted RI must boot untouched. When the flag is retired and RI is the
// only path, the validation becomes unconditional (lmap #5163).
//
// The secret VALUE is NEVER logged, span-attached, serialized, or included in any
// returned error. Only the NAMES of empty env vars are reported (names are not
// secrets). Field names avoid lib-observability redaction tokens (secret,
// credential, client_id, key), which would otherwise blank the value to
// [REDACTED].
//
// authClient is taken as the declaration.TokenMinter interface (satisfied by
// *middleware.AuthClient) so it is stubbable in tests; the disabled path returns
// before it is dereferenced, so callers may pass nil there.
func buildDeclarationPublishers(cfg *Config, authClient declaration.TokenMinter, logger libLog.Logger) ([]func(), error) {
	if !cfg.DeclarationEnabled {
		return nil, nil
	}

	if err := validateDeclarationConfig(cfg); err != nil {
		return nil, err
	}

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
			// Construction only fails on the embedded manifest or the IdP
			// address — both deterministic, both configuration class.
			return nil, fmt.Errorf("build RI declaration publisher for slug %q: %w", spec.slug, err)
		}

		// FailFast is false, so Start never blocks and never returns a publish
		// error; the initial publish runs in the publisher's own recovered
		// goroutine. A non-nil error here is therefore not an unreachable IdP —
		// it is a wiring defect, so it fails closed with the rest.
		stop, err := publisher.Start(context.Background())
		if err != nil {
			drainStops(stops)

			return nil, fmt.Errorf("start RI declaration publisher for slug %q: %w", spec.slug, err)
		}

		stops = append(stops, stop)
	}

	return stops, nil
}

// drainStops releases the publishers already started when a later one fails to
// start, so the fail-closed path leaks no background goroutine on the way out.
func drainStops(stops []func()) {
	for _, stop := range stops {
		stop()
	}
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
