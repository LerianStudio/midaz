// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// BlockedAccountsSource is the durable source of truth the blocked-accounts
// index is rebuilt from — in production, the `account.blocked` column of the
// onboarding PostgreSQL.
//
// The port is declared here, in the package that consumes it, because the index
// it repairs is an implementation detail of this adapter: nothing above the
// repository should have to know that enforcement rides a Redis SET, that the
// SET carries a hydration sentinel, or that a lost sentinel is repaired by
// reading Postgres.
type BlockedAccountsSource interface {
	// ListBlockedAccountIDs returns every live blocked account of the ledger.
	// It MUST be unpaged: a truncated result would rebuild an index that then
	// declares itself complete while missing blocked accounts.
	ListBlockedAccountIDs(ctx context.Context, organizationID, ledgerID uuid.UUID) ([]uuid.UUID, error)
}

// AccountBlockedError is the atomic gate's denial: the transaction touched an
// account carried by the ledger's blocked-accounts index on a leg that had no
// exception grant.
//
// It deliberately stops short of being the client-facing business error. The
// mapping to 0502 belongs to the command layer, which owns the error contract
// and the rejection metric; the adapter's job is to say which account it was,
// in a form the caller can match on without parsing a string.
type AccountBlockedError struct {
	AccountID string
}

func (e AccountBlockedError) Error() string {
	return fmt.Sprintf("account %s is blocked and carries no exception grant for this operation", e.AccountID)
}

// ErrBlockedAccountsIndexUnavailable marks every failure to ESTABLISH the block
// state, as opposed to establishing it and finding a block.
//
// The distinction is the whole point: a denial is a business outcome (0502), an
// unavailable index is an outage. Collapsing the second into the first would
// tell an operator an account is blocked when the truth is that nobody knows —
// and collapsing it the other way, into "not blocked", would be the fail-open
// this design exists to prevent.
var ErrBlockedAccountsIndexUnavailable = errors.New("blocked accounts index unavailable")

// The structured verdicts balance_atomic_operation.lua returns from its check
// pass. They are ordinary return values rather than error replies, so the
// script can refuse a batch without the caller having to distinguish a refusal
// from a Redis failure.
const (
	blockGateNeedsHydrationReply = "NEEDS_HYDRATION"
	blockGateBlockedReplyPrefix  = "BLOCKED:"
)

type blockGateVerdict int

const (
	// blockGateProceed means the reply is the balance payload, not a verdict.
	blockGateProceed blockGateVerdict = iota
	blockGateNeedsHydration
	blockGateBlocked
)

// classifyBlockGateReply separates the gate's verdicts from the balance payload
// the script returns on success.
//
// Anything it does not recognise is passed through as blockGateProceed and left
// to the decoder, which fails loudly on a payload it cannot parse. That is the
// safe direction for an unknown reply: a verdict misread as a payload becomes a
// visible decode error, while a payload misread as a verdict would silently
// discard a completed mutation.
func classifyBlockGateReply(result any) (blockGateVerdict, string) {
	var reply string

	switch value := result.(type) {
	case string:
		reply = value
	case []byte:
		reply = string(value)
	default:
		return blockGateProceed, ""
	}

	if reply == blockGateNeedsHydrationReply {
		return blockGateNeedsHydration, ""
	}

	if accountID, found := strings.CutPrefix(reply, blockGateBlockedReplyPrefix); found {
		return blockGateBlocked, accountID
	}

	return blockGateProceed, ""
}

// resolveBlockGate turns the script's reply into either the balance payload the
// decoder expects or the Go error the verdict means.
//
// An unhydrated index is repaired and the script re-run EXACTLY once. The rerun
// is safe because the gate lives in the script's check pass: a NEEDS_HYDRATION
// reply is returned before the first write, so the first invocation left no
// balance, no schedule entry and no backup-hash record behind. A second
// NEEDS_HYDRATION means the rebuild did not stick, which is an unavailable
// index — never a licence to proceed.
func (rr *RedisConsumerRepository) resolveBlockGate(
	ctx context.Context,
	span trace.Span,
	rds redis.UniversalClient,
	keys []string,
	args []any,
	organizationID, ledgerID uuid.UUID,
	result any,
) (any, error) {
	logger := libObservability.NewLoggerFromContext(ctx)

	verdict, blockedAccountID := classifyBlockGateReply(result)

	if verdict == blockGateNeedsHydration {
		logger.Log(ctx, libLog.LevelWarn, "Blocked accounts index is unhydrated; rebuilding from the source of truth",
			libLog.String("organization_id", organizationID.String()),
			libLog.String("ledger_id", ledgerID.String()))

		if err := rr.rehydrateBlockedAccounts(ctx, organizationID, ledgerID); err != nil {
			return nil, err
		}

		retried, err := rr.runBalanceAtomicScript(ctx, rds, keys, args)
		if err != nil {
			return nil, err
		}

		result = retried
		verdict, blockedAccountID = classifyBlockGateReply(result)

		if verdict == blockGateNeedsHydration {
			err := fmt.Errorf("%w: the index still reports itself unhydrated after a rebuild", ErrBlockedAccountsIndexUnavailable)

			libOpentelemetry.HandleSpanError(span, "Blocked accounts index unhydrated after a rebuild", err)
			logger.Log(ctx, libLog.LevelError, "Blocked accounts index unhydrated after a rebuild", libLog.Err(err))

			return nil, err
		}
	}

	if verdict == blockGateBlocked {
		blockedErr := AccountBlockedError{AccountID: blockedAccountID}

		span.SetAttributes(attribute.String("app.blocked_account_id", blockedAccountID))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction denied by the account-block gate", blockedErr)
		logger.Log(ctx, libLog.LevelWarn, "Transaction denied by the account-block gate",
			libLog.String("account_id", blockedAccountID))

		return nil, blockedErr
	}

	return result, nil
}

