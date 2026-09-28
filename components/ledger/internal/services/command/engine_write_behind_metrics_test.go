// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEngineWriteBehindMetricLabelsAreClosed(t *testing.T) {
	for _, path := range []string{"async", "fallback", "sync", "bulk"} {
		require.Equal(t, path, boundedEngineWriteBehindPath(path))
	}
	require.Equal(t, "unknown", boundedEngineWriteBehindPath("tenant-controlled"))

	for _, outcome := range []string{"published", "completed", "deferred", "failed"} {
		require.Equal(t, outcome, boundedEngineWriteBehindOutcome(outcome))
	}
	require.Equal(t, "unknown", boundedEngineWriteBehindOutcome("payload-controlled"))

	labels := engineWriteBehindMetricLabels("tenant-controlled", "payload-controlled")
	require.Equal(t, map[string]string{"path": "unknown", "outcome": "unknown"}, labels)
	require.NotContains(t, labels, "tenant_id")
	require.NotContains(t, labels, "transaction_id")
	require.NotContains(t, labels, "execution_id")
}
