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
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/publisher"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// Sentinel errors for commit intent recovery validation. Note:
// errInvalidCommitIntentTransition lives in commit_intent.go because
// escalateToManualIntervention raises it and callers in multiple files use
// errors.Is against it.
var (
	errCommitIntentRecoveryConfigNil  = errors.New("commit intent recovery config is nil")
	errCommitIntentRecoveryServiceNil = errors.New("commit intent recovery service is nil")
	errParticipantMissingPreparedTxID = errors.New("participant missing prepared_tx_id")
	errRecoverPeerNotConfigured       = errors.New("recover peer commit failed: peer not configured")
	errRecoverPeerNoGRPCClient        = errors.New("recover peer commit failed: no available gRPC client")
	errRemotePreparedTxNotFound       = errors.New("remote prepared tx not found (requires manual intervention)")
	errRecoveryEscalatedToManual      = errors.New("recovery escalated to manual intervention")
)

const defaultCommitIntentConsumerGroup = "authorizer-cross-shard-recovery"

// CommitRecords retry + DLQ controls.
const (
	// commitRecordsMaxRetries caps the exponential-backoff retry budget for
	// transient CommitRecords failures (network, broker unavailable). After
	// exhaustion the record is routed to the DLQ topic so the consumer does
	// not stall forever on a single poison offset.
	commitRecordsMaxRetries          = 5
	commitRecordsBackoffInitial      = 200 * time.Millisecond
	commitRecordsBackoffMax          = 5 * time.Second
	commitRecordsBackoffMaxExponent  = 20
	shutdownWaitPollInterval         = 100 * time.Millisecond
	defaultShutdownWaitTimeoutForRun = 30 * time.Second
)

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

	// stopping signals Run() to exit cleanly on the next loop iteration.
	// Close() sets it and then waits for running to become false (bounded by
	// shutdownWaitTimeout). This prevents zombie in-flight recovery work
	// during pod rollover.
	stopping atomic.Bool
	running  atomic.Bool

	// shutdownWaitTimeout bounds how long Close() waits for Run() to exit
	// cleanly before returning. Defaults to defaultShutdownWaitTimeoutForRun
	// when zero.
	shutdownWaitTimeout time.Duration
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
		service:             service,
		logger:              logger,
		client:              client,
		pollTimeout:         pollTimeout,
		consumerName:        group,
		shutdownWaitTimeout: defaultShutdownWaitTimeoutForRun,
	}, nil
}

// Close signals Run() to stop, waits for it to exit cleanly (bounded by
// shutdownWaitTimeout), and closes the underlying Kafka consumer. Idempotent
// and safe to call multiple times. Prevents zombie in-flight recovery
// iterations during pod rollover — without the wait, the kgo client could be
// closed while a goroutine was mid-fetch, triggering spurious error logs that
// obscured real shutdown-time failures.
func (r *commitIntentRecoveryRunner) Close() {
	if r == nil {
		return
	}

	r.stopping.Store(true)

	waitTimeout := r.shutdownWaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = defaultShutdownWaitTimeoutForRun
	}

	deadline := time.Now().Add(waitTimeout)

	for r.running.Load() && time.Now().Before(deadline) {
		time.Sleep(shutdownWaitPollInterval)
	}

	if r.running.Load() && r.logger != nil {
		r.logger.Warnf(
			"commit intent recovery runner: shutdown wait timed out after %s; forcing consumer close while Run is still active",
			waitTimeout,
		)
	}

	if r.client != nil {
		r.client.Close()
	}
}

