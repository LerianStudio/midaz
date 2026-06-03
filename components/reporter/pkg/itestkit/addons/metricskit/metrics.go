package metricskit

import (
	"maps"
	"slices"
	"sync"
	"time"
)

// ChaosMetrics collects metrics during chaos tests.
// Thread-safe for concurrent recording from multiple goroutines.
type ChaosMetrics struct {
	mu sync.Mutex

	// Request metrics
	TotalRequests      int
	SuccessfulRequests int
	FailedRequests     int
	TimeoutRequests    int

	// Timing metrics
	TotalLatency time.Duration
	MinLatency   time.Duration
	MaxLatency   time.Duration
	Latencies    []time.Duration

	// Percentile cache (calculated on demand)
	percentileCache map[float64]time.Duration
	percentileDirty bool

	// State tracking
	ChaosStartTime time.Time
	ChaosEndTime   time.Time
	TestStartTime  time.Time
	TestEndTime    time.Time

	// Error classification
	ErrorClassifier *ErrorClassifier
}

// NewChaosMetrics creates a new ChaosMetrics instance.
func NewChaosMetrics() *ChaosMetrics {
	return &ChaosMetrics{
		MinLatency:      time.Hour, // Will be updated with actual min
		Latencies:       make([]time.Duration, 0),
		ErrorClassifier: NewErrorClassifier(),
	}
}

// StartTest records the test start time.
func (m *ChaosMetrics) StartTest() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TestStartTime = time.Now()
}

// EndTest records the test end time.
func (m *ChaosMetrics) EndTest() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TestEndTime = time.Now()
}

// StartChaos records when chaos injection started.
func (m *ChaosMetrics) StartChaos() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ChaosStartTime = time.Now()
}

// EndChaos records when chaos injection ended.
func (m *ChaosMetrics) EndChaos() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ChaosEndTime = time.Now()
}

// RecordRequest records a request result.
func (m *ChaosMetrics) RecordRequest(success bool, timeout bool, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalRequests++
	m.TotalLatency += latency
	m.Latencies = append(m.Latencies, latency)
	m.percentileDirty = true

	if latency < m.MinLatency {
		m.MinLatency = latency
	}

	if latency > m.MaxLatency {
		m.MaxLatency = latency
	}

	if success {
		m.SuccessfulRequests++
	} else {
		m.FailedRequests++
	}

	if timeout {
		m.TimeoutRequests++
	}
}

// RecordError records and classifies an error message.
func (m *ChaosMetrics) RecordError(errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ErrorClassifier != nil {
		m.ErrorClassifier.RecordError(errMsg)
	}
}

// GetErrorCounts returns error counts by category.
func (m *ChaosMetrics) GetErrorCounts() map[ErrorCategory]int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ErrorClassifier == nil {
		return make(map[ErrorCategory]int)
	}

	return m.ErrorClassifier.GetCategoryCounts()
}

// GetTotalRequests returns the total number of requests recorded.
func (m *ChaosMetrics) GetTotalRequests() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.TotalRequests
}

// GetFailedRequests returns the number of failed requests.
func (m *ChaosMetrics) GetFailedRequests() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.FailedRequests
}

// GetTimeoutRequests returns the number of timeout requests.
func (m *ChaosMetrics) GetTimeoutRequests() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.TimeoutRequests
}

// GetMinLatency returns the minimum latency, or 0 if no requests recorded.
func (m *ChaosMetrics) GetMinLatency() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.TotalRequests == 0 {
		return 0
	}

	return m.MinLatency
}

// SuccessRate returns the success rate as a percentage (0-100).
func (m *ChaosMetrics) SuccessRate() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.TotalRequests == 0 {
		return 0
	}

	return float64(m.SuccessfulRequests) / float64(m.TotalRequests) * 100
}

// AverageLatency returns the average latency.
func (m *ChaosMetrics) AverageLatency() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.TotalRequests == 0 {
		return 0
	}

	return m.TotalLatency / time.Duration(m.TotalRequests)
}

// ThroughputRPS returns the requests per second during the test.
func (m *ChaosMetrics) ThroughputRPS() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	duration := m.TestEndTime.Sub(m.TestStartTime)
	if duration <= 0 {
		duration = time.Since(m.TestStartTime)
	}

	if duration <= 0 {
		return 0
	}

	return float64(m.TotalRequests) / duration.Seconds()
}

// SuccessfulThroughputRPS returns the successful requests per second.
func (m *ChaosMetrics) SuccessfulThroughputRPS() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	duration := m.TestEndTime.Sub(m.TestStartTime)
	if duration <= 0 {
		duration = time.Since(m.TestStartTime)
	}

	if duration <= 0 {
		return 0
	}

	return float64(m.SuccessfulRequests) / duration.Seconds()
}

