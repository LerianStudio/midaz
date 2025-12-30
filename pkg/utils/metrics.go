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

// TransactionOutboxFetchFailures is the metric that tracks failures when fetching metadata from
// the Postgres outbox during query-time enrichment.
// This indicates that MongoDB metadata is missing and the outbox fallback also failed.
var TransactionOutboxFetchFailures = metrics.Metric{
	Name:        "transaction_outbox_fetch_failure_total",
	Unit:        "1",
	Description: "Total number of failures when fetching metadata from the Postgres outbox during query-time enrichment.",
}
