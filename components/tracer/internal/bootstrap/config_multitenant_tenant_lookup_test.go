// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	lookupTTL        = 60 * time.Second
	lookupTenantA    = "tenant-a"
	lookupTenantNew  = "tenant-new"
	lookupTenantGone = "tenant-gone"
)

var lookupEpoch = time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)

// stepClock is a clock.Clock whose time only moves when a test advances it.
type stepClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *stepClock) NewTicker(time.Duration) (<-chan time.Time, func()) {
	return make(chan time.Time), func() {}
}

func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// fakeTenantLister answers from a mutable list and counts the calls. When gate
// is set, every call blocks until the gate closes or its context ends.
type fakeTenantLister struct {
	mu       sync.Mutex
	tenants  []*tmclient.TenantSummary
	err      error
	calls    int
	services []string
	gate     chan struct{}
	entered  chan struct{}
}

func (f *fakeTenantLister) GetActiveTenantsByService(ctx context.Context, service string) ([]*tmclient.TenantSummary, error) {
	f.mu.Lock()
	f.calls++
	f.services = append(f.services, service)
	gate, entered := f.gate, f.entered
	f.mu.Unlock()

	if entered != nil {
		entered <- struct{}{}
	}

	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return nil, f.err
	}

	return append([]*tmclient.TenantSummary(nil), f.tenants...), nil
}

func (f *fakeTenantLister) set(err error, tenants ...*tmclient.TenantSummary) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.err, f.tenants = err, tenants
}

func (f *fakeTenantLister) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

func activeTenant(id string) *tmclient.TenantSummary {
	return &tmclient.TenantSummary{ID: id, Name: id, Status: "active"}
}

func newLookupFixture(t *testing.T, tenants ...*tmclient.TenantSummary) (*activeTenantSets, *fakeTenantLister, *stepClock) {
	t.Helper()

	lister := &fakeTenantLister{tenants: tenants}
	clk := &stepClock{now: lookupEpoch}
	sets := newActiveTenantSets(lister, activeTenantSetConfig{
		TTL: lookupTTL, FetchTimeout: 5 * time.Second, Clock: clk, Logger: libLog.NewNop(),
	}, producerauth.ServiceLedger)

	return sets, lister, clk
}

// waitForAttempts waits until the ledger set has completed n list calls, so a
// test can observe the outcome of a detached refresh.
func waitForAttempts(t *testing.T, sets *activeTenantSets, n uint64) {
	t.Helper()

	set := sets.byService[producerauth.ServiceLedger]
	require.Eventually(t, func() bool { return set.completedAttempts() >= n && !set.revalidating.Load() },
		time.Second, time.Millisecond)
}

func lookupLedger(ctx context.Context, sets *activeTenantSets, tenantID string) error {
	return sets.Lookup(ctx, tenantID, producerauth.ServiceLedger)
}

func TestActiveTenantSetsHitAnswersFromTheFreshSet(t *testing.T) {
	t.Parallel()

	sets, lister, clk := newLookupFixture(t, activeTenant(lookupTenantA))

	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA), "the first lookup fetches the set")
	require.Equal(t, 1, lister.callCount())
	require.Equal(t, []string{producerauth.ServiceLedger}, lister.services)

	clk.advance(lookupTTL - time.Second)
	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA))
	require.Equal(t, 1, lister.callCount(), "a fresh set answers a member without a call")

	clk.advance(time.Second)
	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA))
	waitForAttempts(t, sets, 2)
	require.Equal(t, 2, lister.callCount(), "an expired set is refreshed")
}

