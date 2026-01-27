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

	CircuitBreakerConsecutiveFailures = metrics.Metric{
		Name:        "circuit_breaker_consecutive_failures",
		Unit:        "1",
		Description: "Number of consecutive failures before circuit opened.",
	}

	CircuitBreakerTotalFailures = metrics.Metric{
		Name:        "circuit_breaker_total_failures",
		Unit:        "1",
		Description: "Total number of failures recorded by circuit breaker.",
	}

	CircuitBreakerTotalRequests = metrics.Metric{
		Name:        "circuit_breaker_total_requests",
		Unit:        "1",
		Description: "Total number of requests recorded by circuit breaker.",
	}

	CircuitBreakerFailureRatio = metrics.Metric{
		Name:        "circuit_breaker_failure_ratio_bps",
		Unit:        "bps",
		Description: "Failure ratio in basis points (0-10000, where 10000 = 100%). Divide by 100 to get percentage.",
	}
)
