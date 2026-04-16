// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/publisher"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
)

// errCommitIntentPublisherNotConfigured is returned when publishCommitIntent is
// called but no publisher has been wired into the authorizer service.
var errCommitIntentPublisherNotConfigured = errors.New("commit intent publisher is not configured")

// errInvalidCommitIntentTransition signals an attempt to move a commit intent
// through a state transition not permitted by validStatusTransition. Defined
// here (rather than in commit_intent_recovery.go) because
// escalateToManualIntervention raises it, and recovery + cross_shard call sites
// both errors.Is against it.
var errInvalidCommitIntentTransition = errors.New("invalid commit intent status transition")

// crossShardCommitTopic is the Redpanda topic where commit intents are persisted.
// The commit log provides durability for the 2PC commit decision: once a commit
// intent is written with status "PREPARED", the coordinator is bound to drive all
// participants to completion. If the coordinator crashes mid-commit, a recovery
// process can read pending intents and retry commits on participants that haven't
// acknowledged yet.
const crossShardCommitTopic = "authorizer.cross-shard.commits"

// crossShardManualInterventionTopic is a dedicated, log-compacted Redpanda topic
// carrying ONLY intents that transitioned to MANUAL_INTERVENTION_REQUIRED.
// Operators subscribe here to review stuck transactions without tailing the
// entire commits topic. Log compaction retains the latest state per txID, so
// the topic always reflects the current universe of stuck transactions.
const crossShardManualInterventionTopic = "authorizer.cross-shard.manual-intervention"

// crossShardCommitsDLQTopic receives poison records that could not be
// processed by the commit intent recovery consumer even after exponential
// backoff retries. Operators inspect this topic to drive manual recovery
// for records that the normal recovery loop cannot handle (malformed
// payloads, permanently invalid state transitions, etc.).
const crossShardCommitsDLQTopic = "authorizer.cross-shard.commits.dlq"

// Manual-intervention reason labels used by the
// authorizer_manual_intervention_required_total counter and the
// manual-intervention topic. These MUST stay stable — they drive operator
// alerting dashboards.
const (
	manualInterventionReasonLocalNotFound        = "local_not_found"
	manualInterventionReasonRemoteNotFound       = "remote_not_found"
	manualInterventionReasonInvalidTransition    = "invalid_transition"
	manualInterventionReasonParticipantMissingID = "participant_missing_txid"
)

const (
	commitIntentStatusPrepared           = "PREPARED"
	commitIntentStatusCommitted          = "COMMITTED"
	commitIntentStatusCompleted          = "COMPLETED"
	commitIntentStatusManualIntervention = "MANUAL_INTERVENTION_REQUIRED"
	commitIntentAuthHeader               = "x-midaz-commit-intent-signature"
)

// commitIntent records the coordinator's decision to commit a cross-shard
// transaction. It is written to Redpanda between the prepare and commit phases
// of the 2PC protocol, making the commit decision durable even if the coordinator
// crashes.
//
// Status transitions: PREPARED → COMMITTED → COMPLETED
//
//   - PREPARED: all participants have successfully prepared; coordinator is about
//     to issue CommitPrepared to each.
//   - COMMITTED: at least one participant has committed (written for crash
//     recovery — the commit decision is irrevocable).
//   - COMPLETED: all participants have committed successfully.
type commitIntent struct {
	TransactionID  string              `json:"transaction_id"`
	OrganizationID string              `json:"organization_id"`
	LedgerID       string              `json:"ledger_id"`
	Status         string              `json:"status"`
	Participants   []commitParticipant `json:"participants"`
	CreatedAt      time.Time           `json:"created_at"`
}

// commitParticipant tracks a single participant (local or remote) in the 2PC
// protocol. The IsLocal flag distinguishes the coordinator's own engine from
// remote peers, which is critical for recovery: local commits are idempotent via
// WAL replay, while remote commits require a gRPC CommitPrepared call.
type commitParticipant struct {
	InstanceAddr string `json:"instance_addr"`
	PreparedTxID string `json:"prepared_tx_id"`
	Committed    bool   `json:"committed"`
	IsLocal      bool   `json:"is_local"`
}

