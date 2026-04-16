// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// ErrPreparedIntentReplayFailed is returned when the prepared-intent replay
// path cannot rebuild a previously-persisted prepare — either because the
// persisted request is malformed, the balance state required by the prepare
// no longer exists, or the re-prepare itself rejects the operations. The
// bootstrap sequence treats this as a fatal error because proceeding would
// open the same double-spend window the persistence is designed to close
// (D1 audit finding #2).
var ErrPreparedIntentReplayFailed = errors.New("prepared-intent replay failed")

// persistPreparedIntent appends a WAL entry describing a freshly-prepared 2PC
// transaction so the in-memory prepStore entry can be rebuilt after a crash /
// restart. Called immediately after PrepareAuthorize returns a non-nil
// PreparedTx; the WAL append MUST complete before the prepare response is
// returned to the caller, otherwise a crash between PREPARED-in-memory and
// PREPARED-on-disk would re-create the exact gap this feature closes.
//
// The function is forgiving on non-fatal failure: if the WAL append fails the
// caller (gRPC handler) MUST decide whether to proceed (sacrificing crash
// recovery for that specific transaction) or fail the prepare loudly.
// Keeping this as a non-fatal helper lets the existing WAL write-error
// metrics carry the signal so operators can monitor append failure rates
// without introducing a second error path.
func persistPreparedIntent(eng *engine.Engine, ptx *engine.PreparedTx, logger libLog.Logger) error {
	if eng == nil || ptx == nil || ptx.Request == nil {
		return nil
	}

	requestBytes, err := protojson.Marshal(ptx.Request)
	if err != nil {
		return fmt.Errorf("marshal prepared request for wal: %w", err)
	}

	intent := &wal.PreparedIntent{
		PreparedTxID: ptx.ID,
		RequestJSON:  requestBytes,
		PreparedAt:   time.Now().UTC(),
	}

	entry := wal.Entry{
		TransactionID:  ptx.Request.GetTransactionId(),
		OrganizationID: ptx.Request.GetOrganizationId(),
		LedgerID:       ptx.Request.GetLedgerId(),
		Pending:        ptx.Request.GetPending(),
		CrossShard:     ptx.CrossShard,
		Participants:   ptx.Participants,
		CreatedAt:      intent.PreparedAt,
		PreparedIntent: intent,
	}

	if err := eng.AppendWALEntry(entry); err != nil {
		if logger != nil {
			logger.Warnf("Authorizer WAL prepared-intent append failed for tx=%s: %v", ptx.ID, err)
		}

		return fmt.Errorf("append prepared intent wal: %w", err)
	}

	return nil
}

// replayPreparedIntents re-creates in-memory PreparedTx state for any
// prepared-intent WAL entries that were not followed by a commit entry
// for the same transaction_id. The replay is part of cold-start bootstrap
// and runs AFTER ReplayEntries so the engine's balance state reflects
// all committed mutations before we start re-acquiring locks.
//
// Preconditions at call time:
//   - eng has fully loaded balances (LoadBalances completed)
//   - eng has already applied every committed entry via ReplayEntries
//
// On completion, every un-committed prepared intent is either re-prepared
// against the current balance state (restoring locks + prepStore entry) or
// reported via logger if the replay fails. Strict-mode bootstrap returns
// ErrPreparedIntentReplayFailed on any individual failure; lenient mode
// logs and continues so a single malformed entry does not pin the service
// into a crash loop.
func replayPreparedIntents(eng *engine.Engine, entries []wal.Entry, logger libLog.Logger, strict bool, preparedTimeout time.Duration) error {
	if eng == nil || len(entries) == 0 {
		return nil
	}

	// Build a set of transaction IDs that have a committed entry. A committed
	// entry is an Entry with non-empty Mutations (ReplayEntries consumes
	// those for balance mutation). Prepared intents that share a transaction
	// ID with a committed entry are treated as already resolved and NOT
	// replayed — the commit has mutated balances; re-preparing against
	// already-mutated balances would over-count.
	committedTxIDs := collectCommittedTxIDs(entries)
	now := time.Now().UTC()

	var stats replayStats

	for _, e := range entries {
		if e.PreparedIntent == nil {
			continue
		}

		if err := processPreparedIntent(eng, e, committedTxIDs, now, preparedTimeout, logger, strict, &stats); err != nil {
			return err
		}
	}

	if logger != nil {
		logger.Infof("Authorizer prepared-intent replay complete: replayed=%d skipped=%d failed=%d",
			stats.replayed, stats.skipped, stats.failed)
	}

	return nil
}

type replayStats struct {
	replayed, skipped, failed int
}

