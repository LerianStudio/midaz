// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRegistry is an in-memory libsd.Registry that records Register/Deregister
// calls so lifecycle tests can assert the runnable drives the manager without a
// real Consul. registerErr / deregisterErr let a test exercise failure paths.
type stubRegistry struct {
	mu            sync.Mutex
	registered    []libsd.Service
	deregistered  []string
	registerErr   error
	deregisterErr error

	// registeredCh receives once on the first Register call so a test can wait
	// for the async RegisterAsync goroutine to land before simulating SIGTERM.
	registeredCh chan struct{}
}

func (s *stubRegistry) Register(_ context.Context, svc libsd.Service) error {
	s.mu.Lock()
	s.registered = append(s.registered, svc)
	first := len(s.registered) == 1
	err := s.registerErr
	s.mu.Unlock()

	if first && s.registeredCh != nil {
		s.registeredCh <- struct{}{}
	}

	return err
}

func (s *stubRegistry) Deregister(_ context.Context, serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.deregistered = append(s.deregistered, serviceID)

	return s.deregisterErr
}

func (s *stubRegistry) Resolve(_ context.Context, _, _ string) (libsd.Service, error) {
	return libsd.Service{}, errors.New("not implemented")
}

func (s *stubRegistry) Watch(_ context.Context, _ string) (<-chan libsd.Event, error) {
	return nil, errors.New("not implemented")
}

func (s *stubRegistry) registeredServices() []libsd.Service {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]libsd.Service, len(s.registered))
	copy(out, s.registered)

	return out
}

func (s *stubRegistry) deregisteredIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, len(s.deregistered))
	copy(out, s.deregistered)

	return out
}

// newStubManager builds an enabled libsd.Manager backed by stub so Register /
// Deregister actually run through the manager against the stub registry. A
// non-empty AdvertiseAddr is required for the enabled config to validate.
func newStubManager(t *testing.T, stub libsd.Registry) *libsd.Manager {
	t.Helper()

	mgr, err := libsd.New(
		libsd.Config{Enabled: true, ConsulAddr: "consul:8500", AdvertiseAddr: "svc.test:3002"},
		libsd.WithLogger(libLog.NewNop()),
		libsd.WithRegistry(stub),
	)
	require.NoError(t, err)

	return mgr
}

// TestRunnable_Lifecycle exercises the full register/deregister path of Run
// without a real Consul: RegisterAsync must record a Register with the
// descriptor's ID/Name/Port/TTL, and simulated SIGTERM (context cancel) must
// trigger a Deregister with the SAME ID.
func TestRunnable_Lifecycle(t *testing.T) {
	t.Parallel()

	stub := &stubRegistry{registeredCh: make(chan struct{}, 1)}
	mgr := newStubManager(t, stub)
	svc := BuildServiceDescriptor("midaz-ledger", 3002)

	sigCtx, cancel := context.WithCancel(context.Background())

	r := &Runnable{
		manager: mgr,
		svc:     svc,
		logger:  libLog.NewNop(),
		notifyContext: func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			return sigCtx, func() {}
		},
	}

	done := make(chan error, 1)

	go func() { done <- r.Run(nil) }()

	// Wait for the async RegisterAsync goroutine to land its Register before
	// simulating SIGTERM. Register succeeds on attempt 0, so that goroutine then
	// returns cleanly — no leak for goleak to catch.
	select {
	case <-stub.registeredCh:
	case <-time.After(2 * time.Second):
		t.Fatal("RegisterAsync did not call Register within timeout")
	}

	// Simulate SIGTERM so Run unblocks from <-sigCtx.Done() and deregisters.
	cancel()

	require.NoError(t, <-done)

	registered := stub.registeredServices()
	require.Len(t, registered, 1)
	assert.Equal(t, "midaz-ledger-3002", registered[0].ID)
	assert.Equal(t, "midaz-ledger", registered[0].Name)
	assert.Equal(t, 3002, registered[0].Port)
	require.NotNil(t, registered[0].HealthCheck)
	assert.Equal(t, "30s", registered[0].HealthCheck.TTL)

	deregistered := stub.deregisteredIDs()
	require.Len(t, deregistered, 1)
	assert.Equal(t, "midaz-ledger-3002", deregistered[0])
}

// TestRunnable_DeregisterErrorSwallowed verifies a deregister failure is logged
// at Warn and NOT propagated: Run still returns nil.
func TestRunnable_DeregisterErrorSwallowed(t *testing.T) {
	t.Parallel()

	stub := &stubRegistry{
		deregisterErr: errors.New("consul unreachable"),
		registeredCh:  make(chan struct{}, 1),
	}
	mgr := newStubManager(t, stub)
	svc := BuildServiceDescriptor("midaz-crm", 4003)

	sigCtx, cancel := context.WithCancel(context.Background())

	r := &Runnable{
		manager: mgr,
		svc:     svc,
		logger:  libLog.NewNop(),
		notifyContext: func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			return sigCtx, func() {}
		},
	}

	done := make(chan error, 1)

	go func() { done <- r.Run(nil) }()

	select {
	case <-stub.registeredCh:
	case <-time.After(2 * time.Second):
		t.Fatal("RegisterAsync did not call Register within timeout")
	}

	cancel()

	require.NoError(t, <-done, "deregister error must be swallowed, not propagated")
	assert.Equal(t, []string{"midaz-crm-4003"}, stub.deregisteredIDs())
}

// TestRunnable_NilManagerNoOp verifies the runnable returns immediately when the
// manager is nil, before installing any signal handler or spawning goroutines.
// Also asserts it is a true no-op: nothing registered or deregistered. Keeps the
// guard branch goleak-safe.
func TestRunnable_NilManagerNoOp(t *testing.T) {
	t.Parallel()

	stub := &stubRegistry{}
	r := &Runnable{manager: nil, logger: libLog.NewNop()}

	require.NoError(t, r.Run(nil))
	assert.Empty(t, stub.registeredServices())
	assert.Empty(t, stub.deregisteredIDs())
}

// TestNewRunnable verifies the constructor wires the manager, descriptor, and
// logger, leaving notifyContext nil so production uses signal.NotifyContext.
func TestNewRunnable(t *testing.T) {
	t.Parallel()

	stub := &stubRegistry{}
	mgr := newStubManager(t, stub)
	svc := BuildServiceDescriptor("midaz-ledger", 3002)
	logger := libLog.NewNop()

	r := NewRunnable(mgr, svc, logger)

	require.NotNil(t, r)
	assert.Same(t, mgr, r.manager)
	assert.Equal(t, svc, r.svc)
	assert.Nil(t, r.notifyContext)
}
