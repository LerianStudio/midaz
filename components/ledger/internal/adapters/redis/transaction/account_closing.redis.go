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

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

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

	// accountClosingWriteSuffix is appended to the attempt token once that attempt
	// issued its closing write. It is what separates an attempt that cannot have
	// closed anything from one whose write may still land: reconciliation may give
	// back the protection of the first, never of the second, and no elapsed time
	// turns the second into the first.
	//
	// The separator cannot occur in a token, which is a UUID, so the encoding is
	// unambiguous. It is private to this adapter: the marker still holds one
	// attempt's identity, and nothing outside reads its bytes.
	accountClosingWriteSuffix = "|w"
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
	// marker, whatever phase that attempt is in. A missing marker returns
	// ("", false, nil); a marker that exists with an empty token returns
	// ErrAccountProtectionMarkerUnreadable.
	GetAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (string, bool, error)
	// ReadAccountClosingAttempt returns the attempt recorded on the closing marker:
	// its token and whether it already issued its closing write. It is the read
	// reconciliation needs, because those two phases license different actions.
	ReadAccountClosingAttempt(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (AccountClosingAttempt, bool, error)
	// MarkAccountClosingWriteIssued records on the marker that the caller's attempt
	// is about to issue its closing write, so a later reconciliation cannot mistake
	// an unresolved write for an attempt that never wrote. It is conditional on the
	// caller's token and idempotent, returning false when another attempt owns the
	// marker or none does.
	MarkAccountClosingWriteIssued(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
	// ReleaseAccountClosingMarker removes the closing marker only when it still
	// carries the caller's token AND that attempt has not issued its closing write.
	// It is the release reconciliation uses, where an issued write is exactly the
	// case that must keep its protection. It returns false when the marker is
	// missing, owned by another attempt, or already carrying a write.
	ReleaseAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
	// ReleaseAccountClosingAttempt removes the closing marker of the caller's own
	// attempt in either phase. Only the owner calls it, and only once it knows what
	// its write did.
	ReleaseAccountClosingAttempt(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
	// SetAccountClosedMarker writes the confirmed closing instant as a negative cache
	// entry with its fixed TTL. It is unconditional: the instant comes from the
	// authoritative row, so recomposing it after expiry must not depend on whether a
	// previous entry survived.
	SetAccountClosedMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, closedAt time.Time) error
	// GetAccountClosedMarker returns the cached closing instant. A missing marker
	// returns (zero, false, nil); a marker whose value is not a timestamp returns
	// ErrAccountProtectionMarkerUnreadable.
	GetAccountClosedMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (time.Time, bool, error)
	// AcquireAccountAdminOwnership takes the EXCLUSIVE administrative ownership of
	// one account for the caller's token: the mode of a closing, a balance creation
	// and a balance deletion. It returns false while any other holder is present,
	// whether another exclusive owner or live seed admissions, so none of those
	// operations runs while a cache-miss seed is in flight. Like the closing marker
	// it carries no releasing expiry: ownership of work whose result is unknown is
	// resolved by reconciliation, never by age. A key in a state no writer produces
	// returns ErrAccountProtectionMarkerUnreadable.
	AcquireAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
	// ReleaseAccountAdminOwnership drops the exclusive ownership only when it still
	// carries the caller's token. It never touches seed admissions: a key holding
	// them returns (false, nil).
	ReleaseAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
	// AcquireAccountSeedAdmission adds the caller's token to the SHARED seed
	// admissions of one account, the mode of a cache-miss balance load. Admissions
	// coexist with one another; while an exclusive owner holds the account the call
	// returns (false, AccountAdminHolderExclusive, nil) and writes nothing. Admitting
	// a token twice keeps its first acquisition instant. It carries no releasing
	// expiry, for the same reason as the exclusive mode. A key in a state no writer
	// produces, or an empty token, returns ErrAccountProtectionMarkerUnreadable.
	AcquireAccountSeedAdmission(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, AccountAdminHolder, error)
	// ReleaseAccountSeedAdmission removes only the caller's own admission, and the
	// account's key with it once no admission is left. It never touches an
	// exclusive owner: that key returns (false, nil), as does a token that is not a
	// live admission.
	ReleaseAccountSeedAdmission(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error)
	// ScanAccountClosingMarkers reads one bounded page of the closing-marker
	// namespace. It is how reconciliation finds the attempts a restart interrupted,
	// without an unbounded key read and without depending on anything held in
	// memory. The cursor belongs to the caller.
	ScanAccountClosingMarkers(ctx context.Context, cursor uint64, count int64) (AccountProtectionScanPage, error)
	// ScanAccountAdminOwnerships reads one bounded page of the administrative
	// ownership namespace, where an ownership left behind by an operation other
	// than a closing is discovered.
	ScanAccountAdminOwnerships(ctx context.Context, cursor uint64, count int64) (AccountProtectionScanPage, error)
}

// Compile-time guarantee that the transaction cache repository serves the
// account-protection surface.
var _ AccountProtectionRepository = (*RedisConsumerRepository)(nil)

func (rr *RedisConsumerRepository) AcquireAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return rr.acquireAccountProtection(ctx, utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID), token)
}

