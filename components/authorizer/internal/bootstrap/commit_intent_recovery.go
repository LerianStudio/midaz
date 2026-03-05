// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// Sentinel errors for commit intent recovery validation.
var (
	errCommitIntentRecoveryConfigNil  = errors.New("commit intent recovery config is nil")
	errCommitIntentRecoveryServiceNil = errors.New("commit intent recovery service is nil")
	errParticipantMissingPreparedTxID = errors.New("participant missing prepared_tx_id")
	errInvalidCommitIntentTransition  = errors.New("invalid commit intent status transition")
	errRecoverPeerNotConfigured       = errors.New("recover peer commit failed: peer not configured")
	errRecoverPeerNoGRPCClient        = errors.New("recover peer commit failed: no available gRPC client")
	errRemotePreparedTxNotFound       = errors.New("remote prepared tx not found (requires manual intervention)")
	errRecoveryEscalatedToManual      = errors.New("recovery escalated to manual intervention")
)

const defaultCommitIntentConsumerGroup = "authorizer-cross-shard-recovery"

// recoveryBackoff controls exponential backoff when the recovery loop
// processes only skip-able / already-completed records and makes no
// forward progress. This prevents hot-spinning on orphaned 2PC intents
// that belong to other instances.
const (
	recoveryBackoffThreshold   = 3                // consecutive no-op cycles before backoff kicks in
	recoveryBackoffInitial     = 1 * time.Second  // first backoff delay
	recoveryBackoffMax         = 30 * time.Second // upper bound on backoff delay
	recoveryBackoffMaxExponent = 30               // cap exponent to prevent overflow in 1<<exponent
)

type commitIntentRecoveryRunner struct {
	service      *authorizerService
	logger       libLog.Logger
	client       *kgo.Client
	pollTimeout  time.Duration
	consumerName string
}