// ChaosThroughputRPS returns the requests per second during the chaos period.
func (m *ChaosMetrics) ChaosThroughputRPS() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	duration := m.ChaosEndTime.Sub(m.ChaosStartTime)
	if duration <= 0 {
		if !m.ChaosStartTime.IsZero() {
			duration = time.Since(m.ChaosStartTime)
		}
	}

	if duration <= 0 {
		return 0
	}

	return float64(m.TotalRequests) / duration.Seconds()
}

// ChaosDuration returns how long chaos was active.
func (m *ChaosMetrics) ChaosDuration() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ChaosEndTime.IsZero() {
		return time.Since(m.ChaosStartTime)
	}

	return m.ChaosEndTime.Sub(m.ChaosStartTime)
}

// TestDuration returns the total test duration.
func (m *ChaosMetrics) TestDuration() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.TestEndTime.IsZero() {
		return time.Since(m.TestStartTime)
	}

	return m.TestEndTime.Sub(m.TestStartTime)
}

// Percentile returns the latency at the given percentile (0-100).
// Uses nearest-rank method.
func (m *ChaosMetrics) Percentile(p float64) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()

	if p < 0 || p > 100 {
		return 0
	}

	if len(m.Latencies) == 0 {
		return 0
	}

	if m.percentileCache == nil {
		m.percentileCache = make(map[float64]time.Duration)
	}

	if m.percentileDirty {
		m.percentileCache = make(map[float64]time.Duration)
		m.percentileDirty = false
	}

	if cached, ok := m.percentileCache[p]; ok {
		return cached
	}

	sorted := make([]time.Duration, len(m.Latencies))
	copy(sorted, m.Latencies)
	slices.Sort(sorted)

	index := max(int(float64(len(sorted)-1)*p/100.0), 0)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	result := sorted[index]
	m.percentileCache[p] = result

	return result
}

// P50 returns the 50th percentile (median) latency.
func (m *ChaosMetrics) P50() time.Duration { return m.Percentile(50) }

// P90 returns the 90th percentile latency.
func (m *ChaosMetrics) P90() time.Duration { return m.Percentile(90) }

// P95 returns the 95th percentile latency.
func (m *ChaosMetrics) P95() time.Duration { return m.Percentile(95) }

// P99 returns the 99th percentile latency.
func (m *ChaosMetrics) P99() time.Duration { return m.Percentile(99) }

// P999 returns the 99.9th percentile latency.
func (m *ChaosMetrics) P999() time.Duration { return m.Percentile(99.9) }

// Snapshot returns a copy of the current metrics state.
func (m *ChaosMetrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	latencies := make([]time.Duration, len(m.Latencies))
	copy(latencies, m.Latencies)

	var percentileCache map[float64]time.Duration
	if m.percentileCache != nil {
		percentileCache = make(map[float64]time.Duration)
		maps.Copy(percentileCache, m.percentileCache)
	}

	var errorClassifier *ErrorClassifier
	if m.ErrorClassifier != nil {
		errorClassifier = m.ErrorClassifier.Clone()
	}

	return &ChaosMetrics{
		TotalRequests:      m.TotalRequests,
		SuccessfulRequests: m.SuccessfulRequests,
		FailedRequests:     m.FailedRequests,
		TimeoutRequests:    m.TimeoutRequests,
		TotalLatency:       m.TotalLatency,
		MinLatency:         m.MinLatency,
		MaxLatency:         m.MaxLatency,
		Latencies:          latencies,
		percentileCache:    percentileCache,
		percentileDirty:    m.percentileDirty,
		ChaosStartTime:     m.ChaosStartTime,
		ChaosEndTime:       m.ChaosEndTime,
		TestStartTime:      m.TestStartTime,
		TestEndTime:        m.TestEndTime,
		ErrorClassifier:    errorClassifier,
	}
}

// Reset clears all metrics to initial state.
func (m *ChaosMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalRequests = 0
	m.SuccessfulRequests = 0
	m.FailedRequests = 0
	m.TimeoutRequests = 0
	m.TotalLatency = 0
	m.MinLatency = time.Hour
	m.MaxLatency = 0
	m.Latencies = make([]time.Duration, 0)
	m.percentileCache = nil
	m.percentileDirty = false
	m.ChaosStartTime = time.Time{}
	m.ChaosEndTime = time.Time{}
	m.TestStartTime = time.Time{}
	m.TestEndTime = time.Time{}

	if m.ErrorClassifier != nil {
		m.ErrorClassifier.Reset()
	}
}

// Compile-time interface verification
var (
	_ MetricsProvider = (*ChaosMetrics)(nil)
	_ MetricsSnapshot = (*ChaosMetrics)(nil)
)
