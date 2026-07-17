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
	"go.uber.org/goleak"
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

	// events records the order of "deregister" and "close" calls so a test can
	// assert Manager.Close runs after Deregister. Manager.Close delegates to the
	// registry's optional Close() seam, so a Close entry here proves Run closed
	// the manager.
	events []string

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
	s.events = append(s.events, "deregister")

	return s.deregisterErr
}

// Close satisfies the optional Close() seam that libsd.Manager.Close delegates
// to, so a test can observe that Run closed the manager and its ordering
// relative to Deregister.
func (s *stubRegistry) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, "close")

	return nil
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

func (s *stubRegistry) orderedEvents() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, len(s.events))
	copy(out, s.events)

	return out
}

func (s *stubRegistry) closeCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for _, e := range s.events {
		if e == "close" {
			n++
		}
	}

	return n
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
// trigger a Deregister with the SAME ID. It also asserts the metrics recorder
// observes exactly one register-initiated and one deregister-OK over the cycle.
func TestRunnable_Lifecycle(t *testing.T) {
	t.Parallel()

	stub := &stubRegistry{registeredCh: make(chan struct{}, 1)}
	mgr := newStubManager(t, stub)
	svc := BuildServiceDescriptor("midaz-ledger", 3002)
	recorder := &stubRecorder{}

	sigCtx, cancel := context.WithCancel(context.Background())

	r := &Runnable{
		manager: mgr,
		svc:     svc,
		logger:  libLog.NewNop(),
		metrics: recorder,
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
	// Assert against the descriptor the test built, not a hardcoded ID: the
	// instance ID embeds os.Hostname() and is not deterministic across hosts.
	assert.Equal(t, svc.ID, registered[0].ID)
	assert.Equal(t, "midaz-ledger", registered[0].Name)
	assert.Equal(t, 3002, registered[0].Port)
	require.NotNil(t, registered[0].HealthCheck)
	assert.Equal(t, "30s", registered[0].HealthCheck.TTL)

	deregistered := stub.deregisteredIDs()
	require.Len(t, deregistered, 1)
	assert.Equal(t, svc.ID, deregistered[0])

	assert.Equal(t, 1, recorder.registerInitiatedCalls, "exactly one register-initiated over the cycle")
	assert.Equal(t, []string{ResultOK}, recorder.deregisterResults, "clean shutdown records deregister OK")
}

// TestRunnable_DeregisterErrorSwallowed verifies a deregister failure is logged
// at Warn and NOT propagated: Run still returns nil. The failure path must also
// record a single deregister-error metric.
func TestRunnable_DeregisterErrorSwallowed(t *testing.T) {
	t.Parallel()

	stub := &stubRegistry{
		deregisterErr: errors.New("consul unreachable"),
		registeredCh:  make(chan struct{}, 1),
	}
	mgr := newStubManager(t, stub)
	svc := BuildServiceDescriptor("midaz-crm", 4003)
	recorder := &stubRecorder{}

	sigCtx, cancel := context.WithCancel(context.Background())

	r := &Runnable{
		manager: mgr,
		svc:     svc,
		logger:  libLog.NewNop(),
		metrics: recorder,
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
	assert.Equal(t, []string{svc.ID}, stub.deregisteredIDs())

	assert.Equal(t, 1, recorder.registerInitiatedCalls, "exactly one register-initiated over the cycle")
	assert.Equal(t, []string{ResultError}, recorder.deregisterResults, "deregister failure records error result")
}

// TestRunnable_NilRecorderNoOp verifies passing a nil recorder to NewRunnable
// does not panic across a full register/deregister lifecycle: orNop must
// substitute the no-op recorder at construction.
func TestRunnable_NilRecorderNoOp(t *testing.T) {
	t.Parallel()

	stub := &stubRegistry{registeredCh: make(chan struct{}, 1)}
	mgr := newStubManager(t, stub)
	svc := BuildServiceDescriptor("midaz-ledger", 3002)

	r := NewRunnable(mgr, svc, libLog.NewNop(), nil)

	sigCtx, cancel := context.WithCancel(context.Background())
	r.notifyContext = func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
		return sigCtx, func() {}
	}

	done := make(chan error, 1)

	go func() { done <- r.Run(nil) }()

	select {
	case <-stub.registeredCh:
	case <-time.After(2 * time.Second):
		t.Fatal("RegisterAsync did not call Register within timeout")
	}

	cancel()

	require.NoError(t, <-done, "nil recorder must be safe across the full lifecycle")
	assert.Equal(t, []string{svc.ID}, stub.deregisteredIDs())
}

// TestRunnable_NilManagerNoOp verifies the runnable returns immediately when the
// manager is nil, before installing any signal handler or spawning goroutines.
// nil manager -> Run returns nil, a true no-op. Keeps the guard branch
// goleak-safe.
func TestRunnable_NilManagerNoOp(t *testing.T) {
	t.Parallel()

	r := &Runnable{manager: nil, logger: libLog.NewNop()}

	require.NoError(t, r.Run(nil))
}

// TestRunnable_ClosesManagerAfterDeregister proves the graceful-shutdown path
// closes the manager exactly once and only after deregistering (deregister
// before close). goleak guards that the RegisterAsync goroutine is torn down and
// no goroutine survives Run. This test never calls Resolve, so no watcher is
// spawned; the boot-time watcher teardown is covered in boot_closer_test.go.
func TestRunnable_ClosesManagerAfterDeregister(t *testing.T) {
	defer goleak.VerifyNone(t)

	stub := &stubRegistry{registeredCh: make(chan struct{}, 1)}
	mgr := newStubManager(t, stub)
	svc := BuildServiceDescriptor("midaz-ledger", 3002)
	recorder := &stubRecorder{}

	// An already-cancelled signal context simulates SIGTERM: Run registers, then
	// immediately unblocks from <-sigCtx.Done(), deregisters, and closes.
	sigCtx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &Runnable{
		manager: mgr,
		svc:     svc,
		logger:  libLog.NewNop(),
		metrics: recorder,
		notifyContext: func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			return sigCtx, func() {}
		},
	}

	require.NoError(t, r.Run(nil))

	assert.Equal(t, 1, stub.closeCalls(), "manager must be closed exactly once")
	assert.Equal(t, []string{"deregister", "close"}, stub.orderedEvents(),
		"Close must run after Deregister")
}

