package utils

import "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"

// BalanceSynced is the metric that tracks the number of balances synchronized.
var BalanceSynced = metrics.Metric{
	Name:        "balance_synced",
	Unit:        "1",
	Description: "Measures the number of balances synced.",
}

// ParentUUIDCorruption is the metric that tracks corrupted UUIDs detected in parent account chains.
// This indicates potential data corruption that should be investigated.
var ParentUUIDCorruption = metrics.Metric{
	Name:        "onboarding_parent_uuid_corruption",
	Unit:        "1",
	Description: "Counts corrupted UUIDs detected in parent account chains during hierarchy validation.",
}
