package metricskit

import (
	"fmt"
	"strings"
	"time"
)

// AssertionResult represents the outcome of a single assertion.
type AssertionResult struct {
	Name     string
	Passed   bool
	Expected string
	Actual   string
	Message  string
}

// ChaosAssertions provides fluent SLO-based assertions for chaos metrics.
type ChaosAssertions struct {
	snapshot MetricsSnapshot
	results  []AssertionResult
	failed   bool
}

// NewAssertions creates assertions from a metrics snapshot.
func NewAssertions(snapshot MetricsSnapshot) *ChaosAssertions {
	return &ChaosAssertions{
		snapshot: snapshot,
		results:  make([]AssertionResult, 0),
	}
}

// Assert creates assertions directly from a MetricsProvider.
func Assert(provider MetricsProvider) *ChaosAssertions {
	return NewAssertions(provider.Snapshot())
}

// SuccessRateAbove asserts that the success rate is above a threshold.
func (a *ChaosAssertions) SuccessRateAbove(threshold float64) *ChaosAssertions {
	actual := a.snapshot.SuccessRate()
	passed := actual >= threshold

	a.results = append(a.results, AssertionResult{
		Name:     "SuccessRateAbove",
		Passed:   passed,
		Expected: fmt.Sprintf(">= %.2f%%", threshold),
		Actual:   fmt.Sprintf("%.2f%%", actual),
		Message:  fmt.Sprintf("Success rate %.2f%% should be >= %.2f%%", actual, threshold),
	})

	if !passed {
		a.failed = true
	}

	return a
}

// P99Below asserts that P99 latency is below a threshold.
func (a *ChaosAssertions) P99Below(threshold time.Duration) *ChaosAssertions {
	actual := a.snapshot.P99()
	passed := actual <= threshold

	a.results = append(a.results, AssertionResult{
		Name:     "P99Below",
		Passed:   passed,
		Expected: fmt.Sprintf("<= %v", threshold),
		Actual:   fmt.Sprintf("%v", actual),
		Message:  fmt.Sprintf("P99 latency %v should be <= %v", actual, threshold),
	})

	if !passed {
		a.failed = true
	}

	return a
}

// P95Below asserts that P95 latency is below a threshold.
func (a *ChaosAssertions) P95Below(threshold time.Duration) *ChaosAssertions {
	actual := a.snapshot.P95()
	passed := actual <= threshold

	a.results = append(a.results, AssertionResult{
		Name:     "P95Below",
		Passed:   passed,
		Expected: fmt.Sprintf("<= %v", threshold),
		Actual:   fmt.Sprintf("%v", actual),
		Message:  fmt.Sprintf("P95 latency %v should be <= %v", actual, threshold),
	})

	if !passed {
		a.failed = true
	}

	return a
}

// P50Below asserts that median latency is below a threshold.
func (a *ChaosAssertions) P50Below(threshold time.Duration) *ChaosAssertions {
	actual := a.snapshot.P50()
	passed := actual <= threshold

	a.results = append(a.results, AssertionResult{
		Name:     "P50Below",
		Passed:   passed,
		Expected: fmt.Sprintf("<= %v", threshold),
		Actual:   fmt.Sprintf("%v", actual),
		Message:  fmt.Sprintf("P50 (median) latency %v should be <= %v", actual, threshold),
	})

	if !passed {
		a.failed = true
	}

	return a
}

// AverageLatencyBelow asserts that average latency is below a threshold.
func (a *ChaosAssertions) AverageLatencyBelow(threshold time.Duration) *ChaosAssertions {
	actual := a.snapshot.AverageLatency()
	passed := actual <= threshold

	a.results = append(a.results, AssertionResult{
		Name:     "AverageLatencyBelow",
		Passed:   passed,
		Expected: fmt.Sprintf("<= %v", threshold),
		Actual:   fmt.Sprintf("%v", actual),
		Message:  fmt.Sprintf("Average latency %v should be <= %v", actual, threshold),
	})

	if !passed {
		a.failed = true
	}

	return a
}

