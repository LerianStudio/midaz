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

const (
	// systemFactsForwarderCheckerName is the checks-map key of the forwarder probe.
	systemFactsForwarderCheckerName = "system_facts_forwarder"
	// systemFactsEmissionOffReason is the check reason while the contract
	// preflight keeps emission off.
	systemFactsEmissionOffReason = "emission disabled by contract preflight"
)

// SystemFactsForwarderStatusReader reports the forwarder's Run lifecycle;
// *outbox.Forwarder in production.
type SystemFactsForwarderStatusReader interface {
	Status() outbox.ForwarderStatus
}

// SystemFactsForwarderChecker probes the System Facts forwarder's lifecycle
// state. Status().Err can carry hosts and tenant ids, so it is logged and
// never written to the check; the outbox depth is not read for the same reason.
type SystemFactsForwarderChecker struct {
	name       string
	forwarder  SystemFactsForwarderStatusReader
	emitting   bool
	tlsEnabled bool
	logger     libLog.Logger
}

// NewSystemFactsForwarderChecker builds the forwarder checker. emitting false
// names the preflight in the check reason; systemFactsURL is read only for TLS
// detection.
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

// Check always answers up: the money path does not depend on fact delivery, so
// the forwarder state never takes the pod out of rotation. A stopped or failed
// forwarder, and emission kept off by the preflight, are named in the reason.
func (c *SystemFactsForwarderChecker) Check(ctx context.Context) DependencyCheck {
	var reasons []string

	status := c.forwarder.Status()
	if status.State != outbox.ForwarderStateIdle && status.State != outbox.ForwarderStateRunning {
		c.logNotRunning(ctx, status)

		reasons = append(reasons, fmt.Sprintf("forwarder %s; facts are not draining on this pod", status.State))
	}

	if !c.emitting {
		reasons = append(reasons, systemFactsEmissionOffReason)
	}

	return DependencyCheck{Status: StatusUp, Reason: strings.Join(reasons, "; ")}
}

func (c *SystemFactsForwarderChecker) logNotRunning(ctx context.Context, status outbox.ForwarderStatus) {
	fields := []any{libLog.String("state", string(status.State))}
	if status.Err != nil {
		fields = append(fields, libLog.Err(status.Err))
	}

	c.logger.Log(ctx, libLog.LevelWarn, "System Facts forwarder is not running; facts are not draining on this pod", fields...)
}

// appendSystemFactsForwarderChecker registers the forwarder checker whenever
// the producer is enabled; with SYSTEM_FACTS_ENABLED=false nothing is registered.
func appendSystemFactsForwarderChecker(checkers []DependencyChecker, cfg *Config, systemFacts *systemFactsComponents, logger libLog.Logger) []DependencyChecker {
	if !cfg.SystemFactsEnabled {
		return checkers
	}

	return append(checkers, NewSystemFactsForwarderChecker(systemFactsForwarderCheckerName,
		systemFacts.producer.Forwarder(), systemFacts.emitting, cfg.SystemFactsURL, logger))
}
