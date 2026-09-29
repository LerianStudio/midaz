// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	"golang.org/x/sync/singleflight"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
)

const (
	// activeTenantMissRefreshInterval bounds how often an unknown tenant can
	// force a list refresh before the cached set expires, so a stream of
	// requests for tenants that do not exist cannot hammer the tenant-manager.
	activeTenantMissRefreshInterval = 5 * time.Second

	// activeTenantMaxStaleFactor is how many TTLs a set may be served past its
	// expiry while the tenant-manager cannot refresh it.
	activeTenantMaxStaleFactor = 3

	activeTenantStatus = "active"
)

// errActiveTenantsUnavailable marks a lookup the tenant-manager could not
// answer. It never wraps a tenant-manager denial or a context error, so the
// authorizer classifies it as an availability failure.
var errActiveTenantsUnavailable = errors.New("active tenant list unavailable")

var listStatusPattern = regexp.MustCompile(`returned status (\d{3})`)

// activeTenantLister is the credential-free tenant-manager read the producer
// tenant check needs; *tmclient.Client satisfies it.
type activeTenantLister interface {
	GetActiveTenantsByService(ctx context.Context, service string) ([]*tmclient.TenantSummary, error)
}

// activeTenantSetConfig carries the timing of every per-service set.
type activeTenantSetConfig struct {
	// TTL is how long a fetched set answers without a refresh.
	TTL time.Duration
	// FetchTimeout bounds one list call, independently of any caller.
	FetchTimeout time.Duration
	// MissRefreshInterval is the minimum spacing of refreshes forced by a
	// tenant absent from a fresh set.
	MissRefreshInterval time.Duration
	Clock               clock.Clock
	Logger              libLog.Logger
}

// activeTenantSets answers the producer tenant lookup from the tenant-manager's
// list of tenants active for each producer service. It holds tenant ids and
// nothing else: no connection settings or credentials of the producer service
// ever reach the Tracer.
type activeTenantSets struct {
	byService map[string]*activeTenantSet
}

// activeTenantSet is the cached active-tenant list of one producer service.
type activeTenantSet struct {
	lister  activeTenantLister
	service string
	cfg     activeTenantSetConfig
	flight  singleflight.Group

	// revalidating admits one detached background refresh at a time.
	revalidating atomic.Bool

	mu        sync.Mutex
	tenants   map[string]struct{}
	fetchedAt time.Time
	// lastMissRefresh is claimed only by lookups of a tenant absent from the
	// set, so refreshes for any other reason never delay an onboarding.
	lastMissRefresh time.Time
	// lastFailure starts the window during which no list call is made.
	lastFailure time.Time
	// attempts counts completed list calls; a flight started from an older
	// observation skips its call and returns lastErr.
	attempts uint64
	lastErr  error
	// shrinkLogged keeps a sustained empty answer to one Error log.
	shrinkLogged bool
}

// setView is one consistent observation of a set, taken under its lock.
type setView struct {
	member    bool
	fresh     bool
	usable    bool
	backoff   bool
	missClaim bool
	attempts  uint64
}

// newActiveTenantSets builds one set per service. Zero timings fall back to
// the tenant-manager defaults.
func newActiveTenantSets(lister activeTenantLister, cfg activeTenantSetConfig, services ...string) *activeTenantSets {
	if cfg.TTL <= 0 {
		cfg.TTL = time.Duration(defaultMultiTenantCacheTTLSec) * time.Second
	}

	if cfg.FetchTimeout <= 0 {
		cfg.FetchTimeout = time.Duration(defaultMultiTenantTimeout) * time.Second
	}

	if cfg.MissRefreshInterval <= 0 {
		cfg.MissRefreshInterval = activeTenantMissRefreshInterval
	}

	if cfg.Clock == nil {
		cfg.Clock = clock.New()
	}

	if cfg.Logger == nil {
		cfg.Logger = libLog.NewNop()
	}

	sets := &activeTenantSets{byService: make(map[string]*activeTenantSet, len(services))}
	for _, service := range services {
		sets.byService[service] = &activeTenantSet{lister: lister, service: service, cfg: cfg}
	}

	return sets
}