// buildParticipants constructs the participant list from prepare results. Each
// participant's PreparedTxID is captured so recovery can issue targeted
// CommitPrepared calls to the exact prepared transaction on each instance.
func buildParticipants(results []prepareResult, localAddr string) []commitParticipant {
	participants := make([]commitParticipant, 0, len(results))

	for i := range results {
		r := &results[i]
		if r.txID == "" {
			continue
		}

		p := commitParticipant{
			PreparedTxID: r.txID,
			IsLocal:      r.isLocal,
		}

		if r.isLocal {
			p.InstanceAddr = localAddr
		} else if r.peer != nil {
			p.InstanceAddr = r.peer.addr
		}

		participants = append(participants, p)
	}

	return participants
}

// publishManualIntervention serializes the same commit-intent payload to the
// dedicated manual-intervention topic. This is a best-effort publish: the
// primary durability contract is still the commits topic (publishCommitIntent).
// Publishing to this dedicated topic is a FAN-OUT for operator tooling — a
// failure here MUST NOT mask a successful primary publish, so errors are
// logged and ignored by callers that have already written to the commits
// topic.
func (s *authorizerService) publishManualIntervention(ctx context.Context, intent *commitIntent) error {
	if s.pub == nil {
		return errCommitIntentPublisherNotConfigured
	}

	payload, err := json.Marshal(intent)
	if err != nil {
		return fmt.Errorf("marshal manual-intervention intent: %w", err)
	}

	headers := make(map[string]string)
	token := s.peerAuthToken

	if token != "" {
		mac := hmac.New(sha256.New, []byte(token))
		_, _ = mac.Write(payload)
		headers[commitIntentAuthHeader] = hex.EncodeToString(mac.Sum(nil))
	}

	if err := s.pub.Publish(ctx, publisher.Message{
		Topic:        crossShardManualInterventionTopic,
		PartitionKey: intent.TransactionID,
		Payload:      payload,
		Headers:      headers,
		ContentType:  "application/json",
	}); err != nil {
		return fmt.Errorf("publish manual intervention: %w", err)
	}

	return nil
}

// publishCommitIntent serializes a commit intent and writes it to the
// cross-shard commit topic in Redpanda. The transaction ID is used as the
// partition key, guaranteeing that all status updates for a given transaction
// land on the same partition in order.
//
// A configured publisher is required when this method is called. In peer mode,
// failing to persist commit intent means the coordinator cannot safely enter
// the commit phase.
func (s *authorizerService) publishCommitIntent(ctx context.Context, intent *commitIntent) error {
	if s.pub == nil {
		return errCommitIntentPublisherNotConfigured
	}

	payload, err := json.Marshal(intent)
	if err != nil {
		return fmt.Errorf("marshal commit intent: %w", err)
	}

	headers := make(map[string]string)
	token := s.peerAuthToken

	if token != "" {
		mac := hmac.New(sha256.New, []byte(token))
		_, _ = mac.Write(payload)
		headers[commitIntentAuthHeader] = hex.EncodeToString(mac.Sum(nil))
	}

	if err := s.pub.Publish(ctx, publisher.Message{
		Topic:        crossShardCommitTopic,
		PartitionKey: intent.TransactionID,
		Payload:      payload,
		Headers:      headers,
		ContentType:  "application/json",
	}); err != nil {
		return fmt.Errorf("publish commit intent: %w", err)
	}

	return nil
}

