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

// systemFactsContractStateReader reports the contract gate the forwarder
// waits on; *systemFactsContractGate in production.
type systemFactsContractStateReader interface {
	contractState() systemFactsContractState
}

// SystemFactsForwarderChecker probes the contract gate and the forwarder's
// lifecycle state. Status().Err can carry hosts and tenant ids, so it is
// logged and never written to the check; the outbox depth is not read for
// the same reason.
type SystemFactsForwarderChecker struct {
	name       string
	forwarder  SystemFactsForwarderStatusReader
	gate       systemFactsContractStateReader
	tlsEnabled bool
	logger     libLog.Logger
}

// NewSystemFactsForwarderChecker builds the forwarder checker over gate (nil
// reads as pending); systemFactsURL is read only for TLS detection.
func NewSystemFactsForwarderChecker(name string, forwarder SystemFactsForwarderStatusReader, gate systemFactsContractStateReader, systemFactsURL string, logger libLog.Logger) *SystemFactsForwarderChecker {
	if logger == nil {
		logger = libLog.NewNop()
	}

	return &SystemFactsForwarderChecker{
		name:       name,
		forwarder:  forwarder,
		gate:       gate,
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

// Check always answers up: the money path does not depend on fact delivery,
// so neither the gate nor the forwarder state takes the pod out of rotation.
// While the gate is pending or rejected the reason names the contract (the
// forwarder cannot have run); once active, a stopped or failed forwarder is
// named in the reason.
func (c *SystemFactsForwarderChecker) Check(ctx context.Context) DependencyCheck {
	switch c.gateState() {
	case systemFactsContractPending:
		return DependencyCheck{Status: StatusUp, Reason: systemFactsReasonContractPending}
	case systemFactsContractRejected:
		return DependencyCheck{Status: StatusUp, Reason: systemFactsReasonContractRejected}
	case systemFactsContractActive:
	}

	status := c.forwarder.Status()
	if status.State != outbox.ForwarderStateIdle && status.State != outbox.ForwarderStateRunning {
		c.logNotRunning(ctx, status)

		return DependencyCheck{Status: StatusUp, Reason: fmt.Sprintf("forwarder %s; facts are not draining on this pod", status.State)}
	}

	return DependencyCheck{Status: StatusUp}
}

// gateState reads the gate; no gate has released nothing.
func (c *SystemFactsForwarderChecker) gateState() systemFactsContractState {
	if c.gate == nil {
		return systemFactsContractPending
	}

	return c.gate.contractState()
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
		systemFacts.producer.Forwarder(), systemFacts.gate, cfg.SystemFactsURL, logger))
}