func newCommitIntentRecoveryRunner(cfg *Config, service *authorizerService, logger libLog.Logger) (*commitIntentRecoveryRunner, error) {
	if cfg == nil {
		return nil, errCommitIntentRecoveryConfigNil
	}

	if service == nil {
		return nil, errCommitIntentRecoveryServiceNil
	}

	group := strings.TrimSpace(cfg.CommitIntentConsumerGroup)
	if group == "" {
		group = defaultCommitIntentConsumerGroup
	}

	pollTimeout := cfg.CommitIntentPollTimeout
	if pollTimeout <= 0 {
		pollTimeout = time.Second
	}

	// WARNING: InsecureSkipVerify propagated from main config.
	// In production, ensure TLS certificate verification is enabled
	// for recovery consumer connections to prevent MITM attacks.
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

	baseOpts := []kgo.Opt{
		kgo.SeedBrokers(cfg.RedpandaBrokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(crossShardCommitTopic),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	}

	options := make([]kgo.Opt, 0, len(baseOpts)+len(securityOptions))
	options = append(options, baseOpts...)
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

// Close shuts down the underlying Kafka consumer.
func (r *commitIntentRecoveryRunner) Close() {
	if r == nil || r.client == nil {
		return
	}

	r.client.Close()
}

// Run starts the commit intent recovery consumer loop.
func (r *commitIntentRecoveryRunner) Run(ctx context.Context) {
	if r == nil || r.client == nil {
		return
	}

	r.logger.Infof("Authorizer commit intent recovery consumer started: group=%s topic=%s", r.consumerName, crossShardCommitTopic)

	var consecutiveNoOps int

	for {
		if ctx.Err() != nil {
			return
		}

		if r.waitBackoff(ctx, consecutiveNoOps) {
			return
		}

		fetches := r.pollFetches(ctx)
		if ctx.Err() != nil {
			return
		}

		r.logFetchErrors(fetches)

		cycleProcessed, cycleRecovered := r.processRecords(ctx, fetches)

		consecutiveNoOps = updateNoOpCounter(consecutiveNoOps, cycleProcessed, cycleRecovered)
	}
}

// waitBackoff sleeps with exponential backoff when the recovery loop is making
// no forward progress. Returns true if the context was cancelled during the wait.
func (r *commitIntentRecoveryRunner) waitBackoff(ctx context.Context, consecutiveNoOps int) bool {
	if consecutiveNoOps < recoveryBackoffThreshold {
		return false
	}

	exponent := consecutiveNoOps - recoveryBackoffThreshold
	delay := recoveryBackoffInitial * (1 << min(exponent, recoveryBackoffMaxExponent))

	if delay > recoveryBackoffMax {
		delay = recoveryBackoffMax
	}

	r.logger.Infof(
		"Authorizer commit intent recovery entering backoff: consecutive_no_ops=%d delay=%s",
		consecutiveNoOps,
		delay,
	)

	select {
	case <-time.After(delay):
		return false
	case <-ctx.Done():
		return true
	}
}

func (r *commitIntentRecoveryRunner) pollFetches(ctx context.Context) kgo.Fetches {
	pollCtx, cancel := context.WithTimeout(ctx, r.pollTimeout)
	defer cancel()

	return r.client.PollFetches(pollCtx)
}

func (r *commitIntentRecoveryRunner) logFetchErrors(fetches kgo.Fetches) {
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
}

func (r *commitIntentRecoveryRunner) processRecords(ctx context.Context, fetches kgo.Fetches) (bool, bool) {
	cycleProcessed := false
	cycleRecovered := false

	iter := fetches.RecordIter()

	for !iter.Done() {
		record := iter.Next()
		cycleProcessed = true

		recovered, processErr := r.processRecord(ctx, record)
		if processErr != nil {
			r.logger.Warnf("Authorizer commit intent recovery record failed: partition=%d offset=%d err=%v", record.Partition, record.Offset, processErr)

			continue
		}

		if recovered {
			cycleRecovered = true
		}

		if err := r.client.CommitRecords(ctx, record); err != nil {
			r.logger.Warnf("Authorizer commit intent recovery commit failed: partition=%d offset=%d err=%v", record.Partition, record.Offset, err)
		}
	}

	return cycleProcessed, cycleRecovered
}

// updateNoOpCounter tracks consecutive cycles with no forward progress.
// An empty poll (no records at all) does not count toward backoff.
func updateNoOpCounter(current int, cycleProcessed, cycleRecovered bool) int {
	if cycleProcessed && !cycleRecovered {
		return current + 1
	}

	if cycleRecovered {
		return 0
	}

	return current
}

func isExpectedPollError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// processRecord handles a single Kafka record from the recovery topic.
// It returns (recovered, err) where recovered indicates whether actual
// recovery work was performed (at least one participant committed). When
// the record is skipped (unauthenticated, invalid, already completed, or
// all participants belong to other instances), recovered is false.
func (r *commitIntentRecoveryRunner) processRecord(ctx context.Context, record *kgo.Record) (bool, error) {
	if record == nil || len(record.Value) == 0 {
		return false, nil
	}

	if !r.verifyRecordAuth(record) {
		r.logger.Warnf("Authorizer commit intent recovery skipped unauthenticated payload: partition=%d offset=%d", record.Partition, record.Offset)
		return false, nil
	}

	var intent commitIntent
	if err := json.Unmarshal(record.Value, &intent); err != nil {
		r.logger.Warnf("Authorizer commit intent recovery skipped invalid payload: partition=%d offset=%d err=%v", record.Partition, record.Offset, err)
		return false, nil
	}

	if strings.TrimSpace(intent.TransactionID) == "" {
		r.logger.Warnf("Authorizer commit intent recovery skipped payload with empty transaction_id: partition=%d offset=%d", record.Partition, record.Offset)
		return false, nil
	}

	recovered, err := r.service.recoverCommitIntent(ctx, &intent)

	return recovered, err
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
	expected := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1 {
		return true
	}

	previous := strings.TrimSpace(r.service.peerAuthTokenPrev)
	if previous == "" {
		return false
	}

	mac = hmac.New(sha256.New, []byte(previous))
	_, _ = mac.Write(record.Value)
	expected = hex.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// recoverCommitIntent attempts to drive an incomplete commit intent to
// completion. It returns (recovered, err) where recovered is true when at
// least one participant was actually committed during this call. A false
// recovered with nil error means the record was a no-op for this instance
// (e.g., already completed, or all participants belong to other instances).
func (s *authorizerService) recoverCommitIntent(ctx context.Context, intent *commitIntent) (bool, error) {
	if s == nil || intent == nil {
		return false, nil
	}

	if intent.Status == commitIntentStatusCompleted {
		return false, nil
	}

	if intent.Status == commitIntentStatusManualIntervention {
		return false, nil
	}

	if len(intent.Participants) == 0 {
		return false, nil
	}

	// anyParticipantCommitted tracks whether at least one participant has
	// been committed (including from prior recovery runs).
	anyParticipantCommitted := false
	// newCommitsThisCall tracks whether this invocation actually performed
	// a new commit, used to signal forward progress to the caller.
	newCommitsThisCall := false

	for i := range intent.Participants {
		participant := &intent.Participants[i]
		if participant.Committed {
			anyParticipantCommitted = true

			continue
		}

		if strings.TrimSpace(participant.PreparedTxID) == "" {
			return newCommitsThisCall, errParticipantMissingPreparedTxID
		}

		ownedByThisInstance := s.isOwnedLocalParticipant(*participant)

		if participant.IsLocal && !ownedByThisInstance {
			s.logSkippedLocalParticipant(intent.TransactionID, participant)

			continue
		}

		if err := s.recoverSingleParticipant(ctx, intent, participant, ownedByThisInstance); err != nil {
			if errors.Is(err, errRecoveryEscalatedToManual) {
				return newCommitsThisCall, nil
			}

			return newCommitsThisCall, err
		}

		participant.Committed = true
		anyParticipantCommitted = true
		newCommitsThisCall = true
	}

	finalizeErr := s.finalizeRecoveryStatus(ctx, intent, anyParticipantCommitted)
	if finalizeErr != nil {
		return newCommitsThisCall, finalizeErr
	}

	return newCommitsThisCall, nil
}

func (s *authorizerService) logSkippedLocalParticipant(txID string, participant *commitParticipant) {
	if s.logger != nil {
		s.logger.Warnf(
			"Authorizer commit intent recovery skipping local-marked participant not owned by this instance: transaction_id=%s participant_addr=%s local_addr=%s prepared_tx_id=%s",
			txID,
			participant.InstanceAddr,
			s.localInstanceAddr(),
			participant.PreparedTxID,
		)
	}
}

// recoverSingleParticipant drives a single uncommitted participant to completion.
// It commits locally owned participants via the engine and remote participants via gRPC.
func (s *authorizerService) recoverSingleParticipant(
	ctx context.Context,
	intent *commitIntent,
	participant *commitParticipant,
	ownedByThisInstance bool,
) error {
	if ownedByThisInstance {
		return s.recoverLocalParticipant(ctx, intent, participant)
	}

	return s.recoverRemoteParticipant(ctx, participant)
}

func (s *authorizerService) recoverLocalParticipant(
	ctx context.Context,
	intent *commitIntent,
	participant *commitParticipant,
) error {
	_, err := s.engine.CommitPrepared(participant.PreparedTxID)
	if err == nil {
		return nil
	}

	if !errors.Is(err, engine.ErrPreparedTxNotFound) {
		return fmt.Errorf("recover local commit prepared_tx_id=%s: %w", participant.PreparedTxID, err)
	}

	s.logger.Errorf(
		"Authorizer commit intent recovery requires manual intervention: transaction_id=%s prepared_tx_id=%s reason=prepared_state_missing",
		intent.TransactionID,
		participant.PreparedTxID,
	)

	if !validStatusTransition(intent.Status, commitIntentStatusManualIntervention) {
		return fmt.Errorf(
			"%w: %s -> %s for transaction_id=%s",
			errInvalidCommitIntentTransition, intent.Status, commitIntentStatusManualIntervention, intent.TransactionID,
		)
	}

	intent.Status = commitIntentStatusManualIntervention

	if publishErr := s.publishCommitIntent(ctx, intent); publishErr != nil {
		return fmt.Errorf("publish recovery manual intervention status: %w", publishErr)
	}

	return errRecoveryEscalatedToManual
}

func (s *authorizerService) recoverRemoteParticipant(
	ctx context.Context,
	participant *commitParticipant,
) error {
	peer := s.peerByAddr(participant.InstanceAddr)
	if peer == nil {
		return fmt.Errorf("%w: %q", errRecoverPeerNotConfigured, participant.InstanceAddr)
	}

	commitCtx, cancel := context.WithTimeout(ctx, s.resolveCommitDeadline())
	defer cancel()

	commitReq := &authorizerv1.CommitPreparedRequest{PreparedTxId: participant.PreparedTxID}

	authCtx, authErr := withPeerAuth(commitCtx, s.peerAuthToken, peerRPCMethodCommitPrepared, commitReq)
	if authErr != nil {
		return fmt.Errorf("recover peer commit auth peer=%s prepared_tx_id=%s: %w", peer.addr, participant.PreparedTxID, authErr)
	}

	recoveryClient := peer.pickClient()
	if recoveryClient == nil {
		return fmt.Errorf("%w: peer %q", errRecoverPeerNoGRPCClient, peer.addr)
	}

	_, err := recoveryClient.CommitPrepared(authCtx, commitReq)
	if err == nil {
		return nil
	}

	if grpcstatus.Code(err) == codes.NotFound {
		s.logger.Errorf(
			"recover peer commit: remote prepared_tx not found (reaped or restarted): peer=%s prepared_tx_id=%s",
			peer.addr, participant.PreparedTxID,
		)

		return fmt.Errorf(
			"%w: peer=%s prepared_tx_id=%s",
			errRemotePreparedTxNotFound, peer.addr, participant.PreparedTxID,
		)
	}

	return fmt.Errorf("recover peer commit peer=%s prepared_tx_id=%s: %w", peer.addr, participant.PreparedTxID, err)
}

// finalizeRecoveryStatus computes and publishes the final status for a recovery pass.
func (s *authorizerService) finalizeRecoveryStatus(
	ctx context.Context,
	intent *commitIntent,
	anyParticipantCommitted bool,
) error {
	allCommitted := true

	for i := range intent.Participants {
		if !intent.Participants[i].Committed {
			allCommitted = false

			break
		}
	}

	var newStatus string
	if allCommitted {
		newStatus = commitIntentStatusCompleted
	} else if anyParticipantCommitted {
		newStatus = commitIntentStatusCommitted
	}

	if newStatus != "" {
		if !validStatusTransition(intent.Status, newStatus) {
			return fmt.Errorf(
				"%w: %s -> %s for transaction_id=%s",
				errInvalidCommitIntentTransition, intent.Status, newStatus, intent.TransactionID,
			)
		}

		intent.Status = newStatus
	}

	if err := s.publishCommitIntent(ctx, intent); err != nil {
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