// ResolveBlockedAccounts answers which of accountIDs the ledger's
// blocked-accounts index holds, repairing the index once when it reports itself
// unhydrated. See RedisRepository.ResolveBlockedAccounts for the contract.
//
// It is the same repair resolveBlockGate performs for the atomic script, exposed
// as a plain read for the callers that need the block state BEFORE the script
// runs — the commit path, which has to know whether an account-exception
// enrichment is worth its I/O. Keeping the repair here is what lets those
// callers ask a question ("which of these are blocked?") instead of driving a
// hydration protocol they have no business knowing about (decision R2).
func (rr *RedisConsumerRepository) ResolveBlockedAccounts(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	accountIDs []uuid.UUID,
) ([]uuid.UUID, error) {
	// Nothing to ask about: the probe would only be able to answer "no", so the
	// common transaction pays nothing for a gate it does not touch.
	if len(accountIDs) == 0 {
		return nil, nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.resolve_blocked_accounts")
	defer span.End()

	blockedAccountsSpanScope(span, organizationID, ledgerID)

	hydrated, blocked, err := rr.IsHydratedAndBlocked(ctx, organizationID, ledgerID, accountIDs)
	if err != nil {
		// An unreachable index is an outage, not an answer. The wrap is what lets
		// the caller tell it apart from a denial and refuse the request as a 5xx
		// instead of inventing either a block or the absence of one.
		wrapped := fmt.Errorf("%w: %w", ErrBlockedAccountsIndexUnavailable, err)

		libOpentelemetry.HandleSpanError(span, "Failed to read the blocked accounts index", wrapped)

		return nil, wrapped
	}

	if hydrated {
		return blocked, nil
	}

	logger.Log(ctx, libLog.LevelWarn, "Blocked accounts index is unhydrated; rebuilding from the source of truth",
		libLog.String("organization_id", organizationID.String()),
		libLog.String("ledger_id", ledgerID.String()))

	if err := rr.rehydrateBlockedAccounts(ctx, organizationID, ledgerID); err != nil {
		return nil, err
	}

	hydrated, blocked, err = rr.IsHydratedAndBlocked(ctx, organizationID, ledgerID, accountIDs)
	if err != nil {
		wrapped := fmt.Errorf("%w: %w", ErrBlockedAccountsIndexUnavailable, err)

		libOpentelemetry.HandleSpanError(span, "Failed to re-read the blocked accounts index after a rebuild", wrapped)

		return nil, wrapped
	}

	// Re-probed exactly once, never in a loop: a rebuild that did not stick is a
	// broken index, and retrying it here would only turn an outage into a hang.
	if !hydrated {
		err := fmt.Errorf("%w: the index still reports itself unhydrated after a rebuild", ErrBlockedAccountsIndexUnavailable)

		libOpentelemetry.HandleSpanError(span, "Blocked accounts index unhydrated after a rebuild", err)
		logger.Log(ctx, libLog.LevelError, "Blocked accounts index unhydrated after a rebuild", libLog.Err(err))

		return nil, err
	}

	return blocked, nil
}

// rehydrateBlockedAccounts rebuilds one ledger's blocked-accounts index from the
// source of truth.
//
// Every failure path returns ErrBlockedAccountsIndexUnavailable. None of them
// may return nil: the caller reads a nil here as "the index is now
// authoritative" and proceeds to run the script again.
func (rr *RedisConsumerRepository) rehydrateBlockedAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.rehydrate_blocked_accounts")
	defer span.End()

	blockedAccountsSpanScope(span, organizationID, ledgerID)

	// A repository wired without a source cannot repair the index, and an index
	// that cannot be repaired cannot be read as empty: that would turn a lost
	// Redis key into a silent unblock of every blocked account in the ledger.
	if rr.blockedAccountsSource == nil {
		err := fmt.Errorf("%w: no blocked accounts source is configured", ErrBlockedAccountsIndexUnavailable)

		libOpentelemetry.HandleSpanError(span, "No blocked accounts source is configured", err)
		logger.Log(ctx, libLog.LevelError, "No blocked accounts source is configured", libLog.Err(err))

		return err
	}

	accountIDs, err := rr.blockedAccountsSource.ListBlockedAccountIDs(ctx, organizationID, ledgerID)
	if err != nil {
		wrapped := fmt.Errorf("%w: failed to read the blocked accounts source of truth: %w", ErrBlockedAccountsIndexUnavailable, err)

		libOpentelemetry.HandleSpanError(span, "Failed to read the blocked accounts source of truth", wrapped)
		logger.Log(ctx, libLog.LevelError, "Failed to read the blocked accounts source of truth", libLog.Err(err))

		return wrapped
	}

	if err := rr.HydrateBlockedAccounts(ctx, organizationID, ledgerID, accountIDs); err != nil {
		wrapped := fmt.Errorf("%w: %w", ErrBlockedAccountsIndexUnavailable, err)

		libOpentelemetry.HandleSpanError(span, "Failed to rebuild the blocked accounts index", wrapped)

		return wrapped
	}

	span.SetAttributes(attribute.Int("app.blocked_accounts.rehydrated", len(accountIDs)))
	logger.Log(ctx, libLog.LevelInfo, "Blocked accounts index rebuilt from the source of truth",
		libLog.Int("accounts", len(accountIDs)),
		libLog.String("sentinel", utils.BlockedAccountsHydratedMember))

	return nil
}