// collectCommittedTxIDs returns the set of transaction IDs that have a
// committed (mutation-carrying) WAL entry. Prepared intents for those IDs
// are skipped during replay because ReplayEntries has already mutated the
// balance state; re-preparing would over-count.
func collectCommittedTxIDs(entries []wal.Entry) map[string]struct{} {
	out := make(map[string]struct{})

	for _, e := range entries {
		if e.PreparedIntent == nil && len(e.Mutations) > 0 && e.TransactionID != "" {
			out[e.TransactionID] = struct{}{}
		}
	}

	return out
}

// processPreparedIntent handles one prepared-intent entry during replay. It
// updates stats, logs operational warnings, and surfaces errors in strict
// mode. Lenient mode swallows per-entry failures so a single malformed
// intent cannot pin the service into a crash loop.
func processPreparedIntent(
	eng *engine.Engine,
	entry wal.Entry,
	committedTxIDs map[string]struct{},
	now time.Time,
	preparedTimeout time.Duration,
	logger libLog.Logger,
	strict bool,
	stats *replayStats,
) error {
	intent := entry.PreparedIntent

	if _, committed := committedTxIDs[entry.TransactionID]; committed {
		stats.skipped++
		return nil
	}

	// Abort prepared intents that have already passed the prepare timeout.
	// Re-acquiring locks for them would leak locks indefinitely because
	// the auto-abort reaper would only release them after another full
	// prepareTimeout interval — doubling the stuck window.
	if preparedTimeout > 0 && now.Sub(intent.PreparedAt) > preparedTimeout {
		if logger != nil {
			logger.Warnf("Authorizer prepared-intent replay skipping expired tx=%s age=%s timeout=%s",
				intent.PreparedTxID, now.Sub(intent.PreparedAt), preparedTimeout)
		}

		stats.skipped++

		return nil
	}

	if err := rehydratePreparedIntent(eng, intent); err != nil {
		stats.failed++

		if logger != nil {
			logger.Errorf("Authorizer prepared-intent replay failed tx=%s: %v", intent.PreparedTxID, err)
		}

		if strict {
			return fmt.Errorf("%w: tx=%s: %s", ErrPreparedIntentReplayFailed, intent.PreparedTxID, err.Error())
		}

		return nil
	}

	stats.replayed++

	return nil
}

// rehydratePreparedIntent decodes the persisted AuthorizeRequest and re-runs
// PrepareAuthorize against the now-loaded engine. On success the resulting
// PreparedTx is re-keyed to match the originally-persisted prepared_tx_id
// so the coordinator's post-restart CommitPrepared call finds the restored
// entry.
func rehydratePreparedIntent(eng *engine.Engine, intent *wal.PreparedIntent) error {
	if intent == nil {
		return fmt.Errorf("%w", constant.ErrPreparedIntentNil)
	}

	if len(intent.RequestJSON) == 0 {
		return fmt.Errorf("%w", constant.ErrPreparedIntentEmptyRequest)
	}

	if intent.PreparedTxID == "" {
		return fmt.Errorf("%w", constant.ErrPreparedIntentEmptyTxID)
	}

	// Idempotency guard: a previous replay pass (or an already-running service
	// that consumed the intent before this replay) may have already restored
	// the entry. In that case, noop.
	if existing := eng.LookupPreparedTxByID(intent.PreparedTxID); existing != nil {
		return nil
	}

	var req authorizerv1.AuthorizeRequest
	if err := protojson.Unmarshal(intent.RequestJSON, &req); err != nil {
		return fmt.Errorf("unmarshal prepared request: %w", err)
	}

	ptx, resp, err := eng.PrepareAuthorize(&req)
	if err != nil {
		return fmt.Errorf("re-prepare: %w", err)
	}

	// Two rejection modes matter here:
	//
	//   (a) resp != nil, Authorized=false  →  engine refused the prepare. The
	//       world changed between pre-crash prepare and restart (balance
	//       mutated, policy updated, etc). Surface the rejection so operators
	//       or the coordinator can escalate.
	//
	//   (b) resp != nil, Authorized=true, ptx == nil  →  degenerate case
	//       (empty-operations request); nothing to restore.
	//
	// Case (a) is the failure path; case (b) is a silent success (no lock
	// state to recover).
	if resp != nil && !resp.GetAuthorized() {
		return fmt.Errorf("%w: code=%s msg=%s",
			constant.ErrPreparedIntentReplayRejected,
			resp.GetRejectionCode(), resp.GetRejectionMessage())
	}

	if ptx == nil {
		return nil // empty-ops request — nothing to restore
	}

	if err := eng.RestorePreparedTxWithID(intent.PreparedTxID, ptx); err != nil {
		return fmt.Errorf("restore prepared tx id: %w", err)
	}

	return nil
}
