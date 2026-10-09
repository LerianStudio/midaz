// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-system-facts/outbox"
)

// systemFactsForwarderCheckerName is the checks-map key of the forwarder probe.
const systemFactsForwarderCheckerName = "system_facts_forwarder"

// SystemFactsForwarderStatusReader reports the forwarder's Run lifecycle;
// *outbox.Forwarder in production.
type SystemFactsForwarderStatusReader interface {
	Status() outbox.ForwarderStatus
}

// SystemFactsForwarderChecker probes the System Facts forwarder's lifecycle
// state. The state is a closed set the kit declares probe-safe; Status().Err
// can carry hosts and tenant ids, so it is logged and never written to the
// check. The outbox depth is deliberately not read: its error can carry the
// same detail, and the kit's depth gauges already cover the backlog.
type SystemFactsForwarderChecker struct {
	name       string
	forwarder  SystemFactsForwarderStatusReader
	emitting   bool
	tlsEnabled bool
	logger     libLog.Logger
}

// NewSystemFactsForwarderChecker builds the forwarder checker. A nil forwarder
// reports skipped; emitting false keeps the check up and names the preflight in
// its reason; systemFactsURL is read only for TLS detection.
func NewSystemFactsForwarderChecker(name string, forwarder SystemFactsForwarderStatusReader, emitting bool, systemFactsURL string, logger libLog.Logger) *SystemFactsForwarderChecker {
	if logger == nil {
		logger = libLog.NewNop()
	}

	return &SystemFactsForwarderChecker{
		name:       name,
		forwarder:  forwarder,
		emitting:   emitting,
		tlsEnabled: strings.HasPrefix(strings.ToLower(strings.TrimSpace(systemFactsURL)), "https://"),
		logger:     logger,
	}
}

// Name returns the checker identifier.
func (c *SystemFactsForwarderChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether the System Facts URL is https.
func (c *SystemFactsForwarderChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check reads the forwarder state: idle (not started yet; the Launcher starts
// apps concurrently) and running are up, stopped and failed are down with the
// fixed text "forwarder <state>". With emission off the check stays up and its
// reason names the preflight, so readiness never fails for a contract the
// reconciler Job has yet to register.
func (c *SystemFactsForwarderChecker) Check(ctx context.Context) DependencyCheck {
	if c.forwarder == nil {
		return DependencyCheck{Status: StatusSkipped, Reason: "System Facts emission disabled"}
	}

	check := DependencyCheck{Status: StatusUp}
	if !c.emitting {
		check.Reason = "emission disabled by contract preflight"
	}

	status := c.forwarder.Status()
	if status.State == outbox.ForwarderStateIdle || status.State == outbox.ForwarderStateRunning {
		return check
	}

	if status.Err != nil {
		c.logger.Log(ctx, libLog.LevelWarn, "System Facts forwarder is not running",
			libLog.String("state", string(status.State)), libLog.Err(status.Err))
	}

	check.Status = StatusDown
	check.Error = fmt.Sprintf("forwarder %s", status.State)

	return check
}

// appendSystemFactsForwarderChecker registers the forwarder checker whenever
// the producer is enabled, so a forwarder that stopped is visible; with
// SYSTEM_FACTS_ENABLED=false nothing is registered.
func appendSystemFactsForwarderChecker(checkers []DependencyChecker, cfg *Config, systemFacts *systemFactsComponents, logger libLog.Logger) []DependencyChecker {
	if !cfg.SystemFactsEnabled {
		return checkers
	}

	var (
		forwarder SystemFactsForwarderStatusReader
		emitting  bool
	)

	if systemFacts != nil {
		if f := systemFacts.producer.Forwarder(); f != nil {
			forwarder = f
		}

		emitting = systemFacts.emitting
	}

	return append(checkers, NewSystemFactsForwarderChecker(systemFactsForwarderCheckerName, forwarder, emitting, cfg.SystemFactsURL, logger))
}
