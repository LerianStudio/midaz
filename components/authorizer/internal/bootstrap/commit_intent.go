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

// crossShardCommitTopic is the Redpanda topic where commit intents are persisted.
// The commit log provides durability for the 2PC commit decision: once a commit
// intent is written with status "PREPARED", the coordinator is bound to drive all
// participants to completion. If the coordinator crashes mid-commit, a recovery
// process can read pending intents and retry commits on participants that haven't
// acknowledged yet.
const crossShardCommitTopic = "authorizer.cross-shard.commits"

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
//	PREPARED  → COMMITTED | MANUAL_INTERVENTION_REQUIRED
//	COMMITTED → COMPLETED
//
// Any other transition (including backward or self-transitions) is invalid.
func validStatusTransition(from, to string) bool {
	switch from {
	case commitIntentStatusPrepared:
		// PREPARED -> COMMITTED: at least one participant committed.
		// PREPARED -> COMPLETED: all participants committed atomically in a single pass.
		// PREPARED -> MANUAL_INTERVENTION: unrecoverable state detected.
		return to == commitIntentStatusCommitted || to == commitIntentStatusCompleted || to == commitIntentStatusManualIntervention
	case commitIntentStatusCommitted:
		return to == commitIntentStatusCompleted
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
