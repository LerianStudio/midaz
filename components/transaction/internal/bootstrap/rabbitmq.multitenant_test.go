// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// goleakIgnores returns common goleak ignore options for this package.
// These goroutines are from test infrastructure and external dependencies,
// not from the code under test.
func goleakIgnores() []goleak.Option {
	return []goleak.Option{
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreAnyFunction("testing.tRunner"),
		goleak.IgnoreAnyFunction("testing.tRunner.func1"),
		goleak.IgnoreAnyFunction("testing.(*T).Run"),
		goleak.IgnoreAnyFunction("testing.runTests"),
		goleak.IgnoreAnyFunction("testing.(*M).Run"),
		goleak.IgnoreAnyFunction("go.uber.org/goleak.(*opts).retry"),
		goleak.IgnoreAnyFunction("go.uber.org/goleak.Find"),
	}
}

// TestMultiTenantConsumerRunnable_NilConsumer_NoLeak verifies that calling Run()
// with a nil consumer returns immediately without leaking goroutines.
// The signal.NotifyContext goroutine should NOT be created when consumer is nil.
func TestMultiTenantConsumerRunnable_NilConsumer_NoLeak(t *testing.T) {
	t.Parallel()

	// Create runnable with nil consumer
	runnable := &multiTenantConsumerRunnable{
		consumer: nil,
	}

	// Run should return immediately with nil error
	err := runnable.Run(nil)
	require.NoError(t, err, "Run() with nil consumer should return nil error")

	// Verify no goroutine leaks (ignoring test infrastructure goroutines)
	goleak.VerifyNone(t, goleakIgnores()...)
}

// TestMultiTenantConsumerRunnable_ZeroValue_NoLeak verifies that a zero-value
// multiTenantConsumerRunnable struct doesn't leak goroutines when Run() is called.
func TestMultiTenantConsumerRunnable_ZeroValue_NoLeak(t *testing.T) {
	t.Parallel()

	// Zero-value struct (consumer is nil by default)
	runnable := &multiTenantConsumerRunnable{}

	// Run should return immediately with nil error
	err := runnable.Run(&libCommons.Launcher{})
	require.NoError(t, err, "Run() on zero-value struct should return nil error")

	// Verify no goroutine leaks (ignoring test infrastructure goroutines)
	goleak.VerifyNone(t, goleakIgnores()...)
}

// TestMultiTenantConsumerRunnable_RunReturnsNilForNilConsumer verifies that
// Run() returns nil when the consumer field is nil. This is the early-exit path.
func TestMultiTenantConsumerRunnable_RunReturnsNilForNilConsumer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		runnable *multiTenantConsumerRunnable
		launcher *libCommons.Launcher
	}{
		{
			name: "nil_consumer_nil_launcher",
			runnable: &multiTenantConsumerRunnable{
				consumer: nil,
			},
			launcher: nil,
		},
		{
			name: "nil_consumer_non_nil_launcher",
			runnable: &multiTenantConsumerRunnable{
				consumer: nil,
			},
			launcher: &libCommons.Launcher{},
		},
		{
			name:     "zero_value_struct_nil_launcher",
			runnable: &multiTenantConsumerRunnable{},
			launcher: nil,
		},
		{
			name:     "zero_value_struct_non_nil_launcher",
			runnable: &multiTenantConsumerRunnable{},
			launcher: &libCommons.Launcher{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.runnable.Run(tt.launcher)

			assert.NoError(t, err, "Run() should return nil when consumer is nil")
		})
	}

	// Verify no goroutine leaks after all subtests (ignoring test infrastructure)
	goleak.VerifyNone(t, goleakIgnores()...)
}

// TestMultiTenantConsumerRunnable_StructFieldsAccessible verifies that the
// multiTenantConsumerRunnable struct exposes the expected field for consumer.
func TestMultiTenantConsumerRunnable_StructFieldsAccessible(t *testing.T) {
	t.Parallel()

	runnable := &multiTenantConsumerRunnable{}

	// Verify zero-value
	assert.Nil(t, runnable.consumer, "consumer should be nil for zero-value struct")

	// Verify field can be set
	runnable.consumer = nil // Explicitly set to nil (same as zero-value)
	assert.Nil(t, runnable.consumer, "consumer field should be settable")

	// Verify no goroutine leaks (ignoring test infrastructure goroutines)
	goleak.VerifyNone(t, goleakIgnores()...)
}
