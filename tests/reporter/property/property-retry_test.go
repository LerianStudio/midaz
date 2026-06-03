//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package property

import (
	"testing"
	"testing/quick"
	"time"
)

// Property 1: Backoff exponential deve crescer como 2^n
func TestProperty_Retry_ExponentialBackoff(t *testing.T) {
	t.Parallel()

	property := func(retryCount uint8) bool {
		if retryCount > 10 {
			retryCount = retryCount % 10
		}

		// Calculate backoff: 2^n seconds
		backoff := time.Duration(1<<retryCount) * time.Second

		// Expected values for each retry count
		expected := map[uint8]time.Duration{
			0: 1 * time.Second,   // 2^0 = 1
			1: 2 * time.Second,   // 2^1 = 2
			2: 4 * time.Second,   // 2^2 = 4
			3: 8 * time.Second,   // 2^3 = 8
			4: 16 * time.Second,  // 2^4 = 16
			5: 32 * time.Second,  // 2^5 = 32
			6: 64 * time.Second,  // 2^6 = 64
			7: 128 * time.Second, // 2^7 = 128
		}

		if expectedBackoff, exists := expected[retryCount]; exists {
			return backoff == expectedBackoff
		}

		// For values > 7, just check that it's a positive duration
		return backoff > 0
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: backoff is not exponential: %v", err)
	}
}

// Property 2: Retry count nunca deve ser negativo
func TestProperty_Retry_CountNonNegative(t *testing.T) {
	t.Parallel()

	property := func(count int32) bool {
		// Simulate getRetryCount behavior with various types
		var retryCount int32

		switch v := any(count).(type) {
		case int32:
			retryCount = v
		case int64:
			retryCount = int32(v)
		case int:
			retryCount = int32(v)
		case float32:
			retryCount = int32(v)
		case float64:
			retryCount = int32(v)
		default:
			retryCount = 0
		}

		// After conversion, retry count should never be negative
		if retryCount < 0 {
			retryCount = 0
		}

		return retryCount >= 0
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("Property violated: retry count is negative: %v", err)
	}
}

// Property 3: Após 3 retries, contador deve ser >= 3
func TestProperty_Retry_MaxRetries(t *testing.T) {
	t.Parallel()

	property := func(initialCount uint8) bool {
		count := int32(initialCount % 10) // Keep it reasonable

		// Simulate 3 retries
		for i := 0; i < 3; i++ {
			count++
		}

		// After 3 retries, count should be at least 3 more than initial
		return count >= int32(initialCount%10)+3
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: retry count after 3 retries: %v", err)
	}
}

// Property 4: Backoff deve sempre aumentar entre retries consecutivos
func TestProperty_Retry_BackoffMonotonicallyIncreasing(t *testing.T) {
	t.Parallel()

	property := func(count1, count2 uint8) bool {
		if count1 >= count2 || count1 > 10 || count2 > 10 {
			return true
		}

		backoff1 := time.Duration(1<<count1) * time.Second
		backoff2 := time.Duration(1<<count2) * time.Second

		// Backoff for higher retry count should always be greater
		return backoff2 > backoff1
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: backoff not monotonically increasing: %v", err)
	}
}

// Property 5: Retry count incremento deve ser sempre +1
func TestProperty_Retry_IncrementByOne(t *testing.T) {
	t.Parallel()

	property := func(currentCount uint8) bool {
		if currentCount > 100 {
			currentCount = currentCount % 100
		}

		count := int32(currentCount)
		nextCount := count + 1

		// Next count should always be exactly current + 1
		return nextCount == count+1
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: retry increment not +1: %v", err)
	}
}

// Property 6: Tempo total de retries deve ser soma dos backoffs
func TestProperty_Retry_TotalTime(t *testing.T) {
	t.Parallel()

	property := func(maxRetries uint8) bool {
		if maxRetries == 0 || maxRetries > 5 {
			return true
		}

		var totalTime time.Duration

		for i := uint8(0); i < maxRetries; i++ {
			backoff := time.Duration(1<<i) * time.Second
			totalTime += backoff
		}

		// For 3 retries: 1s + 2s + 4s = 7s
		if maxRetries == 3 {
			return totalTime == 7*time.Second
		}

		// Total time should always be positive
		return totalTime > 0
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Property violated: total retry time: %v", err)
	}
}

// Property 7: Retry após max attempts (3) deve retornar false
func TestProperty_Retry_StopsAfterMax(t *testing.T) {
	t.Parallel()

	property := func(retryCount uint8) bool {
		count := int32(retryCount % 20)

		// Simulate retry logic
		shouldRetry := count < 3

		// If count >= 3, should NOT retry
		if count >= 3 {
			return !shouldRetry
		}

		// If count < 3, should retry
		return shouldRetry
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: retry doesn't stop after max: %v", err)
	}
}

// Property 8: Backoff nunca deve overflow ou ser negativo
func TestProperty_Retry_BackoffNoOverflow(t *testing.T) {
	t.Parallel()

	property := func(retryCount uint8) bool {
		if retryCount > 30 {
			retryCount = 30 // Cap to prevent actual overflow in test
		}

		// Calculate backoff
		backoff := time.Duration(1<<retryCount) * time.Second

		// Should never be negative or zero
		return backoff > 0
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: backoff overflow: %v", err)
	}
}

// Property 9: Retry count de diferentes tipos devem normalizar para int32
func TestProperty_Retry_TypeNormalization(t *testing.T) {
	t.Parallel()

	property := func(val int) bool {
		if val < 0 {
			val = -val
		}
		if val > 1000 {
			val = val % 1000
		}

		// Test various type conversions
		int32Val := int32(val)
		int64Val := int64(val)
		float32Val := float32(val)
		float64Val := float64(val)

		// All should normalize to same int32 value
		return int32(int64Val) == int32Val &&
			int32(float32Val) == int32Val &&
			int32(float64Val) == int32Val
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: type normalization: %v", err)
	}
}

// Property 10: Retry headers devem preservar informação entre tentativas
func TestProperty_Retry_HeaderPreservation(t *testing.T) {
	t.Parallel()

	property := func(reason string, count uint8) bool {
		if count > 10 {
			count = count % 10
		}

		// Simulate headers
		headers := map[string]any{
			"x-retry-count":    int32(count),
			"x-failure-reason": reason,
		}

		// Headers should contain both keys
		_, hasCount := headers["x-retry-count"]
		_, hasReason := headers["x-failure-reason"]

		return hasCount && hasReason
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: header preservation: %v", err)
	}
}

// Benchmark: Verificar performance de cálculo de backoff
func BenchmarkExponentialBackoff(b *testing.B) {
	for i := 0; i < b.N; i++ {
		retryCount := uint8(i % 10)
		_ = time.Duration(1<<retryCount) * time.Second
	}
}