func TestActiveTenantSetsMissRefreshIsThrottled(t *testing.T) {
	t.Parallel()

	sets, lister, clk := newLookupFixture(t, activeTenant(lookupTenantA))
	sets.warmUp(t.Context())
	require.Equal(t, 1, lister.callCount())

	require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantGone), tmcore.ErrTenantNotFound)
	require.Equal(t, 2, lister.callCount(), "the first miss refreshes: the warm-up was not a miss refresh")

	require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantGone), tmcore.ErrTenantNotFound)
	require.Equal(t, 2, lister.callCount(), "a miss inside the interval is denied without a call")

	clk.advance(activeTenantMissRefreshInterval - time.Second)
	require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantNew), tmcore.ErrTenantNotFound)
	require.Equal(t, 2, lister.callCount(), "the interval is shared by every absent tenant")

	clk.advance(time.Second)
	require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantGone), tmcore.ErrTenantNotFound)
	require.Equal(t, 3, lister.callCount(), "a miss past the interval refreshes once")
}

func TestActiveTenantSetsAdmitsANewlyOnboardedTenantOnItsFirstMiss(t *testing.T) {
	t.Parallel()

	sets, lister, clk := newLookupFixture(t, activeTenant(lookupTenantA))
	sets.warmUp(t.Context())

	clk.advance(lookupTTL)
	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA), "a stale member revalidates in the background")
	waitForAttempts(t, sets, 2)

	lister.set(nil, activeTenant(lookupTenantA), activeTenant(lookupTenantNew))
	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantNew),
		"neither the warm-up nor the revalidation consumed the miss interval")
	require.Equal(t, 3, lister.callCount())
}

func TestActiveTenantSetsOnboardingWaitsOutARecentMissRefresh(t *testing.T) {
	t.Parallel()

	sets, lister, clk := newLookupFixture(t, activeTenant(lookupTenantA))
	sets.warmUp(t.Context())

	require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantGone), tmcore.ErrTenantNotFound)
	require.Equal(t, 2, lister.callCount())

	lister.set(nil, activeTenant(lookupTenantA), activeTenant(lookupTenantNew))
	require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantNew), tmcore.ErrTenantNotFound,
		"a miss refresh ran inside the interval")

	clk.advance(activeTenantMissRefreshInterval)
	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantNew))
	require.Equal(t, 3, lister.callCount())
}

func TestActiveTenantSetsKeepsOnlyActiveTenants(t *testing.T) {
	t.Parallel()

	sets, _, _ := newLookupFixture(
		t,
		&tmclient.TenantSummary{ID: lookupTenantA, Status: "ACTIVE"},
		&tmclient.TenantSummary{ID: "tenant-suspended", Status: "suspended"},
		&tmclient.TenantSummary{ID: "tenant-purged", Status: "purged"},
		&tmclient.TenantSummary{ID: "", Status: "active"},
		nil,
	)

	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA), "status is compared case-insensitively")

	for _, tenantID := range []string{"tenant-suspended", "tenant-purged"} {
		require.ErrorIs(t, lookupLedger(t.Context(), sets, tenantID), tmcore.ErrTenantNotFound, tenantID)
	}
}

func TestActiveTenantSetsServesAStaleSetWhileTheTenantManagerFails(t *testing.T) {
	t.Parallel()

	sets, lister, clk := newLookupFixture(t, activeTenant(lookupTenantA))
	sets.warmUp(t.Context())

	lister.set(errors.New("tenant manager returned status 503 for service ledger"))
	clk.advance(lookupTTL)

	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA), "a stale member is accepted")
	waitForAttempts(t, sets, 2)
	require.Equal(t, 2, lister.callCount(), "the stale set was revalidated in the background")

	err := lookupLedger(t.Context(), sets, lookupTenantGone)
	require.ErrorIs(t, err, errActiveTenantsUnavailable, "a miss on an unrefreshable set is never a denial")
	require.NotErrorIs(t, err, tmcore.ErrTenantNotFound)

	clk.advance(2 * lookupTTL)
	require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantA), errActiveTenantsUnavailable,
		"a set past the stale bound no longer answers")
	require.Equal(t, 3, lister.callCount(), "past the stale bound the caller waits for a refresh")
}