// Run starts the commit intent recovery consumer loop.
func (r *commitIntentRecoveryRunner) Run(ctx context.Context) {
	if r == nil || r.client == nil {
		return
	}

	r.running.Store(true)
	defer r.running.Store(false)

	r.logger.Infof("Authorizer commit intent recovery consumer started: group=%s topic=%s", r.consumerName, crossShardCommitTopic)

	var consecutiveNoOps int

	for {
		if ctx.Err() != nil {
			return
		}

		if r.stopping.Load() {
			r.logger.Infof("Authorizer commit intent recovery consumer stopping (Close signalled)")
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

		r.commitRecordWithRetry(ctx, record)
	}

	return cycleProcessed, cycleRecovered
}

// commitRecordWithRetry applies exponential backoff to transient CommitRecords
// failures and routes permanent failures (or retries-exhausted) to the DLQ
// topic. The previous behaviour (a single warn-and-continue on error) risked
// pinning the consumer on a poison offset indefinitely — any durable failure
// on a specific record would prevent every subsequent record from committing,
// gradually starving the recovery loop.
//
// Classification:
//   - Permanent (context canceled, non-retryable broker error): route to DLQ
//     immediately. The record is structurally commit-unfit.
//   - Transient (network blip, broker unavailable, generic error): retry with
//     exponential backoff up to commitRecordsMaxRetries. On exhaustion, route
//     to DLQ so the consumer can continue with newer offsets.
//
// DLQ routing is best-effort: a failed DLQ publish emits a DLQ-publish error
// log line, and the record offset is still NOT committed (the consumer will
// re-observe it after rebalance). That is safer than committing a record
// whose business processing was never confirmed.
func (r *commitIntentRecoveryRunner) commitRecordWithRetry(ctx context.Context, record *kgo.Record) {
	var lastErr error

	for attempt := 0; attempt <= commitRecordsMaxRetries; attempt++ {
		if ctx.Err() != nil {
			return
		}

		err := r.client.CommitRecords(ctx, record)
		if err == nil {
			return
		}

		lastErr = err

		if classifyCommitRecordsError(lastErr) == commitRecordsClassPermanent {
			r.routeToDLQ(ctx, record, "permanent", lastErr)
			return
		}

		if attempt == commitRecordsMaxRetries {
			break
		}

		delay := exponentialCommitRecordsDelay(attempt)

		r.logger.Warnf(
			"Authorizer commit intent recovery commit transient failure: partition=%d offset=%d attempt=%d/%d delay=%s err=%v",
			record.Partition, record.Offset, attempt+1, commitRecordsMaxRetries+1, delay, lastErr,
		)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}

	r.routeToDLQ(ctx, record, "retries_exhausted", lastErr)
}

// commitRecordsClass classifies a CommitRecords error as permanent or transient.
type commitRecordsClass int

const (
	commitRecordsClassTransient commitRecordsClass = iota
	commitRecordsClassPermanent
)

// classifyCommitRecordsError treats context cancellation as permanent (we are
// shutting down) and every other error as transient (retryable). This is a
// conservative default — operators can monitor authorizer_commit_records_dlq_total
// to see when retries are exhausting for specific broker failure modes, and
// extend this classifier if a new permanent-class signal emerges from
// production telemetry.
func classifyCommitRecordsError(err error) commitRecordsClass {
	if err == nil {
		return commitRecordsClassTransient
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return commitRecordsClassPermanent
	}

	return commitRecordsClassTransient
}

// exponentialCommitRecordsDelay returns the delay for a given retry attempt,
// doubling each attempt and capped at commitRecordsBackoffMax. Bounded exponent
// prevents overflow when attempts grow (though commitRecordsMaxRetries already
// caps the loop).
func exponentialCommitRecordsDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	if attempt > commitRecordsBackoffMaxExponent {
		attempt = commitRecordsBackoffMaxExponent
	}

	delay := commitRecordsBackoffInitial * (1 << attempt)
	if delay > commitRecordsBackoffMax {
		delay = commitRecordsBackoffMax
	}

	return delay
}

