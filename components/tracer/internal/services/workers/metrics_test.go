// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricDeclarations_AllSixDefined(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		metric           Metric
		wantName         string
		wantUnit         string
		wantDescContains string
	}{
		{
			name:             "polls total",
			metric:           MetricCacheSyncPollsTotal,
			wantName:         "tracer_cache_sync_polls_total",
			wantUnit:         "1",
			wantDescContains: "poll",
		},
		{
			name:             "errors total",
			metric:           MetricCacheSyncErrorsTotal,
			wantName:         "tracer_cache_sync_errors_total",
			wantUnit:         "1",
			wantDescContains: "error",
		},
		{
			name:             "duration milliseconds",
			metric:           MetricCacheSyncDuration,
			wantName:         "tracer_cache_sync_duration_milliseconds",
			wantUnit:         "ms",
			wantDescContains: "Duration",
		},
		{
			name:             "rules changed total",
			metric:           MetricCacheSyncRulesChanged,
			wantName:         "tracer_cache_sync_rules_changed_total",
			wantUnit:         "1",
			wantDescContains: "rules",
		},
		{
			name:             "rule cache size",
			metric:           MetricCacheSyncRuleCacheSize,
			wantName:         "tracer_cache_sync_rule_cache_size",
			wantUnit:         "1",
			wantDescContains: "cache",
		},
		{
			name:             "staleness seconds",
			metric:           MetricCacheSyncStalenessSeconds,
			wantName:         "tracer_cache_sync_staleness_seconds",
			wantUnit:         "s",
			wantDescContains: "staleness",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.wantName, tc.metric.Name)
			assert.Equal(t, tc.wantUnit, tc.metric.Unit)
			assert.NotEmpty(t, tc.metric.Description)
			assert.Contains(t, tc.metric.Description, tc.wantDescContains)
		})
	}
}