func TestActiveTenantSetsStaleMemberDoesNotWaitForAHangingRefresh(t *testing.T) {
	t.Parallel()

	sets, lister, clk := newLookupFixture(t, activeTenant(lookupTenantA))
	sets.warmUp(t.Context())

	gate, entered := make(chan struct{}), make(chan struct{}, 1)
	lister.mu.Lock()
	lister.gate, lister.entered = gate, entered
	lister.mu.Unlock()

	clk.advance(lookupTTL)

	for range 20 {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		err := lookupLedger(ctx, sets, lookupTenantA)

		cancel()
		require.NoError(t, err, "a stale member is answered at once")
	}

	<-entered
	require.Equal(t, 2, lister.callCount(), "twenty stale hits start one background refresh")

	close(gate)
	waitForAttempts(t, sets, 2)
	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA))
	require.Equal(t, 2, lister.callCount(), "the revalidated set is fresh again")
}

func TestActiveTenantSetsBackoffAfterAFailedListCall(t *testing.T) {
	t.Parallel()

	t.Run("without a set", func(t *testing.T) {
		t.Parallel()

		sets, lister, clk := newLookupFixture(t)
		lister.set(errors.New("connection refused"))

		require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantA), errActiveTenantsUnavailable)
		require.Equal(t, 1, lister.callCount())

		for range 10 {
			require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantA), errActiveTenantsUnavailable)
		}

		require.Equal(t, 1, lister.callCount(), "no list call inside the backoff")

		clk.advance(activeTenantMissRefreshInterval)
		require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantA), errActiveTenantsUnavailable)
		require.Equal(t, 2, lister.callCount(), "the backoff has passed")
	})

	t.Run("with a stale set", func(t *testing.T) {
		t.Parallel()

		sets, lister, clk := newLookupFixture(t, activeTenant(lookupTenantA))
		sets.warmUp(t.Context())
		lister.set(errors.New("connection refused"))
		clk.advance(lookupTTL)

		require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA))
		waitForAttempts(t, sets, 2)

		for range 10 {
			require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA), "members inside the stale bound are admitted")
			require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantNew), errActiveTenantsUnavailable,
				"others are unavailable, never denied")
		}

		require.Equal(t, 2, lister.callCount(), "no list call inside the backoff")
	})

	t.Run("with a fresh set", func(t *testing.T) {
		t.Parallel()

		sets, lister, clk := newLookupFixture(t, activeTenant(lookupTenantA))
		sets.warmUp(t.Context())
		lister.set(errors.New("connection refused"))

		require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantNew), errActiveTenantsUnavailable)
		require.Equal(t, 2, lister.callCount())

		clk.advance(activeTenantMissRefreshInterval - time.Second)

		for range 10 {
			require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA))
			require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantGone), errActiveTenantsUnavailable)
		}

		require.Equal(t, 2, lister.callCount(), "no list call inside the backoff")
	})
}

func TestActiveTenantSetsFlightSkipsACallAnotherFlightAnswered(t *testing.T) {
	t.Parallel()

	sets, lister, _ := newLookupFixture(t, activeTenant(lookupTenantA))
	set := sets.byService[producerauth.ServiceLedger]
	observed := set.completedAttempts()

	sets.warmUp(t.Context())
	require.Equal(t, 1, lister.callCount())

	require.NoError(t, set.refresh(t.Context(), observed), "a completed success is returned as is")
	require.Equal(t, 1, lister.callCount(), "the flight saw the newer attempt and made no call")

	lister.set(errors.New("connection refused"))
	observed = set.completedAttempts()
	require.ErrorIs(t, set.refresh(t.Context(), observed), errActiveTenantsUnavailable)
	require.Equal(t, 2, lister.callCount())

	require.ErrorIs(t, set.refresh(t.Context(), observed), errActiveTenantsUnavailable,
		"a completed failure is returned as is")
	require.Equal(t, 2, lister.callCount())
}

