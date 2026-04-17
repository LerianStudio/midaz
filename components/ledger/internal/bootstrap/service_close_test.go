// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/onboarding"
	"github.com/LerianStudio/midaz/v3/components/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// closableStubService extends StubService with a Close() method that records
// invocation and can be configured to return a specific error.
type closableStubService struct {
	StubService
	closeCalls int
	closeErr   error
}

func (s *closableStubService) Close() error {
	s.closeCalls++
	return s.closeErr
}

// Ensure closableStubService still satisfies onboarding.OnboardingService.
var _ onboarding.OnboardingService = (*closableStubService)(nil)

// closableStubTransactionService extends StubTransactionService with a
// Close() method.
type closableStubTransactionService struct {
	StubTransactionService
	closeCalls int
	closeErr   error
}

func (s *closableStubTransactionService) Close() error {
	s.closeCalls++
	return s.closeErr
}

var _ transaction.TransactionService = (*closableStubTransactionService)(nil)

// TestService_GetRunnables_FiltersTransactionFiberServer verifies the
// documented contract: GetRunnables returns the unified HTTP server plus
// all non-HTTP runnables from transaction, with the standalone "Transaction
// Fiber Server" stripped out (it is replaced by UnifiedServer).
func TestService_GetRunnables_FiltersTransactionFiberServer(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	txRunnables := []mbootstrap.RunnableConfig{
		{Name: "Transaction Fiber Server", Runnable: &StubRunnable{name: "tx-fiber"}},
		{Name: "Transaction Broker Consumer", Runnable: &StubRunnable{name: "tx-broker"}},
		{Name: "Transaction Redis Consumer", Runnable: &StubRunnable{name: "tx-redis"}},
	}

	svc := &Service{
		OnboardingService:  &StubService{},
		TransactionService: &StubTransactionService{runnables: txRunnables},
		UnifiedServer:      &UnifiedServer{serverAddress: ":0"},
		Logger:             logger,
		Telemetry:          &libOpentelemetry.Telemetry{},
	}

	got := svc.GetRunnables()

	// Expect 3 entries: UnifiedServer + 2 non-Fiber transaction runnables.
	require.Len(t, got, 3)
	assert.Equal(t, "Unified HTTP Server", got[0].Name)
	assert.Same(t, svc.UnifiedServer, got[0].Runnable)
	assert.Equal(t, "Transaction Broker Consumer", got[1].Name)
	assert.Equal(t, "Transaction Redis Consumer", got[2].Name)

	// Prove the Fiber server was explicitly excluded.
	for _, r := range got {
		assert.NotEqual(t, "Transaction Fiber Server", r.Name,
			"GetRunnables must filter out the standalone transaction Fiber server")
	}
}

// TestService_GetRunnables_NoTransactionRunnables verifies the edge case
// where the transaction service reports zero runnables (purely-HTTP deploy).
func TestService_GetRunnables_NoTransactionRunnables(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	svc := &Service{
		OnboardingService:  &StubService{},
		TransactionService: &StubTransactionService{runnables: nil},
		UnifiedServer:      &UnifiedServer{serverAddress: ":0"},
		Logger:             logger,
	}

	got := svc.GetRunnables()

	require.Len(t, got, 1)
	assert.Equal(t, "Unified HTTP Server", got[0].Name)
}

// TestService_Close_InvokesBothServiceClosers verifies that Close fan-outs
// to both onboarding and transaction services exactly once and aggregates
// errors via errors.Join.
func TestService_Close_InvokesBothServiceClosers(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	onboardingStub := &closableStubService{}
	transactionStub := &closableStubTransactionService{}

	svc := &Service{
		OnboardingService:  onboardingStub,
		TransactionService: transactionStub,
		Logger:             logger,
		// Deliberately no Telemetry — Close must tolerate a nil Telemetry
		// pointer. See service.go:Close.
		Telemetry: nil,
	}

	require.NoError(t, svc.Close())
	assert.Equal(t, 1, onboardingStub.closeCalls)
	assert.Equal(t, 1, transactionStub.closeCalls)
}

// TestService_Close_IsIdempotent verifies that subsequent Close() calls
// are no-ops — the underlying services must be closed exactly once.
func TestService_Close_IsIdempotent(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	onboardingStub := &closableStubService{}
	transactionStub := &closableStubTransactionService{}

	svc := &Service{
		OnboardingService:  onboardingStub,
		TransactionService: transactionStub,
		Logger:             logger,
	}

	require.NoError(t, svc.Close())
	require.NoError(t, svc.Close())
	require.NoError(t, svc.Close())

	assert.Equal(t, 1, onboardingStub.closeCalls)
	assert.Equal(t, 1, transactionStub.closeCalls)
}

// TestService_Close_AggregatesErrors verifies errors.Join behaviour when
// both services fail to close.
func TestService_Close_AggregatesErrors(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	onboardingErr := errTestOnboardingClose
	transactionErr := errTestTransactionClose

	onboardingStub := &closableStubService{closeErr: onboardingErr}
	transactionStub := &closableStubTransactionService{closeErr: transactionErr}

	svc := &Service{
		OnboardingService:  onboardingStub,
		TransactionService: transactionStub,
		Logger:             logger,
	}

	closeErr := svc.Close()
	require.Error(t, closeErr)
	require.ErrorIs(t, closeErr, onboardingErr)
	require.ErrorIs(t, closeErr, transactionErr)

	// Subsequent calls must return the same error without re-invoking closers.
	assert.Equal(t, closeErr, svc.Close())
	assert.Equal(t, 1, onboardingStub.closeCalls)
	assert.Equal(t, 1, transactionStub.closeCalls)
}

// TestService_Close_NilReceiverSafe verifies the nil-guard at the top of
// Close. Calling Close on a nil Service must return nil rather than panic.
func TestService_Close_NilReceiverSafe(t *testing.T) {
	t.Parallel()

	var svc *Service

	require.NoError(t, svc.Close())
}

// TestService_Close_NonClosableServicesNoOp verifies that when the embedded
// services do not implement io.Closer (the stub path), Close returns nil
// and does not panic.
func TestService_Close_NonClosableServicesNoOp(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	svc := &Service{
		OnboardingService:  &StubService{},
		TransactionService: &StubTransactionService{},
		Logger:             logger,
	}

	require.NoError(t, svc.Close())
}

// TestService_Run_AssemblesLauncherOptions is a structural test: we cannot
// cleanly exercise the real s.Run() because it invokes launcher.Run()
// which blocks on signal handling. Instead we reach into the branches Run
// relies on — GetRunnables and the fiber app access — to guarantee they
// behave as expected when a minimally-wired Service is handed a real
// UnifiedServer. This exercises the non-Run code paths that Run itself
// depends on without starting an actual HTTP listener.
func TestService_Run_AssemblesLauncherOptions(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	unified := NewUnifiedServer(":0", logger, newTestTelemetry())
	require.NotNil(t, unified)

	// Sanity: the unified server's internal app is a *fiber.App.
	require.IsType(t, &fiber.App{}, unified.app)

	svc := &Service{
		OnboardingService:  &StubService{},
		TransactionService: &StubTransactionService{},
		UnifiedServer:      unified,
		Logger:             logger,
	}

	got := svc.GetRunnables()
	require.NotEmpty(t, got)
	assert.Same(t, unified, got[0].Runnable)
}
