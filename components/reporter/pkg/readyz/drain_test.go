// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDrainState_DefaultIsNotDraining verifies a freshly-created DrainState
// reports IsDraining()=false until StartDraining() is called.
func TestDrainState_DefaultIsNotDraining(t *testing.T) {
	t.Parallel()

	d := &DrainState{}
	assert.False(t, d.IsDraining(), "fresh DrainState must not be draining")
}

// TestDrainState_StartDrainingFlipsFlag verifies StartDraining() sets the
// flag to true and IsDraining() observes the change.
func TestDrainState_StartDrainingFlipsFlag(t *testing.T) {
	t.Parallel()

	d := &DrainState{}
	d.StartDraining()
	assert.True(t, d.IsDraining(), "StartDraining must flip the flag")
}

// TestDrainState_IdempotentStart verifies StartDraining() can be called
// multiple times without error.
func TestDrainState_IdempotentStart(t *testing.T) {
	t.Parallel()

	d := &DrainState{}
	d.StartDraining()
	d.StartDraining()
	d.StartDraining()
	assert.True(t, d.IsDraining())
}

// TestDrainState_RaceFreeReadWrite spawns concurrent readers and a single
// writer to verify the atomic semantics of StartDraining/IsDraining. Run
// with -race to catch data races.
func TestDrainState_RaceFreeReadWrite(t *testing.T) {
	t.Parallel()

	d := &DrainState{}
	wg := sync.WaitGroup{}

	const readers = 50

	for i := 0; i < readers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			_ = d.IsDraining()
		}()
	}

	wg.Add(1)

	go func() {
		defer wg.Done()
		d.StartDraining()
	}()

	wg.Wait()
	assert.True(t, d.IsDraining())
}
