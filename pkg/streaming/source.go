// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import (
	"errors"
	"fmt"
)

// ErrSourceNotRoster is returned when the configured CloudEvents source is not
// the application's roster name. Exported so bootstrap tests can assert the
// fail-closed contract with errors.Is rather than by matching message text.
var ErrSourceNotRoster = errors.New("streaming: configured ce-source is not this application's roster name")

// RequireRosterSource fails closed unless the configured ce-source is exactly the
// application's roster name.
//
// A grammar-legal source is NOT a usable one. The tenant-manager grants a
// producer WRITE+DESCRIBE on precisely three topic names — its topic, its
// commands queue, its DLQ — derived from the ROSTER service name, and every
// grant is a LITERAL pattern, deliberately: a prefixed grant on
// "lerian.streaming.ledger" would also authorize "lerian.streaming.ledgerx",
// reaching a neighbouring application's stream. The roster name is also what
// gates association admission, so nothing else is ever provisioned.
//
// Any other source is therefore unpublishable by construction: the derived topic
// does not exist and is not auto-created, the credential holds no grant on it,
// and the DLQ that a failed publish would fall back to is ungranted too. midaz
// emits under the IMPORTANT posture, which swallows an emit failure as a single
// Warn and returns success, and the ledger has no streaming readiness prober —
// so the deployment would lose every event while reporting healthy. That failure
// is invisible; a refused boot is not.
//
// This is checked whether or not streaming is ENABLED. A source left over from
// the pre-v3 dotted or URI shape must fail loudly at startup rather than sit in
// an env file until someone flips the flag.
func RequireRosterSource(configured, roster string) error {
	if configured != roster {
		return fmt.Errorf(
			"%w: STREAMING_CLOUDEVENTS_SOURCE must be %q, got %q — the broker topics and Kafka ACLs are provisioned for the roster name only",
			ErrSourceNotRoster, roster, configured,
		)
	}

	return nil
}
