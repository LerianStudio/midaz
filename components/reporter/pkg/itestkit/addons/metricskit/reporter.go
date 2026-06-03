package metricskit

import (
	"fmt"
	"io"
	"strings"
)

// Reporter generates human-readable reports from chaos metrics.
type Reporter struct {
	snapshot MetricsSnapshot
}

// NewReporter creates a reporter from a metrics snapshot.
func NewReporter(snapshot MetricsSnapshot) *Reporter {
	return &Reporter{snapshot: snapshot}
}

// Report creates a reporter directly from a MetricsProvider.
func Report(provider MetricsProvider) *Reporter {
	return NewReporter(provider.Snapshot())
}

// WriteReport writes a formatted report to the given writer.
func (r *Reporter) WriteReport(w io.Writer) error {
	s := r.snapshot

	lines := []string{
		"",
		"╔══════════════════════════════════════════════════════════════╗",
		"║                    CHAOS TEST REPORT                         ║",
		"╠══════════════════════════════════════════════════════════════╣",
		"",
		"  REQUEST METRICS",
		fmt.Sprintf("     Total Requests:      %d", s.GetTotalRequests()),
		fmt.Sprintf("     Successful:          %d", s.GetTotalRequests()-s.GetFailedRequests()),
		fmt.Sprintf("     Failed:              %d", s.GetFailedRequests()),
		fmt.Sprintf("     Timeouts:            %d", s.GetTimeoutRequests()),
		fmt.Sprintf("     Success Rate:        %.2f%%", s.SuccessRate()),
		"",
		"  LATENCY METRICS",
		fmt.Sprintf("     Average:             %v", s.AverageLatency()),
		fmt.Sprintf("     Min:                 %v", s.GetMinLatency()),
		fmt.Sprintf("     P50 (median):        %v", s.P50()),
		fmt.Sprintf("     P90:                 %v", s.P90()),
		fmt.Sprintf("     P95:                 %v", s.P95()),
		fmt.Sprintf("     P99:                 %v", s.P99()),
		fmt.Sprintf("     P99.9:               %v", s.P999()),
		"",
		"  THROUGHPUT",
		fmt.Sprintf("     Overall:             %.2f req/s", s.ThroughputRPS()),
		fmt.Sprintf("     Successful:          %.2f req/s", s.SuccessfulThroughputRPS()),
		fmt.Sprintf("     During Chaos:        %.2f req/s", s.ChaosThroughputRPS()),
		"",
		"  DURATION",
		fmt.Sprintf("     Test Duration:       %v", s.TestDuration()),
		fmt.Sprintf("     Chaos Duration:      %v", s.ChaosDuration()),
		"",
	}

	// Error breakdown
	errorCounts := s.GetErrorCounts()
	if len(errorCounts) > 0 {
		lines = append(lines, "  ERROR BREAKDOWN")
		for category, count := range errorCounts {
			lines = append(lines, fmt.Sprintf("     %-18s %d", string(category)+":", count))
		}

		lines = append(lines, "")
	}

	lines = append(lines,
		"╚══════════════════════════════════════════════════════════════╝",
		"",
	)

	_, err := io.WriteString(w, strings.Join(lines, "\n"))

	return err
}

// String returns the report as a string.
func (r *Reporter) String() string {
	var sb strings.Builder

	_ = r.WriteReport(&sb)

	return sb.String()
}

// CompactSummary returns a one-line summary suitable for logs.
func (r *Reporter) CompactSummary() string {
	s := r.snapshot

	return fmt.Sprintf(
		"requests=%d success_rate=%.2f%% p99=%v throughput=%.2f/s chaos_duration=%v",
		s.GetTotalRequests(),
		s.SuccessRate(),
		s.P99(),
		s.ThroughputRPS(),
		s.ChaosDuration(),
	)
}
