// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// blockingRunnable returns from Run only when its done channel is closed.
// It tracks invocation count so tests can assert each runnable was started.
type blockingRunnable struct {
	started  int32
	finished int32
	done     <-chan struct{}
}

func (b *blockingRunnable) Run(_ *libCommons.Launcher) error {
	atomic.AddInt32(&b.started, 1)

	<-b.done
	atomic.AddInt32(&b.finished, 1)

	return nil
}

// TestService_Run_DispatchesRunnablesAndClosesOnShutdown exercises Service.Run
// end-to-end using stubs. It verifies that:
//   - Run composes the unified HTTP server + transaction runnables (excluding
//     the standalone "Transaction Fiber Server") into a Launcher.
//   - All registered runnables are started in parallel goroutines.
//   - On SIGTERM the UnifiedServer and custom runnables unblock, the launcher
//     returns, and Service.Close is invoked (idempotent).
//
// No t.Parallel(): sends a process-wide SIGTERM.
func TestService_Run_DispatchesRunnablesAndClosesOnShutdown(t *testing.T) {
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	telemetry, err := libOpentelemetry.InitializeTelemetryWithError(&libOpentelemetry.TelemetryConfig{
		LibraryName:     "test-service-run",
		EnableTelemetry: false,
		Logger:          logger,
	})
	require.NoError(t, err)

	// done is closed when we send SIGTERM. All custom blocking runnables
	// release together.
	done := make(chan struct{})

	broker := &blockingRunnable{done: done}
	redis := &blockingRunnable{done: done}

	// Stub transaction service exposes a mix of runnables:
	//   - "Transaction Fiber Server" (must be filtered out by Service.Run)
	//   - "Transaction Broker Consumer", "Transaction Redis Consumer" (kept)
	fiberStub := &blockingRunnable{done: done} // should NOT be invoked
	stubTx := &closableStubTransactionService{
		StubTransactionService: StubTransactionService{
			runnables: []mbootstrap.RunnableConfig{
				{Name: "Transaction Fiber Server", Runnable: fiberStub},
				{Name: "Transaction Broker Consumer", Runnable: broker},
				{Name: "Transaction Redis Consumer", Runnable: redis},
			},
		},
	}
	stubOnb := &closableStubService{}

	unified := NewUnifiedServer("127.0.0.1:0", logger, telemetry)
	require.NotNil(t, unified)

	svc := &Service{
		OnboardingService:  stubOnb,
		TransactionService: stubTx,
		UnifiedServer:      unified,
		Logger:             logger,
		Telemetry:          telemetry,
	}

	runReturned := make(chan struct{})

	go func() {
		svc.Run()
		close(runReturned)
	}()

	// Give the launcher goroutines time to start every runnable and install
	// their signal handlers.
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&broker.started) == 1 &&
			atomic.LoadInt32(&redis.started) == 1
	}, 3*time.Second, 25*time.Millisecond, "runnables did not start")

	assert.Equal(t, int32(0), atomic.LoadInt32(&fiberStub.started),
		"Transaction Fiber Server must be filtered out by Service.Run")

	// Trigger shutdown: SIGTERM unblocks the UnifiedServer, and we manually
	// release the custom runnables so the launcher WaitGroup can drain.
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
	close(done)

	select {
	case <-runReturned:
		// Service.Run returned — Close was called.
		assert.Equal(t, 1, stubOnb.closeCalls, "onboarding.Close should run once")
		assert.Equal(t, 1, stubTx.closeCalls, "transaction.Close should run once")
	case <-time.After(10 * time.Second):
		t.Fatal("Service.Run did not return within 10s of SIGTERM")
	}

	assert.Equal(t, int32(1), atomic.LoadInt32(&broker.finished))
	assert.Equal(t, int32(1), atomic.LoadInt32(&redis.finished))
}