// ThroughputAbove asserts that throughput is above a threshold (req/s).
func (a *ChaosAssertions) ThroughputAbove(threshold float64) *ChaosAssertions {
	actual := a.snapshot.ThroughputRPS()
	passed := actual >= threshold

	a.results = append(a.results, AssertionResult{
		Name:     "ThroughputAbove",
		Passed:   passed,
		Expected: fmt.Sprintf(">= %.2f req/s", threshold),
		Actual:   fmt.Sprintf("%.2f req/s", actual),
		Message:  fmt.Sprintf("Throughput %.2f req/s should be >= %.2f req/s", actual, threshold),
	})

	if !passed {
		a.failed = true
	}

	return a
}

// TimeoutsBelow asserts that the number of timeouts is below a threshold.
func (a *ChaosAssertions) TimeoutsBelow(threshold int) *ChaosAssertions {
	actual := a.snapshot.GetTimeoutRequests()
	passed := actual <= threshold

	a.results = append(a.results, AssertionResult{
		Name:     "TimeoutsBelow",
		Passed:   passed,
		Expected: fmt.Sprintf("<= %d", threshold),
		Actual:   fmt.Sprintf("%d", actual),
		Message:  fmt.Sprintf("Timeout count %d should be <= %d", actual, threshold),
	})

	if !passed {
		a.failed = true
	}

	return a
}

// FailuresBelow asserts that the number of failures is below a threshold.
func (a *ChaosAssertions) FailuresBelow(threshold int) *ChaosAssertions {
	actual := a.snapshot.GetFailedRequests()
	passed := actual <= threshold

	a.results = append(a.results, AssertionResult{
		Name:     "FailuresBelow",
		Passed:   passed,
		Expected: fmt.Sprintf("<= %d", threshold),
		Actual:   fmt.Sprintf("%d", actual),
		Message:  fmt.Sprintf("Failure count %d should be <= %d", actual, threshold),
	})

	if !passed {
		a.failed = true
	}

	return a
}

// MinRequestsReached asserts minimum number of requests were recorded.
func (a *ChaosAssertions) MinRequestsReached(threshold int) *ChaosAssertions {
	actual := a.snapshot.GetTotalRequests()
	passed := actual >= threshold

	a.results = append(a.results, AssertionResult{
		Name:     "MinRequestsReached",
		Passed:   passed,
		Expected: fmt.Sprintf(">= %d", threshold),
		Actual:   fmt.Sprintf("%d", actual),
		Message:  fmt.Sprintf("Total requests %d should be >= %d", actual, threshold),
	})

	if !passed {
		a.failed = true
	}

	return a
}

// Passed returns true if all assertions passed.
func (a *ChaosAssertions) Passed() bool {
	return !a.failed
}

// Failed returns true if any assertion failed.
func (a *ChaosAssertions) Failed() bool {
	return a.failed
}

// Results returns all assertion results.
func (a *ChaosAssertions) Results() []AssertionResult {
	return a.results
}

// FailedResults returns only the failed assertions.
func (a *ChaosAssertions) FailedResults() []AssertionResult {
	var failed []AssertionResult

	for _, r := range a.results {
		if !r.Passed {
			failed = append(failed, r)
		}
	}

	return failed
}

// Summary returns a human-readable summary of all assertions.
func (a *ChaosAssertions) Summary() string {
	var sb strings.Builder

	passed := 0

	for _, r := range a.results {
		if r.Passed {
			passed++
		}
	}

	fmt.Fprintf(&sb, "Assertions: %d/%d passed\n", passed, len(a.results))
	sb.WriteString(strings.Repeat("-", 50) + "\n")

	for _, r := range a.results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
		}

		fmt.Fprintf(&sb, "[%s] %s: %s (expected %s, got %s)\n",
			status, r.Name, r.Message, r.Expected, r.Actual)
	}

	return sb.String()
}
