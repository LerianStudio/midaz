// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRealClock_Now(t *testing.T) {
	c := New()

	before := time.Now().UTC()
	got := c.Now()
	after := time.Now().UTC()

	assert.True(t, !got.Before(before), "Clock.Now() should not be before the test start time")
	assert.True(t, !got.After(after), "Clock.Now() should not be after the test end time")
	assert.Equal(t, time.UTC, got.Location(), "Clock.Now() should return UTC time")
}

// MockClock is a test double that returns a fixed time.
type MockClock struct {
	FixedTime time.Time
}

// Now returns the fixed time.
func (m MockClock) Now() time.Time {
	return m.FixedTime
}

func TestMockClock_Now(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	c := MockClock{FixedTime: fixedTime}

	got := c.Now()

	assert.Equal(t, fixedTime, got)
}

func TestRealClock_NewTicker(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
	}{
		{
			name:     "creates ticker with 10ms interval",
			duration: 10 * time.Millisecond,
		},
		{
			name:     "creates ticker with 100ms interval",
			duration: 100 * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := New()

			tickerChan, stopFunc := c.NewTicker(tc.duration)

			// Assert channel is returned
			assert.NotNil(t, tickerChan, "ticker channel should not be nil")
			assert.NotNil(t, stopFunc, "stop function should not be nil")

			// Ensure stop function can be called (cleanup)
			stopFunc()
		})
	}
}

func TestRealClock_NewTicker_ReceivesTick(t *testing.T) {
	c := New()

	// Use short interval for fast test
	tickerChan, stopFunc := c.NewTicker(5 * time.Millisecond)
	defer stopFunc()

	select {
	case tick := <-tickerChan:
		// Assert tick time is not zero
		assert.False(t, tick.IsZero(), "tick time should not be zero")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected to receive at least one tick")
	}
}

func TestRealClock_NewTicker_StopPreventsMoreTicks(t *testing.T) {
	c := New()

	tickerChan, stopFunc := c.NewTicker(5 * time.Millisecond)

	// Stop the ticker immediately
	stopFunc()

	// Wait a bit and verify no more ticks arrive
	// (channel read should timeout)
	select {
	case <-tickerChan:
		// It's acceptable to receive one tick if it was already queued
		// But subsequent reads should timeout
	case <-time.After(50 * time.Millisecond):
		// Expected: no tick received after stop
	}

	// Verify no subsequent ticks arrive after the first possible queued tick
	select {
	case <-tickerChan:
		t.Fatal("received unexpected tick after stop; ticker should have stopped completely")
	case <-time.After(50 * time.Millisecond):
		// Expected: no additional ticks after stop
	}
}

func TestRealClock_ImplementsClockInterface(t *testing.T) {
	// Compile-time check that RealClock implements Clock
	var _ Clock = RealClock{}
}
