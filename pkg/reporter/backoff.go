// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"crypto/rand"
	"math/big"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
)

const backoffDivisor = 2

// BackoffCalculator computes exponential backoff delays with configurable parameters.
// Both consumer and producer retry logic use this instead of hardcoded calculations.
type BackoffCalculator struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Factor       float64
}

// Next multiplies the current delay by Factor, capped at MaxDelay.
func (bc *BackoffCalculator) Next(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * bc.Factor)
	if next > bc.MaxDelay {
		return bc.MaxDelay
	}

	return next
}

// Jitter returns a random duration in [0, baseDelay], capped at MaxDelay.
// Uses crypto/rand for unbiased distribution to prevent thundering herd.
func (bc *BackoffCalculator) Jitter(baseDelay time.Duration) time.Duration {
	if baseDelay <= 0 {
		return 0
	}

	delayCap := baseDelay
	if delayCap > bc.MaxDelay {
		delayCap = bc.MaxDelay
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(delayCap)))
	if err != nil {
		return delayCap / backoffDivisor
	}

	return time.Duration(n.Int64())
}

// Calculate computes the backoff delay for a given attempt number using exponential
// backoff: InitialDelay * Factor^attempt, capped at MaxDelay, plus random jitter.
func (bc *BackoffCalculator) Calculate(attempt int) time.Duration {
	backoff := bc.InitialDelay

	for range attempt {
		backoff = time.Duration(float64(backoff) * bc.Factor)
		if backoff > bc.MaxDelay {
			backoff = bc.MaxDelay

			break
		}
	}

	result := backoff + bc.Jitter(backoff)
	if result > bc.MaxDelay {
		result = bc.MaxDelay
	}

	return result
}

// Pre-configured instances for consumer and producer retry paths.
var (
	// ConsumerBackoff is the backoff calculator for RabbitMQ consumer retries.
	ConsumerBackoff = &BackoffCalculator{
		InitialDelay: constant.RetryInitialBackoff,
		MaxDelay:     constant.RetryMaxBackoff,
		Factor:       2.0,
	}

	// ProducerBackoff is the backoff calculator for RabbitMQ producer retries.
	ProducerBackoff = &BackoffCalculator{
		InitialDelay: constant.ProducerInitialBackoff,
		MaxDelay:     constant.ProducerMaxBackoff,
		Factor:       constant.ProducerBackoffFactor,
	}
)

// FullJitter returns a random duration in [0, baseDelay], capped at ProducerMaxBackoff.
// Full jitter prevents thundering herd when multiple producers reconnect simultaneously
// after a RabbitMQ restart. Uses crypto/rand for unbiased distribution.
//
// Deprecated: Use ProducerBackoff.Jitter(baseDelay) instead.
func FullJitter(baseDelay time.Duration) time.Duration {
	return ProducerBackoff.Jitter(baseDelay)
}

// NextBackoff doubles the current delay, capped at ProducerMaxBackoff.
//
// Deprecated: Use ProducerBackoff.Next(current) instead.
func NextBackoff(current time.Duration) time.Duration {
	return ProducerBackoff.Next(current)
}