// TestRunnable_ClosesManagerEvenWhenDeregisterFails proves a deregister error
// does not skip the manager Close: the leak-fix must still run on the error
// branch. The close error path is exercised via the logger being non-nil.
func TestRunnable_ClosesManagerEvenWhenDeregisterFails(t *testing.T) {
	defer goleak.VerifyNone(t)

	stub := &stubRegistry{
		deregisterErr: errors.New("consul unreachable"),
		registeredCh:  make(chan struct{}, 1),
	}
	mgr := newStubManager(t, stub)
	svc := BuildServiceDescriptor("midaz-crm", 4003)
	recorder := &stubRecorder{}

	sigCtx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &Runnable{
		manager: mgr,
		svc:     svc,
		logger:  libLog.NewNop(),
		metrics: recorder,
		notifyContext: func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			return sigCtx, func() {}
		},
	}

	require.NoError(t, r.Run(nil), "deregister error must not be propagated")

	assert.Equal(t, 1, stub.closeCalls(), "Close must run even when Deregister fails")
	assert.Equal(t, []string{"deregister", "close"}, stub.orderedEvents(),
		"Close must run after Deregister on the error branch")
}

// TestRunnable_NilManagerDoesNotClose confirms the nil-manager guard returns
// before any Close, so nothing is closed and no goroutine is spawned.
func TestRunnable_NilManagerDoesNotClose(t *testing.T) {
	defer goleak.VerifyNone(t)

	stub := &stubRegistry{}

	r := &Runnable{manager: nil, logger: libLog.NewNop()}

	require.NoError(t, r.Run(nil))
	assert.Equal(t, 0, stub.closeCalls(), "nil-manager Run must not close anything")
}

// TestNewRunnable verifies the constructor wires the manager, descriptor,
// logger, and recorder, leaving notifyContext nil so production uses
// signal.NotifyContext.
func TestNewRunnable(t *testing.T) {
	t.Parallel()

	stub := &stubRegistry{}
	mgr := newStubManager(t, stub)
	svc := BuildServiceDescriptor("midaz-ledger", 3002)
	logger := libLog.NewNop()
	recorder := &stubRecorder{}

	r := NewRunnable(mgr, svc, logger, recorder)

	require.NotNil(t, r)
	assert.Same(t, mgr, r.manager)
	assert.Equal(t, svc, r.svc)
	assert.Same(t, recorder, r.metrics)
	assert.Nil(t, r.notifyContext)
}

// TestNewRunnable_NilRecorderStoresNop verifies a nil recorder is replaced by
// the no-op recorder at construction, so Run never dereferences nil.
func TestNewRunnable_NilRecorderStoresNop(t *testing.T) {
	t.Parallel()

	stub := &stubRegistry{}
	mgr := newStubManager(t, stub)
	svc := BuildServiceDescriptor("midaz-ledger", 3002)

	r := NewRunnable(mgr, svc, libLog.NewNop(), nil)

	require.NotNil(t, r)
	require.NotNil(t, r.metrics)
	_, ok := r.metrics.(NopMetricsRecorder)
	assert.True(t, ok, "nil recorder must be stored as NopMetricsRecorder")
}
