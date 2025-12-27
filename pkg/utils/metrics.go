package utils

import "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"

// BalanceSynced is the metric that tracks the number of balances synchronized.
var BalanceSynced = metrics.Metric{
	Name:        "balance_synced",
	Unit:        "1",
	Description: "Measures the number of balances synced.",
}
