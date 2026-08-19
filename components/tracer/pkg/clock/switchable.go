// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package clock

import (
	"sync"
	"time"
)

// SwitchableClock delegates to a Clock that can be replaced atomically.
// Production bootstrap does not use it; the integration build wraps the service
// clock so time-based cases can share one server without mutating an HTTP or
// environment surface.
type SwitchableClock struct {
	mu        sync.RWMutex
	current   switchableClockState
	nextToken uint64
}

type switchableClockState struct {
	clock Clock
	token uint64
}

// NewSwitchableClock creates a clock backed by initial.
func NewSwitchableClock(initial Clock) *SwitchableClock {
	return &SwitchableClock{current: switchableClockState{clock: initial}}
}

// Now returns the active clock's current time.
func (c *SwitchableClock) Now() time.Time {
	c.mu.RLock()
	current := c.current.clock
	c.mu.RUnlock()

	return current.Now()
}

// NewTicker creates a ticker from the active clock.
func (c *SwitchableClock) NewTicker(d time.Duration) (<-chan time.Time, func()) {
	c.mu.RLock()
	current := c.current.clock
	c.mu.RUnlock()

	return current.NewTicker(d)
}

// Use replaces the active clock and returns an idempotent restore function.
// A stale restore never overwrites a newer replacement.
func (c *SwitchableClock) Use(next Clock) func() {
	c.mu.Lock()
	previous := c.current
	c.nextToken++
	token := c.nextToken
	c.current = switchableClockState{clock: next, token: token}
	c.mu.Unlock()

	var once sync.Once

	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()

			if c.current.token != token {
				return
			}

			c.current = previous
		})
	}
}