// routeToDLQ publishes the poison record to the commits DLQ topic and
// increments the DLQ counter. Best-effort: a failed DLQ publish is logged at
// ERROR but does not block the consumer loop from progressing.
func (r *commitIntentRecoveryRunner) routeToDLQ(ctx context.Context, record *kgo.Record, reason string, cause error) {
	if r == nil || r.service == nil {
		return
	}

	if r.service.metrics != nil {
		r.service.metrics.RecordCommitRecordsDLQ(ctx, reason)
	}

	if r.logger != nil {
		r.logger.Errorf(
			"Authorizer commit intent recovery routing record to DLQ: partition=%d offset=%d reason=%s cause=%v",
			record.Partition, record.Offset, reason, cause,
		)
	}

	if r.service.pub == nil {
		return
	}

	// Preserve the original headers so DLQ consumers can read the same
	// HMAC signature (and anything else) the original topic carried. Add a
	// dlq-reason header so operators can group records by failure class
	// without re-parsing the payload.
	headers := make(map[string]string, len(record.Headers)+1)
	for _, h := range record.Headers {
		headers[strings.ToLower(strings.TrimSpace(h.Key))] = string(h.Value)
	}

	headers["x-midaz-commit-records-dlq-reason"] = reason
	if cause != nil {
		headers["x-midaz-commit-records-dlq-cause"] = strings.TrimSpace(cause.Error())
	}

	partitionKey := ""
	if record.Key != nil {
		partitionKey = string(record.Key)
	}

	if err := r.service.pub.Publish(ctx, publisher.Message{
		Topic:        crossShardCommitsDLQTopic,
		PartitionKey: partitionKey,
		Payload:      record.Value,
		Headers:      headers,
		ContentType:  "application/json",
	}); err != nil && r.logger != nil {
		r.logger.Errorf(
			"Authorizer commit intent recovery DLQ publish failed (record will be re-observed after rebalance): partition=%d offset=%d err=%v",
			record.Partition, record.Offset, err,
		)
	}
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
			// This is an unrecoverable state: the participant lacks the
			// prepared_tx_id required to drive commit. Emit the SLI counter
			// so operators can alert on it alongside other escalation reasons.
			if s.metrics != nil {
				s.metrics.RecordManualInterventionRequired(ctx, manualInterventionReasonParticipantMissingID)
			}

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

	return s.recoverRemoteParticipant(ctx, intent, participant)
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

	if escalateErr := s.escalateToManualIntervention(ctx, intent, manualInterventionReasonLocalNotFound); escalateErr != nil {
		// Invalid-transition classification is emitted separately so
		// operators can distinguish "state machine violated" from
		// "escalation publish failed" on the counter.
		if errors.Is(escalateErr, errInvalidCommitIntentTransition) && s.metrics != nil {
			s.metrics.RecordManualInterventionRequired(ctx, manualInterventionReasonInvalidTransition)
		}

		return fmt.Errorf("escalate local not-found: %w", escalateErr)
	}

	return errRecoveryEscalatedToManual
}

func (s *authorizerService) recoverRemoteParticipant(
	ctx context.Context,
	intent *commitIntent,
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
			"recover peer commit: remote prepared_tx not found (reaped or restarted): peer=%s prepared_tx_id=%s transaction_id=%s",
			peer.addr, participant.PreparedTxID, intent.TransactionID,
		)

		// Symmetric escalation: the local-not-found path escalates via
		// escalateToManualIntervention. The remote-not-found path MUST match
		// that behaviour so operators see a uniform signal and the DLQ /
		// manual-intervention topic receive every stuck transaction regardless
		// of which side of the 2PC lost its prepared state. Prior to this
		// change, returning errRemotePreparedTxNotFound left the intent in
		// PREPARED/COMMITTED status and the counter silent — the fix below
		// preserves the sentinel for callers that switch on it while also
		// driving durable escalation.
		if escalateErr := s.escalateToManualIntervention(ctx, intent, manualInterventionReasonRemoteNotFound); escalateErr != nil {
			if errors.Is(escalateErr, errInvalidCommitIntentTransition) && s.metrics != nil {
				s.metrics.RecordManualInterventionRequired(ctx, manualInterventionReasonInvalidTransition)
			}

			s.logger.Errorf(
				"recover peer commit: manual-intervention escalation failed: peer=%s prepared_tx_id=%s transaction_id=%s err=%v",
				peer.addr, participant.PreparedTxID, intent.TransactionID, escalateErr,
			)
		}

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
