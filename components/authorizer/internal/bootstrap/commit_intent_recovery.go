// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"github.com/twmb/franz-go/pkg/kgo"
)

const defaultCommitIntentConsumerGroup = "authorizer-cross-shard-recovery"

type commitIntentRecoveryRunner struct {
	service      *authorizerService
	logger       libLog.Logger
	client       *kgo.Client
	pollTimeout  time.Duration
	consumerName string
}

func newCommitIntentRecoveryRunner(cfg *Config, service *authorizerService, logger libLog.Logger) (*commitIntentRecoveryRunner, error) {
	if cfg == nil {
		return nil, fmt.Errorf("commit intent recovery config is nil")
	}

	if service == nil {
		return nil, fmt.Errorf("commit intent recovery service is nil")
	}

	group := strings.TrimSpace(cfg.CommitIntentConsumerGroup)
	if group == "" {
		group = defaultCommitIntentConsumerGroup
	}

	pollTimeout := cfg.CommitIntentPollTimeout
	if pollTimeout <= 0 {
		pollTimeout = time.Second
	}

	securityOptions, err := brokersecurity.BuildFranzGoOptions(brokersecurity.Config{
		TLSEnabled:            cfg.RedpandaTLSEnabled,
		TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkip,
		TLSCAFile:             cfg.RedpandaTLSCAFile,
		SASLEnabled:           cfg.RedpandaSASLEnabled,
		SASLMechanism:         cfg.RedpandaSASLMechanism,
		SASLUsername:          cfg.RedpandaSASLUsername,
		SASLPassword:          cfg.RedpandaSASLPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("build commit intent recovery security options: %w", err)
	}

	options := []kgo.Opt{
		kgo.SeedBrokers(cfg.RedpandaBrokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(crossShardCommitTopic),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	}

	options = append(options, securityOptions...)

	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("initialize commit intent recovery consumer: %w", err)
	}

	return &commitIntentRecoveryRunner{
		service:      service,
		logger:       logger,
		client:       client,
		pollTimeout:  pollTimeout,
		consumerName: group,
	}, nil
}

func (r *commitIntentRecoveryRunner) Close() {
	if r == nil || r.client == nil {
		return
	}

	r.client.Close()
}

func (r *commitIntentRecoveryRunner) Run(ctx context.Context) {
	if r == nil || r.client == nil {
		return
	}

	r.logger.Infof("Authorizer commit intent recovery consumer started: group=%s topic=%s", r.consumerName, crossShardCommitTopic)

	for {
		if ctx.Err() != nil {
			return
		}

		pollCtx, cancel := context.WithTimeout(ctx, r.pollTimeout)
		fetches := r.client.PollFetches(pollCtx)
		cancel()

		if ctx.Err() != nil {
			return
		}

		for _, fetchErr := range fetches.Errors() {
			if isExpectedPollError(fetchErr.Err) {
				continue
			}

			r.logger.Warnf(
				"Authorizer commit intent recovery poll error: topic=%s partition=%d err=%v",
				fetchErr.Topic,
				fetchErr.Partition,
				fetchErr.Err,
			)
		}

		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()

			processErr := r.processRecord(ctx, record)
			if processErr != nil {
				r.logger.Warnf("Authorizer commit intent recovery record failed: partition=%d offset=%d err=%v", record.Partition, record.Offset, processErr)
				continue
			}

			if err := r.client.CommitRecords(ctx, record); err != nil {
				r.logger.Warnf("Authorizer commit intent recovery commit failed: partition=%d offset=%d err=%v", record.Partition, record.Offset, err)
			}
		}
	}
}

func isExpectedPollError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func (r *commitIntentRecoveryRunner) processRecord(ctx context.Context, record *kgo.Record) error {
	if record == nil || len(record.Value) == 0 {
		return nil
	}

	if !r.verifyRecordAuth(record) {
		r.logger.Warnf("Authorizer commit intent recovery skipped unauthenticated payload: partition=%d offset=%d", record.Partition, record.Offset)
		return nil
	}

	var intent commitIntent
	if err := json.Unmarshal(record.Value, &intent); err != nil {
		r.logger.Warnf("Authorizer commit intent recovery skipped invalid payload: partition=%d offset=%d err=%v", record.Partition, record.Offset, err)
		return nil
	}

	if strings.TrimSpace(intent.TransactionID) == "" {
		r.logger.Warnf("Authorizer commit intent recovery skipped payload with empty transaction_id: partition=%d offset=%d", record.Partition, record.Offset)
		return nil
	}

	return r.service.recoverCommitIntent(ctx, &intent)
}