// AccountClosingAttempt is what one closing marker records: the token of the
// attempt that owns the account and whether that attempt already issued the write
// that records the closing instant.
//
// WriteIssued is the difference reconciliation turns on. An attempt that never
// issued its write cannot have closed anything, so its abandoned protection may be
// given back. One that did may have committed with the answer lost, so its
// protection stays until the authoritative row says what happened.
type AccountClosingAttempt struct {
	Token       string
	WriteIssued bool
}

func (rr *RedisConsumerRepository) GetAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (string, bool, error) {
	attempt, found, err := rr.ReadAccountClosingAttempt(ctx, organizationID, ledgerID, accountID)

	return attempt.Token, found, err
}

func (rr *RedisConsumerRepository) ReadAccountClosingAttempt(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (AccountClosingAttempt, bool, error) {
	value, err := rr.Get(ctx, utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID))
	if err != nil {
		return AccountClosingAttempt{}, false, err
	}

	if value == "" {
		return AccountClosingAttempt{}, false, nil
	}

	attempt, err := decodeAccountClosingAttempt(value)
	if err != nil {
		return AccountClosingAttempt{}, false, err
	}

	return attempt, true, nil
}

func (rr *RedisConsumerRepository) MarkAccountClosingWriteIssued(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, fmt.Errorf("%w: owner token is empty", ErrAccountProtectionMarkerUnreadable)
	}

	key := utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID)

	return rr.runAccountClosingScript(ctx, markAccountClosingWriteScript, "redis.mark_account_closing_write", key, token, token+accountClosingWriteSuffix)
}

func (rr *RedisConsumerRepository) ReleaseAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return rr.releaseAccountProtection(ctx, utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID), token)
}

func (rr *RedisConsumerRepository) ReleaseAccountClosingAttempt(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, fmt.Errorf("%w: owner token is empty", ErrAccountProtectionMarkerUnreadable)
	}

	key := utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID)

	return rr.runAccountClosingScript(ctx, deleteAccountClosingScript, "redis.release_account_closing_attempt", key, token, token+accountClosingWriteSuffix)
}

// decodeAccountClosingAttempt reads one marker value. An empty token is reported
// as unreadable rather than as an absent marker: a marker that exists but cannot
// be attributed leaves the protection state unknown, and treating that as absence
// would turn a failure into an authorization.
func decodeAccountClosingAttempt(value string) (AccountClosingAttempt, error) {
	trimmed := strings.TrimSpace(value)

	token, writeIssued := strings.CutSuffix(trimmed, accountClosingWriteSuffix)
	if strings.TrimSpace(token) == "" {
		return AccountClosingAttempt{}, fmt.Errorf("%w: closing marker carries no owner token", ErrAccountProtectionMarkerUnreadable)
	}

	return AccountClosingAttempt{Token: token, WriteIssued: writeIssued}, nil
}

// runAccountClosingScript runs one conditional marker script over the tenant's
// key and reports whether it applied.
func (rr *RedisConsumerRepository) runAccountClosingScript(ctx context.Context, script *redis.Script, spanName, key string, args ...any) (bool, error) {
	applied, err := rr.runAccountProtectionScript(ctx, script, spanName, key, args...)

	return applied == accountProtectionScriptApplied, err
}

// runAccountProtectionScript runs one conditional protection script over the
// tenant's key and returns its verdict. A script reports a key in a state no
// writer produces with accountProtectionScriptUnreadable, which becomes
// ErrAccountProtectionMarkerUnreadable here so no caller can read it as a refusal
// or as success. Any other reply outside the three verdicts is a technical error.
func (rr *RedisConsumerRepository) runAccountProtectionScript(ctx context.Context, script *redis.Script, spanName, key string, args ...any) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)

		return accountProtectionScriptRefused, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to connect on redis", err)

		return accountProtectionScriptRefused, err
	}

	result, err := script.Run(ctx, rds, []string{key}, args...).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to run the account protection script", err)

		return accountProtectionScriptRefused, err
	}

	verdict, ok := result.(int64)
	if !ok {
		err = fmt.Errorf("unexpected result type from account protection script: %T", result)

		libOpentelemetry.HandleSpanError(span, "Unexpected result type", err)

		return accountProtectionScriptRefused, err
	}

	switch verdict {
	case accountProtectionScriptApplied, accountProtectionScriptRefused:
		return verdict, nil
	case accountProtectionScriptUnreadable:
		err = fmt.Errorf("%w: the key holds a state no protection writer produces", ErrAccountProtectionMarkerUnreadable)

		libOpentelemetry.HandleSpanError(span, "Account protection key is unreadable", err)

		return accountProtectionScriptRefused, err
	default:
		err = fmt.Errorf("unexpected verdict from account protection script: %d", verdict)

		libOpentelemetry.HandleSpanError(span, "Unexpected result", err)

		return accountProtectionScriptRefused, err
	}
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
