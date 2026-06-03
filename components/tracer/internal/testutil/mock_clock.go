// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package testutil

import (
	"fmt"
	"os"
	"sync"
	"time"

	"tracer/pkg/clock"
)

// DefaultTestTime is the standard test time, stable within a single test run.
// Set to 1 minute ago to stay within the 24-hour validation window.
// Not reproducible across runs; use a hardcoded time if cross-run determinism is needed.
var DefaultTestTime = time.Now().Add(-1 * time.Minute).UTC()

// FixedTime returns DefaultTestTime, stable within a single test run.
// Use this instead of time.Now() in tests to avoid timestamp validation failures.
func FixedTime() time.Time {
	return DefaultTestTime
}

// TestNow returns the current time consistent with the server's clock.
// If MOCK_TIME env var is set (integration tests with RestartServerWithConfig),
// returns the mocked time so transactionTimestamps stay within the 24h validation window.
// Otherwise returns time.Now() for unit tests and integration tests without MOCK_TIME.
func TestNow() time.Time {
	if mt := os.Getenv("MOCK_TIME"); mt != "" {
		t, err := time.Parse(time.RFC3339, mt)
		if err != nil {
			panic(fmt.Sprintf("MOCK_TIME=%q is not valid RFC3339: %v", mt, err))
		}

		return t.UTC()
	}

	return time.Now().UTC()
}

// MockClock is a test double for clock.Clock that returns a fixed time.
//
// The Now/NewTicker receivers are intentionally value receivers so that the
// clock can be passed around by value or pointer. Tests that need to mutate
// time mid-test should use SetTime (M9) to keep the invariant explicit
// rather than relying on field-level mutation through a pointer.
type MockClock struct {
	FixedTime  time.Time
	TickerChan chan time.Time // Optional: set to control ticker behavior in tests
}

// Now returns the fixed time.
func (m MockClock) Now() time.Time {
	return m.FixedTime
}

// SetTime updates the clock's fixed time. Use this from tests instead of
// direct field mutation so the intent is obvious and future changes to
// MockClock's storage strategy (value vs atomic) do not break callers (M9).
//
// Pointer receiver: mutations via SetTime persist when MockClock is shared
// through a pointer — the same contract callers currently rely on via the
// exported FixedTime field.
func (m *MockClock) SetTime(t time.Time) {
	m.FixedTime = t
}

// NewTicker returns a controllable ticker for testing.
// If TickerChan is set, it returns that channel; otherwise returns a channel that never fires.
// The stop function closes the channel only if it was created internally.
// The stop function is idempotent and safe to call multiple times.
func (m MockClock) NewTicker(_ time.Duration) (<-chan time.Time, func()) {
	if m.TickerChan != nil {
		return m.TickerChan, func() {} // caller controls the channel
	}
	// Return a channel that never fires for tests that don't need ticker
	ch := make(chan time.Time)

	var once sync.Once

	return ch, func() { once.Do(func() { close(ch) }) }
}

// NewMockClock creates a new MockClock with the given fixed time.
func NewMockClock(fixedTime time.Time) clock.Clock {
	return MockClock{FixedTime: fixedTime}
}

// NewDefaultMockClock creates a MockClock initialized with DefaultTestTime.
func NewDefaultMockClock() clock.Clock {
	return MockClock{FixedTime: DefaultTestTime}
}
