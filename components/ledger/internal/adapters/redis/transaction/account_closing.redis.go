// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	// AccountClosedMarkerTTLSeconds is the lifetime, in whole seconds, of the
	// negative cache that records a confirmed closing. Five minutes is a cache
	// horizon, not a lifetime of the closing: when it expires the next load reads
	// the authoritative closed_at again and recomposes it. Set and SetNX multiply
	// their ttl argument by time.Second, so this is passed as a whole-second count.
	AccountClosedMarkerTTLSeconds = 300

	// accountClosedMarkerLayout is the instant format of the closed marker. A
	// nanosecond-precision RFC 3339 value round-trips the timestamp the database
	// generated without widening it.
	accountClosedMarkerLayout = time.RFC3339Nano
)

// ErrAccountProtectionMarkerUnreadable reports a marker that exists but cannot be
// interpreted: an empty owner token, or a closing instant that does not parse.
// It is deliberately distinct from absence — a missing marker is the normal state
// of an open account, while an unreadable one means the protection state could not
// be established and the caller must refuse rather than assume the account is open.
var ErrAccountProtectionMarkerUnreadable = errors.New("account protection marker is unreadable")

// AccountProtectionRepository is the account-scoped protection surface of the
// transaction cache: the exceptional closing/closed markers and the administrative
// ownership that coordinates closing with balance creation, deletion and cache-miss
// admission.
//
// There is no key for an open account. Absence of every marker is the normal state,
// it never authorizes admitting a balance on its own, and no path backfills keys for
// accounts that were never closed.
//
// Every method here resolves to ONE cache command, which the instrumented pool
// already spans; the error class and the account scope belong on the caller's
// domain span, where the refusal is decided.
type AccountProtectionRepository interface {
	// AcquireAccountClosingMarker installs the closing marker for one account when
	// no attempt owns it, carrying the caller's token. It returns false when another
	// attempt already holds the marker. The marker gets NO expiry: an attempt whose
	// write outcome is unknown keeps the account protected until that write is
	// resolved.
	AcquireAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
	// GetAccountClosingMarker returns the token of the attempt that owns the closing
	// marker. A missing marker returns ("", false, nil); a marker that exists with an
	// empty token returns ErrAccountProtectionMarkerUnreadable.
	GetAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (string, bool, error)
	// ReleaseAccountClosingMarker removes the closing marker only when it still
	// carries the caller's token, so a late release can never drop a newer attempt's
	// protection. It returns false when the marker is missing or owned by another.
	ReleaseAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
	// SetAccountClosedMarker writes the confirmed closing instant as a negative cache
	// entry with its fixed TTL. It is unconditional: the instant comes from the
	// authoritative row, so recomposing it after expiry must not depend on whether a
	// previous entry survived.
	SetAccountClosedMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, closedAt time.Time) error
	// GetAccountClosedMarker returns the cached closing instant. A missing marker
	// returns (zero, false, nil); a marker whose value is not a timestamp returns
	// ErrAccountProtectionMarkerUnreadable.
	GetAccountClosedMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (time.Time, bool, error)
	// AcquireAccountAdminOwnership takes the administrative ownership of one account
	// for the caller's token, returning false when another operation owns it. Like
	// the closing marker it carries no releasing expiry: ownership of work whose
	// result is unknown is resolved by reconciliation, never by age.
	AcquireAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
	// ReleaseAccountAdminOwnership drops the ownership only when it still carries the
	// caller's token.
	ReleaseAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
}

// Compile-time guarantee that the transaction cache repository serves the
// account-protection surface.
var _ AccountProtectionRepository = (*RedisConsumerRepository)(nil)

func (rr *RedisConsumerRepository) AcquireAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return rr.acquireAccountProtection(ctx, utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID), token)
}

func (rr *RedisConsumerRepository) GetAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (string, bool, error) {
	value, err := rr.Get(ctx, utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID))
	if err != nil {
		return "", false, err
	}

	if value == "" {
		return "", false, nil
	}

	token := strings.TrimSpace(value)
	if token == "" {
		return "", false, fmt.Errorf("%w: closing marker carries no owner token", ErrAccountProtectionMarkerUnreadable)
	}

	return token, true, nil
}

func (rr *RedisConsumerRepository) ReleaseAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return rr.releaseAccountProtection(ctx, utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID), token)
}

func (rr *RedisConsumerRepository) SetAccountClosedMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, closedAt time.Time) error {
	if closedAt.IsZero() {
		return fmt.Errorf("%w: closing instant is zero", ErrAccountProtectionMarkerUnreadable)
	}

	key := utils.AccountClosedMarkerKey(organizationID, ledgerID, accountID)

	return rr.Set(ctx, key, closedAt.UTC().Format(accountClosedMarkerLayout), AccountClosedMarkerTTLSeconds)
}

func (rr *RedisConsumerRepository) GetAccountClosedMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (time.Time, bool, error) {
	value, err := rr.Get(ctx, utils.AccountClosedMarkerKey(organizationID, ledgerID, accountID))
	if err != nil {
		return time.Time{}, false, err
	}

	if value == "" {
		return time.Time{}, false, nil
	}

	closedAt, parseErr := time.Parse(accountClosedMarkerLayout, strings.TrimSpace(value))
	if parseErr != nil {
		return time.Time{}, false, fmt.Errorf("%w: closed marker is not a timestamp", ErrAccountProtectionMarkerUnreadable)
	}

	return closedAt.UTC(), true, nil
}

func (rr *RedisConsumerRepository) AcquireAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return rr.acquireAccountProtection(ctx, utils.AccountAdminOwnershipKey(organizationID, ledgerID, accountID), token)
}

func (rr *RedisConsumerRepository) ReleaseAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return rr.releaseAccountProtection(ctx, utils.AccountAdminOwnershipKey(organizationID, ledgerID, accountID), token)
}

// acquireAccountProtection installs one protection key for the caller's token when
// it is free. A zero TTL is passed on purpose: these keys are released by their
// owner or by reconciliation, never by age.
func (rr *RedisConsumerRepository) acquireAccountProtection(ctx context.Context, key, token string) (bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, fmt.Errorf("%w: owner token is empty", ErrAccountProtectionMarkerUnreadable)
	}

	return rr.SetNX(ctx, key, token, 0)
}

// releaseAccountProtection drops one protection key only when it still carries the
// caller's token, so a release that arrives late cannot remove protection another
// operation installed afterwards.
func (rr *RedisConsumerRepository) releaseAccountProtection(ctx context.Context, key, token string) (bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, fmt.Errorf("%w: owner token is empty", ErrAccountProtectionMarkerUnreadable)
	}

	return rr.DeleteIfValue(ctx, key, token)
}
