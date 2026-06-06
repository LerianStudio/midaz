// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"os"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/buildinfo"
	"github.com/gofiber/fiber/v2"
)

// SelfProbeGate is the contract LivenessHandler depends on to decide whether
// to return 200 (delegates to libHTTP.Ping) or 503 ("self-probe has not
// completed"). Bootstrap supplies bootstrap.IsSelfProbeOK as the production
// implementation; tests inject a stub.
//
// Defined as a function type rather than an interface because the contract
// is a single zero-arg bool getter — an interface would be needless ceremony.
type SelfProbeGate func() bool

// defaultSelfProbeGate is overridden by SetSelfProbeGate at boot. Stays nil
// in tests that exercise the handler in isolation; LivenessHandler treats
// nil as "no gate wired ⇒ admit" so existing test fixtures keep working.
var defaultSelfProbeGate SelfProbeGate

// SetSelfProbeGate installs the production self-probe gate. Bootstrap calls
// this once at boot with bootstrap.IsSelfProbeOK so the Health handler can
// short-circuit to 503 while RunSelfProbe has not yet flipped the atomic.
//
// Concurrent callers are not expected — this runs once during InitServers
// before any HTTP traffic — so no synchronization is needed.
func SetSelfProbeGate(gate SelfProbeGate) {
	defaultSelfProbeGate = gate
}

// LivenessHandler returns the K8s liveness probe handler.
//
// Behavior:
//   - selfProbeOK == false (or no gate wired in test contexts): return 503
//     with body explaining why. K8s livenessProbe will restart the pod if
//     /health never reports healthy, preventing zombie pods that boot
//     without their dependencies.
//   - selfProbeOK == true: delegate to libHTTP.Ping for the canonical
//     "healthy" 200 response.
//
// Liveness is "process is alive" — once the startup self-probe has confirmed
// dependencies were reachable AT BOOT, /health does NOT re-probe them.
// Re-probing is /readyz's job; mixing the two creates the /health/live +
// /health/ready split anti-pattern (#3).
//
//	@Summary		Health check (liveness probe)
//	@Description	Returns 503 until startup self-probe completes; 200 "healthy" thereafter.
//	@ID				getHealth
//	@Tags			health
//	@Accept			plain
//	@Produce		plain
//	@Success		200	{string}	string	"healthy"
//	@Failure		503	{object}	map[string]string	"self-probe has not completed"
//	@Router			/health [get]
func (h *HealthChecker) LivenessHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		gate := defaultSelfProbeGate
		if gate != nil && !gate() {
			// Use libHTTP.Respond so the response goes through the canonical
			// envelope helper instead of bypassing it with c.Status().JSON().
			return libHTTP.Respond(c, fiber.StatusServiceUnavailable, fiber.Map{
				"status":  "unhealthy",
				"message": "self-probe has not completed",
			})
		}

		return libHTTP.Ping(c)
	}
}

// versionHandler is built once from the VERSION env var, preserving the
// lib-commons Version source semantics (it read VERSION directly), and adds
// the buildinfo provenance fields (commit/buildTime/dirty) to the wire shape.
var versionHandler = buildinfo.VersionHandler(os.Getenv("VERSION"))

// Version godoc
//
//	@Summary		Get service version
//	@Description	Returns the current version of the service plus build provenance
//	@ID				getVersion
//	@Tags			info
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	api.VersionResponse	"Version information"
//	@Router			/version [get]
func Version(c *fiber.Ctx) error {
	return versionHandler(c)
}