func TestActiveTenantSetsKeepsThePreviousSetOnAnEmptyList(t *testing.T) {
	t.Parallel()

	lister := &fakeTenantLister{tenants: []*tmclient.TenantSummary{activeTenant(lookupTenantA)}}
	clk := &stepClock{now: lookupEpoch}
	logger := testutil.NewMockLogger()
	sets := newActiveTenantSets(lister, activeTenantSetConfig{
		TTL: lookupTTL, FetchTimeout: 5 * time.Second, Clock: clk, Logger: logger,
	}, producerauth.ServiceLedger)
	sets.warmUp(t.Context())

	lister.set(nil)
	clk.advance(lookupTTL)
	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA))
	waitForAttempts(t, sets, 2)

	clk.advance(activeTenantMissRefreshInterval)
	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA), "the previous set still answers")
	waitForAttempts(t, sets, 3)
	require.Equal(t, 3, lister.callCount())

	var shrinkLogs int

	for _, call := range logger.Snapshot() {
		if call.Message == "Active tenant list came back empty; keeping the previous list until its stale bound" {
			shrinkLogs++
		}
	}

	require.Equal(t, 1, shrinkLogs, "a sustained empty answer is logged once")

	clk.advance(2 * lookupTTL)
	require.ErrorIs(t, lookupLedger(t.Context(), sets, lookupTenantA), tmcore.ErrTenantNotFound,
		"past the stale bound the empty list is accepted")
}

func TestActiveTenantSetsWithoutASetFailIsUnavailable(t *testing.T) {
	t.Parallel()

	sets, lister, clk := newLookupFixture(t)
	lister.set(context.DeadlineExceeded)

	err := lookupLedger(t.Context(), sets, lookupTenantA)
	require.ErrorIs(t, err, errActiveTenantsUnavailable)
	require.NotErrorIs(t, err, context.DeadlineExceeded, "a failed list call must not read as the caller's deadline")

	clk.advance(activeTenantMissRefreshInterval)

	authorizer := producerauth.NewTenantAuthorizer(sets.Lookup, true)
	err = authorizer.Authorize(t.Context(), lookupTenantA, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken})
	require.ErrorIs(t, err, constant.ErrTenantServiceUnavailable, "maps to 503 0161")
	require.Equal(t, 2, lister.callCount(), "without a set every lookup past the backoff retries the list")
}

func TestActiveTenantSetsCallerCancellationDoesNotAbortTheSharedRefresh(t *testing.T) {
	t.Parallel()

	sets, lister, _ := newLookupFixture(t, activeTenant(lookupTenantA))
	gate, entered := make(chan struct{}), make(chan struct{}, 1)
	lister.gate, lister.entered = gate, entered

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() { done <- lookupLedger(ctx, sets, lookupTenantA) }()

	<-entered
	cancel()

	err := <-done
	require.ErrorIs(t, err, context.Canceled)

	authorizer := producerauth.NewTenantAuthorizer(sets.Lookup, true)
	require.ErrorIs(t, authorizer.Authorize(ctx, lookupTenantA, producerauth.Producer{Service: producerauth.ServiceLedger}), context.Canceled,
		"a canceled caller keeps context.Canceled, mapped to 0330")
	require.Zero(t, sets.byService[producerauth.ServiceLedger].completedAttempts(), "the list call is still in flight")

	close(gate)
	waitForAttempts(t, sets, 1)
	require.NoError(t, lookupLedger(t.Context(), sets, lookupTenantA))
	require.Equal(t, 1, lister.callCount(), "the detached refresh completed and filled the set")
}

func TestActiveTenantSetsConcurrentLookupsShareOneRefresh(t *testing.T) {
	t.Parallel()

	const callers = 8

	sets, lister, _ := newLookupFixture(t, activeTenant(lookupTenantA))
	gate, entered := make(chan struct{}), make(chan struct{}, callers)
	lister.gate, lister.entered = gate, entered

	var (
		wg      sync.WaitGroup
		started sync.WaitGroup
	)

	errs := make(chan error, callers)

	started.Add(callers)

	for range callers {
		wg.Go(func() {
			started.Done()
			errs <- lookupLedger(t.Context(), sets, lookupTenantA)
		})
	}

	started.Wait()
	<-entered
	require.Never(t, func() bool { return lister.callCount() > 1 }, 50*time.Millisecond, 5*time.Millisecond,
		"every caller joins the in-flight list call while the gate is closed")

	close(gate)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	require.Equal(t, 1, lister.callCount(), "concurrent callers share the in-flight list call")
}

