package utils

import "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"

var (
	BalanceSynced = metrics.Metric{
		Name:        "balance_synced",
		Unit:        "1",
		Description: "Measures the number of balances synced.",
	}

	CircuitBreakerState = metrics.Metric{
		Name:        "circuit_breaker_state",
		Unit:        "1",
		Description: "Current state of circuit breaker (0=closed, 1=open, 2=half_open).",
	}
)
