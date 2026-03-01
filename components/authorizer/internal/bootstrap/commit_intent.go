// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/publisher"
)

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
		return fmt.Errorf("commit intent publisher is not configured")
	}

	payload, err := json.Marshal(intent)
	if err != nil {
		return fmt.Errorf("marshal commit intent: %w", err)
	}

	headers := make(map[string]string)
	token := ""
	if s != nil {
		token = s.peerAuthToken
	}

	if token != "" {
		mac := hmac.New(sha256.New, []byte(token))
		_, _ = mac.Write(payload)
		headers[commitIntentAuthHeader] = fmt.Sprintf("%x", mac.Sum(nil))
	}

	return s.pub.Publish(ctx, publisher.Message{
		Topic:        crossShardCommitTopic,
		PartitionKey: intent.TransactionID,
		Payload:      payload,
		Headers:      headers,
		ContentType:  "application/json",
	})
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
