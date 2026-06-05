// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"

	"github.com/stretchr/testify/assert"
)

func TestFullJitter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		baseDelay time.Duration
		wantMin   time.Duration
		wantMax   time.Duration
	}{
		{
			name:      "zero base returns zero",
			baseDelay: 0,
			wantMin:   0,
			wantMax:   0,
		},
		{
			name:      "negative base returns zero",
			baseDelay: -1 * time.Second,
			wantMin:   0,
			wantMax:   0,
		},
		{
			name:      "initial backoff range",
			baseDelay: constant.ProducerInitialBackoff,
			wantMin:   0,
			wantMax:   constant.ProducerInitialBackoff,
		},
		{
			name:      "1 second base",
			baseDelay: 1 * time.Second,
			wantMin:   0,
			wantMax:   1 * time.Second,
		},
		{
			name:      "base exceeding max is capped",
			baseDelay: 30 * time.Second,
			wantMin:   0,
			wantMax:   constant.ProducerMaxBackoff,
		},
		{
			name:      "base exactly at max",
			baseDelay: constant.ProducerMaxBackoff,
			wantMin:   0,
			wantMax:   constant.ProducerMaxBackoff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Run multiple iterations to verify range consistency
			for i := 0; i < 100; i++ {
				result := FullJitter(tt.baseDelay)
				assert.GreaterOrEqual(t, result, tt.wantMin, "jitter below minimum on iteration %d", i)
				assert.LessOrEqual(t, result, tt.wantMax, "jitter above maximum on iteration %d", i)
			}
		})
	}
}

func TestFullJitter_Distribution(t *testing.T) {
	t.Parallel()

	// Verify that FullJitter produces non-deterministic output
	// by checking that not all values are the same over many iterations.
	const iterations = 50

	baseDelay := 1 * time.Second
	results := make(map[time.Duration]bool)

	for i := 0; i < iterations; i++ {
		results[FullJitter(baseDelay)] = true
	}

	// With crypto/rand we expect at least some variance
	assert.Greater(t, len(results), 1, "expected non-deterministic jitter values")
}

func TestNextBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current time.Duration
		want    time.Duration
	}{
		{
			name:    "initial backoff doubles",
			current: constant.ProducerInitialBackoff,
			want:    1 * time.Second,
		},
		{
			name:    "1 second doubles to 2 seconds",
			current: 1 * time.Second,
			want:    2 * time.Second,
		},
		{
			name:    "2 seconds doubles to 4 seconds",
			current: 2 * time.Second,
			want:    4 * time.Second,
		},
		{
			name:    "4 seconds doubles to 8 seconds",
			current: 4 * time.Second,
			want:    8 * time.Second,
		},
		{
			name:    "8 seconds doubles but capped at max",
			current: 8 * time.Second,
			want:    constant.ProducerMaxBackoff,
		},
		{
			name:    "already at max stays at max",
			current: constant.ProducerMaxBackoff,
			want:    constant.ProducerMaxBackoff,
		},
		{
			name:    "beyond max stays at max",
			current: 20 * time.Second,
			want:    constant.ProducerMaxBackoff,
		},
		{
			name:    "zero remains zero",
			current: 0,
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NextBackoff(tt.current)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestBackoffProgression(t *testing.T) {
	t.Parallel()

	// Verify the full backoff progression matches midaz pattern:
	// 500ms -> 1s -> 2s -> 4s -> 8s -> 10s (cap)
	expected := []time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		constant.ProducerMaxBackoff,
	}

	backoff := constant.ProducerInitialBackoff

	for i, want := range expected {
		assert.Equal(t, want, backoff, "backoff mismatch at step %d", i)
		backoff = NextBackoff(backoff)
	}

	// After capping, should stay at max
	assert.Equal(t, constant.ProducerMaxBackoff, backoff, "backoff should stay at max after cap")
}

func TestBackoffCalculator_Next(t *testing.T) {
	t.Parallel()

	bc := &BackoffCalculator{
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
		Factor:       2.0,
	}

	tests := []struct {
		name    string
		current time.Duration
		want    time.Duration
	}{
		{"doubles 1s to 2s", 1 * time.Second, 2 * time.Second},
		{"doubles 2s to 4s", 2 * time.Second, 4 * time.Second},
		{"caps at max", 8 * time.Second, 10 * time.Second},
		{"stays at max", 10 * time.Second, 10 * time.Second},
		{"zero stays zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, bc.Next(tt.current))
		})
	}
}

func TestBackoffCalculator_Jitter(t *testing.T) {
	t.Parallel()

	bc := &BackoffCalculator{
		InitialDelay: 1 * time.Second,
		MaxDelay:     5 * time.Second,
		Factor:       2.0,
	}

	t.Run("returns zero for zero base", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, time.Duration(0), bc.Jitter(0))
	})

	t.Run("returns zero for negative base", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, time.Duration(0), bc.Jitter(-1*time.Second))
	})

	t.Run("caps at MaxDelay", func(t *testing.T) {
		t.Parallel()
		for i := 0; i < 100; i++ {
			j := bc.Jitter(20 * time.Second)
			assert.LessOrEqual(t, j, bc.MaxDelay)
		}
	})

	t.Run("is non-deterministic", func(t *testing.T) {
		t.Parallel()
		results := make(map[time.Duration]bool)
		for i := 0; i < 50; i++ {
			results[bc.Jitter(1*time.Second)] = true
		}
		assert.Greater(t, len(results), 1)
	})
}

func TestBackoffCalculator_Calculate(t *testing.T) {
	t.Parallel()

	bc := &BackoffCalculator{
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Factor:       2.0,
	}

	t.Run("attempt 0 is base + jitter", func(t *testing.T) {
		t.Parallel()
		for i := 0; i < 50; i++ {
			d := bc.Calculate(0)
			assert.GreaterOrEqual(t, d, 1*time.Second)
			assert.LessOrEqual(t, d, 2*time.Second) // base + max jitter (capped at base)
		}
	})

	t.Run("high attempt caps at max + jitter", func(t *testing.T) {
		t.Parallel()
		for i := 0; i < 50; i++ {
			d := bc.Calculate(100)
			assert.GreaterOrEqual(t, d, 30*time.Second)
			assert.LessOrEqual(t, d, 60*time.Second) // max + jitter(max)
		}
	})
}

func TestConsumerBackoff_PreConfigured(t *testing.T) {
	t.Parallel()

	assert.Equal(t, constant.RetryInitialBackoff, ConsumerBackoff.InitialDelay)
	assert.Equal(t, constant.RetryMaxBackoff, ConsumerBackoff.MaxDelay)
	assert.Equal(t, 2.0, ConsumerBackoff.Factor)
}

func TestProducerBackoff_PreConfigured(t *testing.T) {
	t.Parallel()

	assert.Equal(t, constant.ProducerInitialBackoff, ProducerBackoff.InitialDelay)
	assert.Equal(t, constant.ProducerMaxBackoff, ProducerBackoff.MaxDelay)
	assert.Equal(t, constant.ProducerBackoffFactor, ProducerBackoff.Factor)
}
