// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package accountprotection coordinates the operations that may not run while an
// account is being closed: admitting a balance into the transaction cache after a
// miss, creating a balance and deleting one.
//
// PostgreSQL's account.closed_at is the source of truth. The cache keeps only the
// exceptional markers, so an account that was never closed owns no key at all and
// the hot path of a cache hit reaches neither this package nor the database.
package accountprotection

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ClosingStateReader reads the authoritative closing state of accounts. The
// implementation must read the PRIMARY: a closing that committed a moment ago has
// to be visible here, and replica lag would report a closed account as open.
type ClosingStateReader interface {
	ListClosedAtByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]*time.Time, error)
}

// MarkerStore is the cache surface the protection uses: the exceptional closing
// and closed markers, and the administrative ownership shared by closing, balance
// creation, balance deletion and cache-miss admission.
type MarkerStore interface {
	GetAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (string, bool, error)
	GetAccountClosedMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (time.Time, bool, error)
	SetAccountClosedMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, closedAt time.Time) error
	AcquireAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
	ReleaseAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
}

// ClosedAccountError reports that one account of the requested set is closed. The
// caller decides which business error that maps to, because admitting a movement
// and creating a balance refuse a closed account with different codes.
type ClosedAccountError struct {
	AccountID uuid.UUID
	ClosedAt  time.Time
}

func (e ClosedAccountError) Error() string {
	return fmt.Sprintf("account %s is closed", e.AccountID)
}

// Guard answers two questions for a set of accounts: who owns them
// administratively right now, and whether any of them is closed.
type Guard struct {
	accounts ClosingStateReader
	markers  MarkerStore
}

// NewGuard builds a guard over the authoritative account reader and the cache.
func NewGuard(accounts ClosingStateReader, markers MarkerStore) *Guard {
	return &Guard{accounts: accounts, markers: markers}
}

// Admission is the administrative ownership one operation holds over a set of
// accounts. It is released by its owner or, when the operation's outcome is
// unknown, left in place for reconciliation — never by age.
type Admission struct {
	guard          *Guard
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	accountIDs     []uuid.UUID
	token          string
	indeterminate  bool
}

// AcquireAdmission takes the administrative ownership of every requested account.
//
// Acquisition walks the accounts in a stable order, so two operations competing
// over overlapping sets always contend on the same account first and neither can
// hold half of what the other needs. A refusal releases every ownership this call
// took and nothing else: a key another operation owns is never touched.
//
// Duplicated identifiers collapse into one acquisition. That is what makes the
// external companion free: the companion balance belongs to the same account, so
// it is already covered and never asks for an ownership of its own.
//
// A nil guard, or a guard without a cache, means the deployment carries no
// protection surface; acquisition then yields a handle that owns nothing, so the
// existing behavior is preserved byte for byte.
func (g *Guard) AcquireAdmission(ctx context.Context, organizationID, ledgerID uuid.UUID, accountIDs []uuid.UUID) (*Admission, error) {
	if g == nil || g.markers == nil {
		return &Admission{}, nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.acquire_account_admission")
	defer span.End()

	ordered := sortedUniqueAccountIDs(accountIDs)

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.Int("app.request.account_ids_count", len(ordered)),
	)

	admission := &Admission{
		guard:          g,
		organizationID: organizationID,
		ledgerID:       ledgerID,
		accountIDs:     make([]uuid.UUID, 0, len(ordered)),
		token:          uuid.NewString(),
	}

	for _, accountID := range ordered {
		if err := g.refuseWhenClosing(ctx, span, logger, organizationID, ledgerID, accountID); err != nil {
			admission.Release(ctx)

			return nil, err
		}

		acquired, err := g.markers.AcquireAccountAdminOwnership(ctx, organizationID, ledgerID, accountID, admission.token)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to acquire the account administrative ownership", err)
			logger.Log(ctx, libLog.LevelError, "Failed to acquire the account administrative ownership", libLog.Err(err))

			admission.Release(ctx)

			return nil, pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
		}

		if !acquired {
			conflict := pkg.ValidateBusinessError(constant.ErrAccountClosingInProgress, constant.EntityAccount)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "The account is owned by another administrative operation", conflict)

			admission.Release(ctx)

			return nil, conflict
		}

		admission.accountIDs = append(admission.accountIDs, accountID)
	}

	return admission, nil
}