func TestActiveTenantSetsRefusesAServiceWithoutAList(t *testing.T) {
	t.Parallel()

	sets, lister, _ := newLookupFixture(t, activeTenant(lookupTenantA))

	require.ErrorIs(t, sets.Lookup(t.Context(), lookupTenantA, "admin"), tmcore.ErrTenantServiceAccessDenied)
	require.Zero(t, lister.callCount())
}

// TestTenantAuthorizerOverTenantManagerClient exercises the production lookup
// against the tenant-manager HTTP contract, so the client's request, status
// mapping and the authorizer's classification stay aligned.
func TestTenantAuthorizerOverTenantManagerClient(t *testing.T) {
	t.Parallel()

	serve := func(t *testing.T, status int, body string) (*producerauth.TenantAuthorizer, *int) {
		t.Helper()

		var calls int

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/tenants/active" || r.URL.Query().Get("service") != producerauth.ServiceLedger ||
				r.Header.Get("X-API-Key") != "svc-api-key" {
				w.WriteHeader(http.StatusNotFound)

				return
			}

			calls++

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)

		client, err := tmclient.NewClient(srv.URL, testutil.NewMockLogger(),
			tmclient.WithServiceAPIKey("svc-api-key"), tmclient.WithAllowInsecureHTTP())
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })

		sets := newActiveTenantSets(client, activeTenantSetConfig{TTL: lookupTTL, Logger: libLog.NewNop()}, producerauth.ServiceLedger)

		return producerauth.NewTenantAuthorizer(sets.Lookup, true), &calls
	}

	producer := producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken}

	t.Run("listed tenants", func(t *testing.T) {
		t.Parallel()

		authorizer, calls := serve(t, http.StatusOK,
			`[{"id":"tenant-ok","name":"ok","status":"active"},{"id":"tenant-suspended","name":"s","status":"suspended"}]`)

		require.NoError(t, authorizer.Authorize(t.Context(), "tenant-ok", producer))

		for _, tenantID := range []string{"tenant-missing", "tenant-suspended"} {
			require.ErrorIs(t, authorizer.Authorize(t.Context(), tenantID, producer), constant.ErrInsufficientPrivileges, tenantID)
		}

		require.Equal(t, 2, *calls, "the first miss refreshes once and the second is answered by the fresh set")
	})

	for name, status := range map[string]int{"server error": http.StatusBadGateway, "list permission denied": http.StatusForbidden} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			authorizer, _ := serve(t, status, `{"error":"no"}`)
			require.ErrorIs(t, authorizer.Authorize(t.Context(), "tenant-ok", producer), constant.ErrTenantServiceUnavailable)
		})
	}
}

func TestListFailureStatusReadsTheClientStatus(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.StatusForbidden, listFailureStatus(errors.New("tenant manager returned status 403 for service ledger")))
	require.Zero(t, listFailureStatus(errors.New("failed to execute request: connection refused")))
}

func TestActiveTenantSetMatchesCanonicalTenantIDs(t *testing.T) {
	t.Parallel()

	lister := &fakeTenantLister{}
	lister.set(
		nil,
		&tmclient.TenantSummary{ID: "0195D3B4-5A01-7000-8000-000000000007", Status: "active"},
		&tmclient.TenantSummary{ID: "tenant-a", Status: "active"},
	)

	sets := newActiveTenantSets(lister, activeTenantSetConfig{}, producerauth.ServiceLedger)

	for _, tenantID := range []string{"0195d3b45a0170008000000000000007", "0195d3b4-5a01-7000-8000-000000000007", "tenant-a"} {
		require.NoError(t, sets.Lookup(t.Context(), tenantID, producerauth.ServiceLedger), tenantID)
	}

	require.ErrorIs(t, sets.Lookup(t.Context(), "tenant_a", producerauth.ServiceLedger), tmcore.ErrTenantNotFound, "a non-UUID tenant matches only itself")
}