// escalateToManualIntervention is the single entry point for transitioning
// a commit intent into MANUAL_INTERVENTION_REQUIRED. It:
//
//  1. validates the transition against the state machine (returns an error
//     if invalid; callers MUST treat this as a programming error and log);
//  2. sets intent.Status = MANUAL_INTERVENTION_REQUIRED;
//  3. publishes to the primary commits topic (durability contract);
//  4. best-effort publishes to the dedicated manual-intervention topic
//     (operator fan-out; failure is logged but does not break the call);
//  5. increments authorizer_manual_intervention_required_total with the
//     supplied reason label.
//
// The reason parameter MUST be one of the manualInterventionReason* constants
// so the SLI counter stays bounded in cardinality.
func (s *authorizerService) escalateToManualIntervention(ctx context.Context, intent *commitIntent, reason string) error {
	if s == nil || intent == nil {
		return fmt.Errorf("escalate to manual intervention: %w", errCommitIntentPublisherNotConfigured)
	}

	// Only validate forward transitions. If the intent is already in
	// MANUAL_INTERVENTION_REQUIRED (e.g., the recovery runner observed an
	// intent that was escalated by a prior pass), the call is a no-op so
	// republishing does not double-count the counter.
	if intent.Status == commitIntentStatusManualIntervention {
		return nil
	}

	if !validStatusTransition(intent.Status, commitIntentStatusManualIntervention) {
		return fmt.Errorf(
			"%w: from=%s to=%s tx_id=%s",
			errInvalidCommitIntentTransition, intent.Status, commitIntentStatusManualIntervention, intent.TransactionID,
		)
	}

	intent.Status = commitIntentStatusManualIntervention

	// Primary publish: commits topic. This is the durability contract. If
	// this fails, the caller MUST treat the escalation as failed so recovery
	// can retry on the next cycle.
	if err := s.publishCommitIntent(ctx, intent); err != nil {
		return fmt.Errorf("publish manual intervention to commits topic: %w", err)
	}

	// Secondary publish: manual-intervention topic. Best-effort — a failure
	// here does not roll back the escalation because the primary publish
	// already succeeded.
	if err := s.publishManualIntervention(ctx, intent); err != nil && s.logger != nil {
		s.logger.Warnf(
			"manual-intervention topic publish failed (non-fatal; commits topic succeeded): tx_id=%s err=%v",
			intent.TransactionID, err,
		)
	}

	// Counter: the SLI operators alert on. Increment after both publishes
	// so the metric cannot race ahead of the durable state.
	if s.metrics != nil {
		s.metrics.RecordManualInterventionRequired(ctx, reason)
	}

	return nil
}

// walParticipantsFromIntent converts a commitIntent's participant list into the
// WAL-compatible representation. This is used to stamp cross-shard metadata onto
// the PreparedTx before commit, so the WAL entry captures the full 2PC lineage.
func walParticipantsFromIntent(intent *commitIntent) []wal.WALParticipant {
	if intent == nil || len(intent.Participants) == 0 {
		return nil
	}

	participants := make([]wal.WALParticipant, 0, len(intent.Participants))

	for _, p := range intent.Participants {
		participants = append(participants, wal.WALParticipant{
			InstanceAddr: p.InstanceAddr,
			PreparedTxID: p.PreparedTxID,
			IsLocal:      p.IsLocal,
		})
	}

	return participants
}

// validStatusTransition checks that commit intent status transitions follow
// the allowed state machine:
//
//	PREPARED  → COMMITTED | COMPLETED | MANUAL_INTERVENTION_REQUIRED
//	COMMITTED → COMPLETED | MANUAL_INTERVENTION_REQUIRED
//
// Any other transition (including backward or self-transitions) is invalid.
// The COMMITTED → MANUAL_INTERVENTION_REQUIRED edge is deliberate: after the
// local participant commits (moving the intent to COMMITTED), a remote peer
// commit can still fail with NotFound — at which point the transaction is
// partially committed and requires operator action. Before this edge existed,
// the escalation silently dropped the status update on COMMITTED intents and
// the manual-intervention topic + counter never fired.
func validStatusTransition(from, to string) bool {
	switch from {
	case commitIntentStatusPrepared:
		// PREPARED -> COMMITTED: at least one participant committed.
		// PREPARED -> COMPLETED: all participants committed atomically in a single pass.
		// PREPARED -> MANUAL_INTERVENTION: unrecoverable state detected.
		return to == commitIntentStatusCommitted || to == commitIntentStatusCompleted || to == commitIntentStatusManualIntervention
	case commitIntentStatusCommitted:
		return to == commitIntentStatusCompleted || to == commitIntentStatusManualIntervention
	default:
		return false
	}
}

// clone returns a deep copy of the commitIntent. The Participants slice is
// independently allocated so mutations on the copy do not race with the original.
// Used by asyncCommitPhase to hand a snapshot to startAsyncPublish while
// commitLocalParticipants mutates the original concurrently.
func (ci *commitIntent) clone() *commitIntent {
	cp := *ci
	cp.Participants = make([]commitParticipant, len(ci.Participants))
	copy(cp.Participants, ci.Participants)

	return &cp
}

func markParticipantCommitted(intent *commitIntent, preparedTxID string) bool {
	if intent == nil || preparedTxID == "" {
		return false
	}

	for i := range intent.Participants {
		if intent.Participants[i].PreparedTxID == preparedTxID {
			intent.Participants[i].Committed = true
			return true
		}
	}

	return false
}
