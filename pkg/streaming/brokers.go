// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import (
	"errors"
	"fmt"
)

// ErrMissingBrokers is returned when streaming is enabled but no broker address
// is configured. Exported so bootstrap tests in both components can assert the
// fail-closed contract with errors.Is rather than by matching message text, and
// so the two components are indistinguishable to an operator reading the log.
var ErrMissingBrokers = errors.New("streaming: STREAMING_ENABLED=true but no broker is configured")

// RequireBrokers fails closed when streaming is enabled with an empty broker
// list.
//
// An enabled producer with nowhere to publish is not a degraded mode, it is
// total, silent event loss. Every emit resolves against a NoopEmitter and is
// discarded without an error, so the IMPORTANT posture has nothing to log as a
// Warn — nothing failed. Readiness cannot see it either: the ledger has no
// streaming prober at all, and the tracer's prober asks the emitter whether it
// is healthy, which a NoopEmitter always answers yes to. The pod goes Ready, the
// dashboards stay green, and the event stream is empty.
//
// That is the same invisible-total-loss failure RequireRosterSource exists to
// kill, so it takes the same posture: refuse boot. An operator who genuinely
// wants no streaming has STREAMING_ENABLED=false, which keeps its graceful
// NoopEmitter and reports "skipped" on the readiness probe.
func RequireBrokers(brokers []string) error {
	if len(brokers) == 0 {
		return fmt.Errorf(
			"%w: set STREAMING_BROKERS to the broker list, or STREAMING_ENABLED=false to run without streaming"+
				" — an enabled producer with no broker discards every event while readiness reports healthy",
			ErrMissingBrokers,
		)
	}

	return nil
}
