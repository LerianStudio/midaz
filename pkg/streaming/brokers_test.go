// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// TestRequireBrokers locks the fail-closed contract: no broker means no boot, and
// the refusal carries both the sentinel (so bootstrap tests match on identity,
// not message text) and the STREAMING_ENABLED=false escape hatch (so an operator
// does not "fix" it by pointing STREAMING_BROKERS at a dead host).
func TestRequireBrokers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		brokers []string
		wantErr bool
	}{
		{name: "nil slice", brokers: nil, wantErr: true},
		{name: "empty slice", brokers: []string{}, wantErr: true},
		{name: "one broker", brokers: []string{"broker-a:9092"}},
		{name: "several brokers", brokers: []string{"broker-a:9092", "broker-b:9092"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := pkgStreaming.RequireBrokers(tc.brokers)
			if !tc.wantErr {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, pkgStreaming.ErrMissingBrokers)
			assert.Contains(t, err.Error(), "STREAMING_BROKERS",
				"the refusal must name the variable the operator has to set")
			assert.Contains(t, err.Error(), "STREAMING_ENABLED=false",
				"the refusal must name the supported way to run without streaming")
		})
	}
}

// TestRequireBrokers_MirrorsRosterGateStyle keeps the two boot gates reading as
// one contract: both wrap an exported sentinel and both explain the consequence
// rather than only naming the field, because an operator meets them at 3am in a
// crash-looping pod's log.
func TestRequireBrokers_MirrorsRosterGateStyle(t *testing.T) {
	t.Parallel()

	brokerErr := pkgStreaming.RequireBrokers(nil)
	require.Error(t, brokerErr)

	rosterErr := pkgStreaming.RequireRosterSource("not-the-roster", "ledger")
	require.Error(t, rosterErr)

	for _, err := range []error{brokerErr, rosterErr} {
		assert.True(t, strings.HasPrefix(err.Error(), "streaming: "),
			"both boot gates must be attributable to streaming at a glance: %q", err.Error())
		assert.Contains(t, err.Error(), " — ",
			"both boot gates must state the consequence after the em dash: %q", err.Error())
	}
}
