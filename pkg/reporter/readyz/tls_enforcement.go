// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"fmt"
	"strings"
)

// ============================================================================
// SaaS TLS enforcement — Gate 4 implementation.
//
// Background (Monetarie incident): a SaaS-mode service started successfully
// while its MongoDB DSN was misconfigured without TLS against DocumentDB.
// Liveness probes passed, traffic routed, and clients hit errors. This
// enforcement makes the bootstrap REFUSE to start when DEPLOYMENT_MODE=saas
// and any configured DSN lacks TLS — and the error nominates exactly which
// dependency failed.
//
// Centralization rule: this is the ONLY place TLS enforcement happens. There
// must be NO inline DEPLOYMENT_MODE checks scattered at connection sites.
// Connection-site code keeps its existing behavior (which may be permissive
// for byoc/local). The enforcement gate sits BEFORE any connection opens.
// ============================================================================

// SaaSTLSDep represents a single dependency to check during SaaS TLS
// enforcement.
//
//   - Name is human-readable (e.g., "mongodb", "rabbitmq", "redis"). It is
//     used verbatim in the error message so operators can map the failure
//     back to a specific config field.
//   - URI is the DSN/URL to inspect. Empty URI means "dependency not
//     configured" — that dep is skipped, not flagged as a TLS failure.
//   - DetectFn is one of the Detect* helpers from tls_detection.go (which
//     use url.Parse, never substring matching).
type SaaSTLSDep struct {
	Name     string
	URI      string
	DetectFn func(string) (bool, error)
}

// ValidateSaaSTLS hard-fails the bootstrap when deploymentMode is "saas" AND
// any configured (non-empty) dependency DSN lacks TLS.
//
// Behavior contract:
//   - deploymentMode != "saas" (case-insensitive, trim-tolerant) → returns
//     nil. No enforcement in byoc/local/empty modes.
//   - deploymentMode == "saas" + all deps have TLS (or empty URIs) → nil.
//   - deploymentMode == "saas" + any dep with non-empty URI fails its
//     DetectFn → wrapped error mentioning that dep.
//   - deploymentMode == "saas" + any dep DSN passes through DetectFn but
//     reports TLS=false → error nominating that dep.
//   - Validation stops at the FIRST failing dep (no aggregation). This keeps
//     the bootstrap log focused on the actionable failure rather than
//     cascading downstream noise.
//
// Design intent: called at bootstrap BEFORE any connection opens, so a
// misconfigured SaaS deployment cannot enter the silent-insecure-start state
// that motivated this gate.
func ValidateSaaSTLS(deploymentMode string, deps []SaaSTLSDep) error {
	if !strings.EqualFold(strings.TrimSpace(deploymentMode), "saas") {
		return nil
	}

	for _, dep := range deps {
		uri := strings.TrimSpace(dep.URI)
		if uri == "" {
			// Dep not configured — skip rather than treat as a TLS failure.
			// This lets services pass deps unconditionally and have optional
			// ones (e.g., Fetcher, MultiTenant Redis) auto-skip when unset.
			continue
		}

		tls, err := dep.DetectFn(uri)
		if err != nil {
			return fmt.Errorf(
				"DEPLOYMENT_MODE=saas: TLS validation for %q failed: %w",
				dep.Name, err,
			)
		}

		if !tls {
			return fmt.Errorf(
				"DEPLOYMENT_MODE=saas: TLS required for %q but not configured "+
					"(URI scheme/params indicate insecure connection)",
				dep.Name,
			)
		}
	}

	return nil
}
