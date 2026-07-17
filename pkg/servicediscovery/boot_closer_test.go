// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"errors"
	"sync"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// bootCloserStubRegistry is a minimal in-memory libsd.Registry whose Watch
// returns a live channel so a Resolve lazy-spawns a background watcher goroutine
// that only dies when the manager's base context is cancelled by Close. It
// records Close via the optional Close() seam that libsd.Manager.Close delegates
// to, so a test can assert CloseOnBootFailure actually closed the manager.
type bootCloserStubRegistry struct {
	mu         sync.Mutex
	closeCalls int
	watchCh    chan libsd.Event
}

func newBootCloserStubRegistry() *bootCloserStubRegistry {
	return &bootCloserStubRegistry{watchCh: make(chan libsd.Event)}
}

func (s *bootCloserStubRegistry) Register(_ context.Context, _ libsd.Service) error { return nil }
func (s *bootCloserStubRegistry) Deregister(_ context.Context, _ string) error      { return nil }
func (s *bootCloserStubRegistry) Resolve(_ context.Context, _, _ string) (libsd.Service, error) {
	return libsd.Service{}, errors.New("no healthy instances")
}

// Watch returns the shared live channel; the watcher goroutine blocks on it
// until the manager's base context is cancelled by Close.
func (s *bootCloserStubRegistry) Watch(_ context.Context, _ string) (<-chan libsd.Event, error) {
	return s.watchCh, nil
}

// Close satisfies the optional seam libsd.Manager.Close delegates to.
func (s *bootCloserStubRegistry) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closeCalls++

	return nil
}

func (s *bootCloserStubRegistry) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.closeCalls
}

// newBootCloserStubManager builds an enabled libsd.Manager backed by stub so a
// Resolve runs through the manager and lazy-spawns the watcher goroutine.
func newBootCloserStubManager(t *testing.T, stub libsd.Registry) *libsd.Manager {
	t.Helper()

	mgr, err := libsd.New(
		libsd.Config{Enabled: true, ConsulAddr: "consul:8500", AdvertiseAddr: "svc.test:3002"},
		libsd.WithLogger(libLog.NewNop()),
		libsd.WithRegistry(stub),
	)
	require.NoError(t, err)

	return mgr
}

// TestBootCloser_ArmedClosesManagerAndReapsWatcher proves an armed BootCloser
// tears down the boot-time watcher goroutine (goleak-guarded) and closes the
// manager exactly once. It builds an enabled manager over a stub registry,
// triggers a Resolve to lazy-spawn the watcher, then asserts CloseOnBootFailure
// leaves no leaked goroutine and Close was invoked once.
func TestBootCloser_ArmedClosesManagerAndReapsWatcher(t *testing.T) {
	defer goleak.VerifyNone(t)

	stub := newBootCloserStubRegistry()
	mgr := newBootCloserStubManager(t, stub)

	// Resolve lazy-spawns the managed watcher goroutine (Watch returns a live
	// channel) that only exits when Close cancels the manager base context.
	_, _ = mgr.Resolve(context.Background(), "plugin-auth", "fallback:4000")

	closer := NewBootCloser(libLog.NewNop(), mgr)
	closer.CloseOnBootFailure()

	assert.Equal(t, 1, stub.closeCount(), "armed CloseOnBootFailure must close the manager exactly once")
}

// TestBootCloser_DisarmedDoesNotClose proves Disarm suppresses the close: after
// disarming, CloseOnBootFailure must not touch the manager (Close-count == 0),
// modeling the success path where the launcher Runnable owns the graceful close.
func TestBootCloser_DisarmedDoesNotClose(t *testing.T) {
	defer goleak.VerifyNone(t)

	stub := newBootCloserStubRegistry()
	mgr := newBootCloserStubManager(t, stub)

	_, _ = mgr.Resolve(context.Background(), "plugin-auth", "fallback:4000")

	closer := NewBootCloser(libLog.NewNop(), mgr)
	closer.Disarm()
	closer.CloseOnBootFailure()

	assert.Equal(t, 0, stub.closeCount(), "disarmed CloseOnBootFailure must not close the manager")

	// The watcher is still live because the closer did not close it; close it
	// here so this test leaves no leaked goroutine for goleak to catch.
	require.NoError(t, mgr.Close())
}

// TestBootCloser_NilManagerNoOp confirms an armed closer with a nil manager is a
// safe no-op: it neither panics nor closes anything.
func TestBootCloser_NilManagerNoOp(t *testing.T) {
	t.Parallel()

	closer := NewBootCloser(libLog.NewNop(), nil)

	require.NotPanics(t, closer.CloseOnBootFailure)
}

// TestBootCloser_NilReceiverNoOp confirms a nil BootCloser is safe across Disarm
// and CloseOnBootFailure: neither panics.
func TestBootCloser_NilReceiverNoOp(t *testing.T) {
	t.Parallel()

	var closer *BootCloser

	require.NotPanics(t, closer.Disarm)
	require.NotPanics(t, closer.CloseOnBootFailure)
}
