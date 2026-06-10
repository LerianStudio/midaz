// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"sync"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCleanupManager(t *testing.T) {
	t.Parallel()

	logger := log.NewNop()
	cm := NewCleanupManager(logger)

	require.NotNil(t, cm)
	assert.Equal(t, 0, cm.Len())
}

func TestCleanupManager_Register_IncreasesLen(t *testing.T) {
	t.Parallel()

	cm := NewCleanupManager(log.NewNop())

	cm.Register("first", func() {})
	assert.Equal(t, 1, cm.Len())

	cm.Register("second", func() {})
	assert.Equal(t, 2, cm.Len())

	cm.Register("third", func() {})
	assert.Equal(t, 3, cm.Len())
}

func TestCleanupManager_ExecuteAll_RunsInReverseOrder(t *testing.T) {
	t.Parallel()

	cm := NewCleanupManager(log.NewNop())

	var order []string

	cm.Register("first", func() { order = append(order, "first") })
	cm.Register("second", func() { order = append(order, "second") })
	cm.Register("third", func() { order = append(order, "third") })

	cm.ExecuteAll()

	require.Len(t, order, 3)
	assert.Equal(t, []string{"third", "second", "first"}, order,
		"cleanup functions must execute in LIFO (reverse) order")
}

func TestCleanupManager_ExecuteAll_DrainsStack(t *testing.T) {
	t.Parallel()

	cm := NewCleanupManager(log.NewNop())

	cm.Register("resource", func() {})
	assert.Equal(t, 1, cm.Len())

	cm.ExecuteAll()
	assert.Equal(t, 0, cm.Len(), "stack must be empty after ExecuteAll")
}

func TestCleanupManager_ExecuteAll_SafeToCallMultipleTimes(t *testing.T) {
	t.Parallel()

	cm := NewCleanupManager(log.NewNop())

	callCount := 0
	cm.Register("once", func() { callCount++ })

	cm.ExecuteAll()
	cm.ExecuteAll()

	assert.Equal(t, 1, callCount,
		"cleanup function must only run once even if ExecuteAll is called multiple times")
}

func TestCleanupManager_ExecuteAll_EmptyStackDoesNotPanic(t *testing.T) {
	t.Parallel()

	cm := NewCleanupManager(log.NewNop())

	assert.NotPanics(t, func() {
		cm.ExecuteAll()
	})
}

func TestCleanupManager_Len_ReturnsZeroForNewManager(t *testing.T) {
	t.Parallel()

	cm := NewCleanupManager(log.NewNop())
	assert.Equal(t, 0, cm.Len())
}

func TestCleanupManager_ConcurrentRegisterAndLen(t *testing.T) {
	t.Parallel()

	cm := NewCleanupManager(log.NewNop())

	const goroutines = 50
	var wg sync.WaitGroup

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			cm.Register("concurrent", func() {})
		}()
	}

	wg.Wait()

	assert.Equal(t, goroutines, cm.Len(),
		"all concurrent Register calls must be recorded")
}

func TestCleanupManager_RegisterAfterExecuteAll(t *testing.T) {
	t.Parallel()

	cm := NewCleanupManager(log.NewNop())

	cm.Register("before", func() {})
	cm.ExecuteAll()
	assert.Equal(t, 0, cm.Len())

	// Register new cleanup after draining — the manager must accept it.
	cm.Register("after", func() {})
	assert.Equal(t, 1, cm.Len())

	called := false
	cm.Register("after2", func() { called = true })
	cm.ExecuteAll()
	assert.True(t, called, "newly registered cleanup must execute on second ExecuteAll")
}