// Lookup satisfies producerauth.TenantLookup.
//
// A set is fresh for TTL and usable for activeTenantMaxStaleFactor TTLs.
// A member of a fresh set is accepted without a call; a member of a stale but
// usable set is accepted at once while one detached refresh runs in the
// background. A non-member of a usable set claims at most one blocking refresh
// per MissRefreshInterval; without the claim it is denied with
// tmcore.ErrTenantNotFound by a fresh set and answered errActiveTenantsUnavailable
// by a stale one. Without a usable set the caller waits for a shared refresh.
// For MissRefreshInterval after a failed list call no call is made and callers
// get those same answers from whatever set is usable. A failed refresh never
// turns into a denial, and a caller that goes away first gets its own context
// error.
func (s *activeTenantSets) Lookup(ctx context.Context, tenantID, service string) error {
	set, ok := s.byService[service]
	if !ok {
		return fmt.Errorf("service %q has no active tenant list: %w", service, tmcore.ErrTenantServiceAccessDenied)
	}

	return set.lookup(ctx, tenantID)
}

// warmUp fetches every set once. Failures are logged by the refresh and never
// propagated: requests refresh on demand, and they wait for this fetch only
// while no set exists.
func (s *activeTenantSets) warmUp(ctx context.Context) {
	for _, set := range s.byService {
		if err := set.refresh(ctx, set.completedAttempts()); err != nil {
			set.cfg.Logger.Log(ctx, libLog.LevelWarn, "Active tenant list not warmed at boot; requests will refresh it",
				libLog.String("tenant_service", set.service))
		}
	}
}

func (s *activeTenantSet) lookup(ctx context.Context, tenantID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	view := s.observe(tenantID)

	switch {
	case view.member && view.fresh:
		return nil
	case view.member && view.usable:
		if !view.backoff {
			s.revalidate(ctx, view.attempts)
		}

		return nil
	case view.backoff:
		return s.unavailable()
	case view.usable && !view.missClaim:
		if view.fresh {
			return s.notActive()
		}

		return s.unavailable()
	}

	if err := s.refresh(ctx, view.attempts); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return err
	}

	if s.isMember(tenantID) {
		return nil
	}

	return s.notActive()
}

// observe takes one consistent view of the set for tenantID and, for a
// non-member of a usable set outside the failure backoff, claims the next
// miss refresh when MissRefreshInterval has passed since the last one.
func (s *activeTenantSet) observe(tenantID string) setView {
	now := s.cfg.Clock.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	age := now.Sub(s.fetchedAt)
	view := setView{
		fresh:    s.tenants != nil && age < s.cfg.TTL,
		usable:   s.tenants != nil && age < activeTenantMaxStaleFactor*s.cfg.TTL,
		backoff:  !s.lastFailure.IsZero() && now.Sub(s.lastFailure) < s.cfg.MissRefreshInterval,
		attempts: s.attempts,
	}
	_, view.member = s.tenants[tenantKey(tenantID)]

	if view.usable && !view.member && !view.backoff &&
		(s.lastMissRefresh.IsZero() || now.Sub(s.lastMissRefresh) >= s.cfg.MissRefreshInterval) {
		s.lastMissRefresh = now
		view.missClaim = true
	}

	return view
}

func (s *activeTenantSet) completedAttempts() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.attempts
}

func (s *activeTenantSet) isMember(tenantID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, member := s.tenants[tenantKey(tenantID)]

	return member
}

func (s *activeTenantSet) notActive() error {
	return fmt.Errorf("tenant is not active for %s: %w", s.service, tmcore.ErrTenantNotFound)
}

func (s *activeTenantSet) unavailable() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastErr != nil {
		return s.lastErr
	}

	return fmt.Errorf("%w for %s", errActiveTenantsUnavailable, s.service)
}

// revalidate starts one detached refresh unless one is already running. It
// joins any flight a blocking caller started, so the two never double the
// list call.
func (s *activeTenantSet) revalidate(ctx context.Context, observed uint64) {
	if !s.revalidating.CompareAndSwap(false, true) {
		return
	}

	libRuntime.SafeGoWithContextAndComponent(context.WithoutCancel(ctx), s.cfg.Logger, "bootstrap", "active-tenant-revalidate",
		libRuntime.KeepRunning, func(ctx context.Context) {
			defer s.revalidating.Store(false)

			if err := s.refresh(ctx, observed); err != nil {
				s.cfg.Logger.Log(ctx, libLog.LevelDebug, "Background active tenant revalidation failed; the stale list keeps answering",
					libLog.String("tenant_service", s.service))
			}
		})
}