// EnsureOpen refuses the operation when any of the accounts is closed.
//
// The cached closing instant answers first, so a second attempt over an account
// already proven closed costs no query. Otherwise the authoritative row decides,
// and a closing found there recomposes the negative cache before the refusal — the
// expiry of that cache is a cache horizon, never a reopening.
//
// The absence of every marker proves nothing on its own, which is why it leads to
// the authoritative read instead of to an admission.
func (g *Guard) EnsureOpen(ctx context.Context, organizationID, ledgerID uuid.UUID, accountIDs []uuid.UUID) error {
	if g == nil || g.markers == nil || g.accounts == nil {
		return nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.ensure_accounts_open")
	defer span.End()

	ordered := sortedUniqueAccountIDs(accountIDs)

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.Int("app.request.account_ids_count", len(ordered)),
	)

	if len(ordered) == 0 {
		return nil
	}

	for _, accountID := range ordered {
		closedAt, found, err := g.markers.GetAccountClosedMarker(ctx, organizationID, ledgerID, accountID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to read the account closed marker", err)
			logger.Log(ctx, libLog.LevelError, "Failed to read the account closed marker", libLog.Err(err))

			return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
		}

		if found {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "The account is closed", nil)

			return ClosedAccountError{AccountID: accountID, ClosedAt: closedAt}
		}
	}

	closingStates, err := g.accounts.ListClosedAtByIDs(ctx, organizationID, ledgerID, ordered)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read the authoritative account closing state", err)
		logger.Log(ctx, libLog.LevelError, "Failed to read the authoritative account closing state", libLog.Err(err))

		return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
	}

	for _, accountID := range ordered {
		closedAt, ok := closingStates[accountID]
		if !ok || closedAt == nil {
			continue
		}

		if err := g.markers.SetAccountClosedMarker(ctx, organizationID, ledgerID, accountID, *closedAt); err != nil {
			// The refusal below does not depend on the cache, so a failed write only
			// costs the next attempt one more authoritative read.
			libOpentelemetry.HandleSpanError(span, "Failed to recompose the account closed marker", err)
			logger.Log(ctx, libLog.LevelWarn, "Failed to recompose the account closed marker", libLog.Err(err))
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "The account is closed", nil)

		return ClosedAccountError{AccountID: accountID, ClosedAt: *closedAt}
	}

	return nil
}

// EnsureAvailable refuses an operation over an account the cache marks as closing
// or closed. It reads the markers only: no ownership is taken and no account row
// is read, so a path already holding a warm balance keeps costing no query.
//
// It is the availability check of the paths that admit balances without reaching
// the engine, where the marker check lives inside the same atomic execution. The
// absence of both markers is the normal state of an open account and passes;
// a marker that cannot be read refuses, because reading a protection failure as
// absence would turn it into an authorization.
func (g *Guard) EnsureAvailable(ctx context.Context, organizationID, ledgerID uuid.UUID, accountIDs []uuid.UUID) error {
	if g == nil || g.markers == nil {
		return nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.ensure_accounts_available")
	defer span.End()

	ordered := sortedUniqueAccountIDs(accountIDs)

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.Int("app.request.account_ids_count", len(ordered)),
	)

	for _, accountID := range ordered {
		closedAt, found, err := g.markers.GetAccountClosedMarker(ctx, organizationID, ledgerID, accountID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to read the account closed marker", err)
			logger.Log(ctx, libLog.LevelError, "Failed to read the account closed marker", libLog.Err(err))

			return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
		}

		if found {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "The account is closed", nil)

			return ClosedAccountError{AccountID: accountID, ClosedAt: closedAt}
		}

		if err := g.refuseWhenClosing(ctx, span, logger, organizationID, ledgerID, accountID); err != nil {
			return err
		}
	}

	return nil
}

