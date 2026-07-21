// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// TestTenantListenerApp_ShutdownIsConcurrentSafe verifies H2: calling
// Shutdown from many goroutines must not double-close the channel. The old
// select/default idiom was racy — two concurrent callers could both observe
// the open channel and both attempt to close, panicking the second caller.
//
// We construct the struct directly (bypassing newTenantListenerApp) because
// the constructor requires a non-nil *tmevent.TenantEventListener, which is
// irrelevant for testing the Shutdown primitive.
func TestTenantListenerApp_ShutdownIsConcurrentSafe(t *testing.T) {
	t.Parallel()

	app := &tenantListenerApp{
		logger:     testutil.NewMockLogger(),
		shutdownCh: make(chan struct{}),
	}

	const goroutines = 100

	var wg sync.WaitGroup

	wg.Add(goroutines)

	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			<-start
			app.Shutdown()
		}()
	}

	// Release all goroutines at once to maximise the chance of interleaving.
	close(start)
	wg.Wait()

	// Channel must be closed (observable via non-blocking recv returning zero).
	select {
	case <-app.shutdownCh:
		// Success — channel is closed.
	default:
		t.Fatal("shutdownCh must be closed after Shutdown() returns")
	}

	// A subsequent Shutdown call must also be safe.
	assert.NotPanics(t, func() { app.Shutdown() },
		"calling Shutdown a second time must not panic")
}