// refresh replaces the set with the tenant-manager's current list. Concurrent
// callers share one list call, which runs detached from any caller and is
// bounded by FetchTimeout, so a caller that goes away neither cancels it for
// the others nor waits for it. A flight whose starter observed an attempt
// that has since completed makes no call and returns that attempt's outcome.
func (s *activeTenantSet) refresh(ctx context.Context, observed uint64) error {
	result := s.flight.DoChan(s.service, func() (any, error) {
		return nil, s.fetch(context.WithoutCancel(ctx), observed)
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case outcome := <-result:
		return outcome.Err
	}
}

func (s *activeTenantSet) fetch(ctx context.Context, observed uint64) (err error) {
	s.mu.Lock()
	if s.attempts != observed {
		lastErr := s.lastErr
		s.mu.Unlock()

		return lastErr
	}
	s.mu.Unlock()

	defer func() {
		if recovered := recover(); recovered != nil {
			libRuntime.HandlePanicValue(ctx, s.cfg.Logger, recovered, "bootstrap", "active-tenant-list")
			err = s.complete(ctx, nil, fmt.Errorf("%w for %s: list call panicked", errActiveTenantsUnavailable, s.service))
		}
	}()

	fetchCtx, cancel := context.WithTimeout(ctx, s.cfg.FetchTimeout)
	defer cancel()

	summaries, listErr := s.lister.GetActiveTenantsByService(fetchCtx, s.service)
	if listErr != nil {
		s.logListFailure(fetchCtx, listErr)

		return s.complete(ctx, nil, fmt.Errorf("%w for %s: %v", errActiveTenantsUnavailable, s.service, listErr)) //nolint:errorlint // the cause must not leak a context error into the classification
	}

	tenants := make(map[string]struct{}, len(summaries))
	for _, summary := range summaries {
		if summary != nil && summary.ID != "" && strings.EqualFold(summary.Status, activeTenantStatus) {
			tenants[tenantKey(summary.ID)] = struct{}{}
		}
	}

	return s.complete(ctx, tenants, nil)
}

// complete records the outcome of one list call. An empty list never
// replaces a usable non-empty set: it is treated as a failure, so the
// previous set keeps answering until its stale bound.
func (s *activeTenantSet) complete(ctx context.Context, tenants map[string]struct{}, err error) error {
	now := s.cfg.Clock.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.attempts++

	if err == nil && len(tenants) == 0 && len(s.tenants) > 0 &&
		now.Sub(s.fetchedAt) < activeTenantMaxStaleFactor*s.cfg.TTL {
		err = fmt.Errorf("%w for %s: list returned no active tenants", errActiveTenantsUnavailable, s.service)

		if !s.shrinkLogged {
			s.shrinkLogged = true
			s.cfg.Logger.Log(ctx, libLog.LevelError,
				"Active tenant list came back empty; keeping the previous list until its stale bound",
				libLog.String("tenant_service", s.service), libLog.Int("previous_tenant_count", len(s.tenants)))
		}
	}

	if err != nil {
		s.lastFailure, s.lastErr = now, err

		return err
	}

	s.tenants, s.fetchedAt = tenants, now
	s.lastFailure, s.lastErr, s.shrinkLogged = time.Time{}, nil, false

	return nil
}

func (s *activeTenantSet) logListFailure(ctx context.Context, err error) {
	fields := []any{libLog.String("tenant_service", s.service), libLog.Err(err)}

	if status := listFailureStatus(err); status >= 400 && status < 500 {
		fields = append(fields, libLog.Int("http.status_code", status),
			libLog.String("hint", "the MULTI_TENANT_SERVICE_API_KEY may lack permission to list active tenants"))
	}

	s.cfg.Logger.Log(ctx, libLog.LevelWarn, "Active tenant list unavailable for producer tenant authorization", fields...)
}

// listFailureStatus extracts the HTTP status the tenant-manager client reports
// in its list error, or 0 when the failure carries none.
func listFailureStatus(err error) int {
	match := listStatusPattern.FindStringSubmatch(err.Error())
	if match == nil {
		return 0
	}

	status, convErr := strconv.Atoi(match[1])
	if convErr != nil {
		return 0
	}

	return status
}

// tenantKey is the set key of a tenant id: its canonical form, so a UUID
// tenant matches whether the tenant-manager lists it or a caller names it
// with or without dashes. An id that is not a valid tenant id is kept
// verbatim, and matches only itself.
func tenantKey(tenantID string) string {
	canonical, err := tmcore.CanonicalTenantID(tenantID)
	if err != nil {
		return tenantID
	}

	return canonical
}