func (r *commitIntentRecoveryRunner) verifyRecordAuth(record *kgo.Record) bool {
	if r == nil || r.service == nil || record == nil {
		return false
	}

	token := strings.TrimSpace(r.service.peerAuthToken)
	if token == "" {
		return true
	}

	headers := make(map[string]string, len(record.Headers))
	for _, header := range record.Headers {
		headers[strings.ToLower(strings.TrimSpace(header.Key))] = strings.TrimSpace(string(header.Value))
	}

	provided := headers[commitIntentAuthHeader]
	if provided == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write(record.Value)
	expected := fmt.Sprintf("%x", mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1 {
		return true
	}

	previous := strings.TrimSpace(r.service.peerAuthTokenPrev)
	if previous == "" {
		return false
	}

	mac = hmac.New(sha256.New, []byte(previous))
	_, _ = mac.Write(record.Value)
	expected = fmt.Sprintf("%x", mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *authorizerService) recoverCommitIntent(ctx context.Context, intent *commitIntent) error {
	if s == nil || intent == nil {
		return nil
	}

	if intent.Status == commitIntentStatusCompleted {
		return nil
	}

	if intent.Status == commitIntentStatusManualIntervention {
		return nil
	}

	if len(intent.Participants) == 0 {
		return nil
	}

	committedAny := false

	for i := range intent.Participants {
		participant := &intent.Participants[i]
		if participant.Committed {
			committedAny = true
			continue
		}

		if strings.TrimSpace(participant.PreparedTxID) == "" {
			return fmt.Errorf("participant missing prepared_tx_id")
		}

		ownedByThisInstance := s.isOwnedLocalParticipant(*participant)

		if participant.IsLocal && !ownedByThisInstance {
			if s.logger != nil {
				s.logger.Warnf(
					"Authorizer commit intent recovery skipping local-marked participant not owned by this instance: transaction_id=%s participant_addr=%s local_addr=%s prepared_tx_id=%s",
					intent.TransactionID,
					participant.InstanceAddr,
					s.localInstanceAddr(),
					participant.PreparedTxID,
				)
			}

			continue
		}

		if ownedByThisInstance {
			if _, err := s.engine.CommitPrepared(participant.PreparedTxID); err != nil {
				if errors.Is(err, engine.ErrPreparedTxNotFound) {
					s.logger.Errorf(
						"Authorizer commit intent recovery requires manual intervention: transaction_id=%s prepared_tx_id=%s reason=prepared_state_missing",
						intent.TransactionID,
						participant.PreparedTxID,
					)

					intent.Status = commitIntentStatusManualIntervention
					if publishErr := s.publishCommitIntent(context.Background(), intent); publishErr != nil {
						return fmt.Errorf("publish recovery manual intervention status: %w", publishErr)
					}

					return nil
				}

				return fmt.Errorf("recover local commit prepared_tx_id=%s: %w", participant.PreparedTxID, err)
			}
		} else {
			peer := s.peerByAddr(participant.InstanceAddr)
			if peer == nil {
				return fmt.Errorf("recover peer commit failed: peer %q not configured", participant.InstanceAddr)
			}

			commitCtx, cancel := context.WithTimeout(ctx, s.resolveCommitDeadline())
			commitReq := &authorizerv1.CommitPreparedRequest{PreparedTxId: participant.PreparedTxID}
			authCtx, authErr := withPeerAuth(commitCtx, s.peerAuthToken, peerRPCMethodCommitPrepared, commitReq)
			if authErr != nil {
				cancel()
				return fmt.Errorf("recover peer commit auth peer=%s prepared_tx_id=%s: %w", peer.addr, participant.PreparedTxID, authErr)
			}

			_, err := peer.client.CommitPrepared(authCtx, commitReq)
			cancel()
			if err != nil {
				return fmt.Errorf("recover peer commit peer=%s prepared_tx_id=%s: %w", peer.addr, participant.PreparedTxID, err)
			}
		}

		participant.Committed = true
		committedAny = true
	}

	allCommitted := true
	for i := range intent.Participants {
		if !intent.Participants[i].Committed {
			allCommitted = false
			break
		}
	}

	if allCommitted {
		intent.Status = commitIntentStatusCompleted
	} else if committedAny {
		intent.Status = commitIntentStatusCommitted
	}

	if err := s.publishCommitIntent(context.Background(), intent); err != nil {
		return fmt.Errorf("publish recovery commit intent status: %w", err)
	}

	return nil
}

func (s *authorizerService) peerByAddr(addr string) *peerClient {
	if s == nil {
		return nil
	}

	target := strings.TrimSpace(addr)
	if target == "" {
		return nil
	}

	for _, peer := range s.peers {
		if peer == nil {
			continue
		}

		if addrEquivalent(peer.addr, target) {
			return peer
		}
	}

	return nil
}

func (s *authorizerService) localInstanceAddr() string {
	if s == nil {
		return ""
	}

	if trimmed := strings.TrimSpace(s.instanceAddr); trimmed != "" {
		return trimmed
	}

	return strings.TrimSpace(s.grpcAddr)
}

func (s *authorizerService) isOwnedLocalParticipant(participant commitParticipant) bool {
	if s == nil {
		return false
	}

	participantAddr := strings.TrimSpace(participant.InstanceAddr)
	if participantAddr == "" {
		if len(s.peers) == 0 {
			return true
		}

		return participant.IsLocal
	}

	if addrEquivalent(participantAddr, s.localInstanceAddr()) {
		return true
	}

	if localGRPC := strings.TrimSpace(s.grpcAddr); localGRPC != "" && addrEquivalent(participantAddr, localGRPC) {
		return true
	}

	return false
}

func addrEquivalent(left, right string) bool {
	leftHost, leftPort := splitHostPort(strings.TrimSpace(left))
	rightHost, rightPort := splitHostPort(strings.TrimSpace(right))

	if !strings.EqualFold(leftHost, rightHost) {
		return false
	}

	if leftPort == "" || rightPort == "" {
		return true
	}

	return leftPort == rightPort
}

func splitHostPort(addr string) (string, string) {
	if addr == "" {
		return "", ""
	}

	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		return strings.TrimSpace(host), strings.TrimSpace(port)
	}

	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(addr, "]"), "[")), ""
	}

	if strings.Count(addr, ":") == 0 {
		return strings.TrimSpace(addr), ""
	}

	return strings.TrimSpace(addr), ""
}