// refuseWhenClosing rejects an operation over an account a closing attempt owns.
// An unreadable marker is a refusal of its own: reading it as absence would turn a
// protection failure into an authorization.
//
// The two refusals are of different classes and reach the acquisition span through
// different helpers: a marker that could not be read is a technical failure of the
// protection surface, while a marker that is there is the business outcome the
// coordination exists to produce.
func (g *Guard) refuseWhenClosing(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID, accountID uuid.UUID) error {
	_, found, err := g.markers.GetAccountClosingMarker(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

		libOpentelemetry.HandleSpanError(span, "Failed to read the account closing marker", err)
		logger.Log(ctx, libLog.LevelError, "Failed to read the account closing marker", libLog.Err(err))

		return indeterminate
	}

	if found {
		conflict := pkg.ValidateBusinessError(constant.ErrAccountClosingInProgress, constant.EntityAccount)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "A closing attempt owns the account", conflict)

		return conflict
	}

	return nil
}

// Token is the administrative token of this admission. It stays inside the
// services and the cache adapter: it is not a transaction generation and never
// reaches a receipt, a recovery record or a public contract.
func (a *Admission) Token() string {
	if a == nil {
		return ""
	}

	return a.token
}

// Accounts are the accounts this admission owns, in acquisition order.
func (a *Admission) Accounts() []uuid.UUID {
	if a == nil {
		return nil
	}

	return a.accountIDs
}

// MarkIndeterminate records that the operation's outcome could not be
// established, so Release keeps the ownership in place. Work that may still land
// must not have its protection removed on the way out; reconciliation resolves it
// once the result is known.
func (a *Admission) MarkIndeterminate() {
	if a == nil {
		return
	}

	a.indeterminate = true
}

// Release drops every ownership this admission holds, in the reverse order of
// acquisition and only where the key still carries its token. It is best-effort:
// a failed release leaves a key for reconciliation rather than failing an
// operation whose result is already known.
func (a *Admission) Release(ctx context.Context) {
	if a == nil || a.guard == nil || a.guard.markers == nil || len(a.accountIDs) == 0 {
		return
	}

	if a.indeterminate {
		return
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()

	releaseCtx, span := tracer.Start(releaseCtx, "exec.release_account_admission")
	defer span.End()

	for i := len(a.accountIDs) - 1; i >= 0; i-- {
		released, err := a.guard.markers.ReleaseAccountAdminOwnership(releaseCtx, a.organizationID, a.ledgerID, a.accountIDs[i], a.token)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to release the account administrative ownership", err)
			logger.Log(releaseCtx, libLog.LevelWarn, "Failed to release the account administrative ownership", libLog.Err(err))

			continue
		}

		if !released {
			logger.Log(releaseCtx, libLog.LevelDebug, "The account administrative ownership was not owned at release")
		}
	}

	a.accountIDs = nil
}

// releaseTimeout bounds the cleanup that runs after the operation decided its own
// outcome. The context is detached from the request so a cancellation cannot leave
// an ownership behind, while the timeout keeps a stuck cache from holding the
// caller.
const releaseTimeout = 5 * time.Second

// AsClosedAccountError reports whether err carries a closed account, so callers
// can translate it into the business error their own surface uses.
func AsClosedAccountError(err error) (ClosedAccountError, bool) {
	var closed ClosedAccountError
	if errors.As(err, &closed) {
		return closed, true
	}

	return ClosedAccountError{}, false
}

func sortedUniqueAccountIDs(accountIDs []uuid.UUID) []uuid.UUID {
	unique := make(map[uuid.UUID]struct{}, len(accountIDs))
	ordered := make([]uuid.UUID, 0, len(accountIDs))

	for _, accountID := range accountIDs {
		if accountID == uuid.Nil {
			continue
		}

		if _, seen := unique[accountID]; seen {
			continue
		}

		unique[accountID] = struct{}{}

		ordered = append(ordered, accountID)
	}

	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].String() < ordered[j].String()
	})

	return ordered
}
