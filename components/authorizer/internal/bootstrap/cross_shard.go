// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// Default RPC deadlines when no custom deadline is configured.
const (
	defaultCommitRPCDeadline = 10 * time.Second
	defaultAbortRPCDeadline  = 5 * time.Second
)

// errNoGRPCClientForPeer is returned when a peer has no available gRPC clients.
var errNoGRPCClientForPeer = errors.New("no available gRPC client for peer")

func (s *authorizerService) resolveCommitDeadline() time.Duration {
	if s == nil || s.commitRPCDeadline <= 0 {
		return defaultCommitRPCDeadline
	}

	return s.commitRPCDeadline
}

func (s *authorizerService) resolveAbortDeadline() time.Duration {
	if s == nil || s.abortRPCDeadline <= 0 {
		return defaultAbortRPCDeadline
	}

	return s.abortRPCDeadline
}

// isLocalShard returns true when shard ID falls within this instance's owned range.
func (s *authorizerService) isLocalShard(shardID int) bool {
	return shardID >= s.ownedShardStart && shardID <= s.ownedShardEnd
}

// peerForShard returns the peer client that owns the given shard.
// Returns nil if no peer is configured for this shard (configuration error).
func (s *authorizerService) peerForShard(shardID int) *peerClient {
	for _, p := range s.peers {
		if shardID >= p.shardStart && shardID <= p.shardEnd {
			return p
		}
	}

	return nil
}

// prepareResult captures the outcome of a single PrepareAuthorize call — either
// local (engine) or remote (peer gRPC). Used by authorizeCrossShard to track
// all participants in the 2PC protocol.
type prepareResult struct {
	txID     string
	balances []*authorizerv1.BalanceSnapshot
	resp     *authorizerv1.AuthorizeResponse
	err      error
	isLocal  bool
	peer     *peerClient
}

// authorizeCrossShard implements the coordinator role of the 2PC protocol for
// transactions that span multiple authorizer instances.
//
// Protocol:
//
//  1. PREPARE phase: issue PrepareAuthorize to all participants (local engine +
//     remote peers) in deterministic shard order. Each participant validates operations, acquires
//     deterministic per-balance locks, and returns a prepared transaction ID.
//
//  2. DECISION: if ALL participants report Authorized=true, proceed to COMMIT.
//     If ANY participant rejects or errors, ABORT ALL.
//
//  3. COMMIT phase: issue CommitPrepared to all participants. Each participant
//     writes its WAL entry, mutates live balances, and releases locks.
//
//  4. ABORT phase (failure path): issue AbortPrepared to all participants that
//     returned a prepared transaction ID. Each releases locks without mutation.
//
// The balance snapshots from all participants are merged into a single response.
func (s *authorizerService) authorizeCrossShard(
	ctx context.Context,
	req *authorizerv1.AuthorizeRequest,
	shardOps map[int][]*authorizerv1.BalanceOperation,
) (*authorizerv1.AuthorizeResponse, error) {
	start := time.Now()

	if rejection := s.engine.ValidateRequestLimits(req); rejection != nil {
		s.recordCrossShardRejectionMetrics(ctx, req, rejection, shardOps, start)

		return rejection, nil
	}

	localOps, peerOpsMap, partitionErr := s.partitionShardOps(shardOps)
	if partitionErr != nil {
		return nil, partitionErr
	}

	results, prepareErr := s.executePreparePhase(ctx, req, localOps, peerOpsMap)
	if prepareErr != nil {
		return nil, prepareErr
	}

	if resp, err := s.evaluateDecisionAndAbort(ctx, req, results); resp != nil || err != nil {
		return resp, err
	}

	// ─── PHASE E: DURABLE COMMIT INTENT ────────────────────────────────
	//
	// Write the commit decision to Redpanda BEFORE issuing any commits.
	// This makes the decision durable: if the coordinator crashes mid-commit,
	// a recovery process can read the intent and drive remaining participants
	// to completion. The commit intent is the point of no return — once
	// written, we are committed to driving all participants to completion.

	intent := commitIntent{
		TransactionID:  req.GetTransactionId(),
		OrganizationID: req.GetOrganizationId(),
		LedgerID:       req.GetLedgerId(),
		Status:         commitIntentStatusPrepared,
		Participants:   buildParticipants(results, s.instanceAddr),
		CreatedAt:      time.Now(),
	}

	// ─── PHASE 2: COMMIT (local sequential, remote parallel) ─────────────
	//
	// Commit order: local first, then peers in parallel. If the local commit
	// fails we still have the durable commit intent in Redpanda — recovery
	// can retry. Remote peers are fanned out concurrently via
	// commitAllRemotePeers; latency is bounded by the slowest peer, not the
	// sum. Commit must attempt every peer even on partial failure so recovery
	// has a clear per-peer signal.

	allSnapshots := make([]*authorizerv1.BalanceSnapshot, 0, len(results))

	var commitFailed bool

	committedAny := false

	publishCommittedStatus := func() { //nolint:contextcheck // best-effort publish must survive context cancellation
		intent.Status = commitIntentStatusCommitted
		if err := s.publishCommitIntent(context.Background(), &intent); err != nil {
			s.logger.Warnf(
				"cross-shard COMMITTED intent write failed (non-fatal): tx_id=%s err=%v",
				req.GetTransactionId(),
				err,
			)
		}
	}

	localRes, err := s.runLocalCommitPhase(ctx, results, &intent, req, committedAny, publishCommittedStatus)
	if err != nil {
		return nil, err
	}

	allSnapshots = append(allSnapshots, localRes.snapshots...)
	committedAny = localRes.committedAny
	commitFailed = localRes.failed

	remoteSnapshots, remoteFailed := s.commitAllRemotePeers(ctx, results, req.GetTransactionId(), &intent, &committedAny, publishCommittedStatus)
	allSnapshots = append(allSnapshots, remoteSnapshots...)

	if remoteFailed {
		commitFailed = true
	}

	if commitFailed {
		return nil, s.handleIncompleteCommit(req, &intent)
	}

	s.finalizeCommit(ctx, req, &intent, start, shardOps)

	return &authorizerv1.AuthorizeResponse{
		Authorized: true,
		Balances:   allSnapshots,
	}, nil
}

func (s *authorizerService) recordCrossShardRejectionMetrics(
	ctx context.Context,
	req *authorizerv1.AuthorizeRequest,
	rejection *authorizerv1.AuthorizeResponse,
	shardOps map[int][]*authorizerv1.BalanceOperation,
	start time.Time,
) {
	if !s.metrics.Enabled() {
		return
	}

	pending := false
	txStatus := ""
	operations := 0

	if req != nil {
		pending = req.GetPending()
		txStatus = req.GetTransactionStatus()
		operations = len(req.GetOperations())
	}

	s.metrics.RecordAuthorize(
		ctx,
		"authorize_cross_shard",
		"rejected",
		rejection.GetRejectionCode(),
		pending,
		txStatus,
		operations,
		len(shardOps),
		time.Since(start),
		true,
	)
}

func (s *authorizerService) partitionShardOps(
	shardOps map[int][]*authorizerv1.BalanceOperation,
) ([]*authorizerv1.BalanceOperation, map[*peerClient][]*authorizerv1.BalanceOperation, error) {
	localOps := make([]*authorizerv1.BalanceOperation, 0)
	peerOpsMap := make(map[*peerClient][]*authorizerv1.BalanceOperation)

	for shardID, ops := range shardOps {
		if s.isLocalShard(shardID) {
			localOps = append(localOps, ops...)

			continue
		}

		peer := s.peerForShard(shardID)
		if peer == nil {
			return nil, nil, status.Errorf(codes.Internal, "no peer configured for shard %d", shardID) //nolint:wrapcheck // gRPC status error
		}

		peerOpsMap[peer] = append(peerOpsMap[peer], ops...)
	}

	return localOps, peerOpsMap, nil
}

// orderedPrepare represents a single participant's prepare function with its
// shard range for deterministic ordering. The run function receives the
// errgroup-derived context so that remote gRPC calls are cancelled when any
// peer short-circuits the prepare phase (rejection or error).
type orderedPrepare struct {
	shardStart int
	shardEnd   int
	peerAddr   string
	run        func(ctx context.Context) prepareResult
}

func (s *authorizerService) executePreparePhase(
	ctx context.Context,
	req *authorizerv1.AuthorizeRequest,
	localOps []*authorizerv1.BalanceOperation,
	peerOpsMap map[*peerClient][]*authorizerv1.BalanceOperation,
) ([]prepareResult, error) {
	buildSubRequest := func(ops []*authorizerv1.BalanceOperation) *authorizerv1.AuthorizeRequest {
		return &authorizerv1.AuthorizeRequest{
			TransactionId:     req.GetTransactionId(),
			OrganizationId:    req.GetOrganizationId(),
			LedgerId:          req.GetLedgerId(),
			Pending:           req.GetPending(),
			TransactionStatus: req.GetTransactionStatus(),
			Operations:        ops,
			Metadata:          req.GetMetadata(),
		}
	}

	participantCount := len(peerOpsMap)
	if len(localOps) > 0 {
		participantCount++
	}

	orderedPrepares := s.buildOrderedPrepares(ctx, localOps, peerOpsMap, buildSubRequest, participantCount)
	sortPreparesByShardOrder(orderedPrepares)

	return s.runPrepareSequence(ctx, req, orderedPrepares)
}

func (s *authorizerService) buildOrderedPrepares(
	_ context.Context,
	localOps []*authorizerv1.BalanceOperation,
	peerOpsMap map[*peerClient][]*authorizerv1.BalanceOperation,
	buildSubRequest func([]*authorizerv1.BalanceOperation) *authorizerv1.AuthorizeRequest,
	participantCount int,
) []orderedPrepare {
	prepares := make([]orderedPrepare, 0, participantCount)

	if len(localOps) > 0 {
		prepares = append(prepares, orderedPrepare{
			shardStart: s.ownedShardStart,
			shardEnd:   s.ownedShardEnd,
			peerAddr:   s.instanceAddr,
			// Local participant does not issue network I/O; the engine call is
			// synchronous and cannot be cancelled mid-flight, so the ctx param
			// is intentionally unused here.
			run: func(_ context.Context) prepareResult {
				return s.prepareLocalParticipant(buildSubRequest(localOps))
			},
		})
	}

	for peer, ops := range peerOpsMap {
		currentPeer := peer
		currentOps := ops

		prepares = append(prepares, orderedPrepare{
			shardStart: currentPeer.shardStart,
			shardEnd:   currentPeer.shardEnd,
			peerAddr:   currentPeer.addr,
			run: func(rpcCtx context.Context) prepareResult {
				return s.prepareRemoteParticipant(rpcCtx, currentPeer, buildSubRequest(currentOps))
			},
		})
	}

	return prepares
}

func (s *authorizerService) prepareLocalParticipant(subReq *authorizerv1.AuthorizeRequest) prepareResult {
	ptx, resp, err := s.engine.PrepareAuthorize(subReq)

	r := prepareResult{
		isLocal: true,
		err:     err,
	}

	if ptx != nil {
		r.txID = ptx.ID
	}

	if resp != nil {
		r.resp = resp
		r.balances = resp.GetBalances()
	}

	return r
}

func (s *authorizerService) prepareRemoteParticipant(
	ctx context.Context,
	peer *peerClient,
	subReq *authorizerv1.AuthorizeRequest,
) prepareResult {
	authCtx, authErr := withPeerAuth(ctx, s.peerAuthToken, peerRPCMethodPrepareAuthorize, subReq)
	if authErr != nil {
		return prepareResult{peer: peer, err: authErr}
	}

	client := peer.pickClient()
	if client == nil {
		return prepareResult{peer: peer, err: fmt.Errorf("no available gRPC client for peer %s: %w", peer.addr, errNoGRPCClientForPeer)}
	}

	pResp, err := client.PrepareAuthorize(authCtx, subReq)

	r := prepareResult{
		peer: peer,
		err:  err,
	}

	if pResp != nil {
		r.txID = pResp.GetPreparedTxId()
		r.balances = pResp.GetBalances()
		r.resp = &authorizerv1.AuthorizeResponse{
			Authorized:       pResp.GetAuthorized(),
			RejectionCode:    pResp.GetRejectionCode(),
			RejectionMessage: pResp.GetRejectionMessage(),
		}
	}

	return r
}

func sortPreparesByShardOrder(prepares []orderedPrepare) {
	sort.SliceStable(prepares, func(i, j int) bool {
		if prepares[i].shardStart == prepares[j].shardStart {
			if prepares[i].shardEnd == prepares[j].shardEnd {
				return prepares[i].peerAddr < prepares[j].peerAddr
			}

			return prepares[i].shardEnd < prepares[j].shardEnd
		}

		return prepares[i].shardStart < prepares[j].shardStart
	})
}

// errPrepareShortCircuit is a sentinel used internally by runPrepareSequence
// to trigger early cancellation of the errgroup when any participant rejects
// or errors. It is never returned to the caller — results are inspected by
// evaluateDecisionAndAbort to build the final response.
var errPrepareShortCircuit = errors.New("prepare short-circuit")

// runPrepareSequence fans out PrepareAuthorize calls to all participants in
// parallel. Latency is dominated by the slowest participant rather than the
// sum of all RTTs, which is critical for high-TPS cross-shard workloads.
//
// Ordering semantics:
//   - orderedPrepares is pre-sorted by shard range so that lock acquisition
//     downstream remains deterministic. RPC dispatch itself is unordered
//     (parallel), but result indices preserve the input order. Downstream
//     consumers (evaluateDecisionAndAbort, abortAllPrepared, commit phase)
//     iterate results in that stable order.
//   - Per-participant lock ordering is enforced inside each engine's
//     PrepareAuthorize and is not affected by this change.
//
// Short-circuit semantics:
//   - On the first participant that returns an error or a non-authorized
//     response, the derived context is cancelled so in-flight peer RPCs
//     unblock promptly.
//   - Participants that complete before cancellation have their results
//     recorded. Participants cancelled mid-flight return a non-nil error
//     (context.Canceled or a wrapped RPC error), which evaluateDecisionAndAbort
//     treats as a prepare failure — abort is then driven against every
//     participant that produced a prepared_tx_id.
//
// Concurrency safety:
//   - Each goroutine writes to its own pre-allocated slot (results[i]).
//     No shared mutable state, no locks required.
func (s *authorizerService) runPrepareSequence(
	ctx context.Context,
	req *authorizerv1.AuthorizeRequest,
	orderedPrepares []orderedPrepare,
) ([]prepareResult, error) {
	if len(orderedPrepares) == 0 {
		return nil, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, s.handlePrepareCtxCancelled(ctx, req, nil, err)
	}

	slots := make([]prepareResult, len(orderedPrepares))
	executed := make([]bool, len(orderedPrepares))

	group, groupCtx := errgroup.WithContext(ctx)

	for i := range orderedPrepares {
		idx := i
		ordered := orderedPrepares[i]

		group.Go(func() error {
			// If the group was already cancelled (another participant failed),
			// skip this participant entirely. It will be filtered out of results.
			if err := groupCtx.Err(); err != nil {
				return nil //nolint:nilerr // cancellation propagated by errgroup, not this goroutine
			}

			r := ordered.run(groupCtx)
			slots[idx] = r
			executed[idx] = true

			// Trigger short-circuit: either a transport/engine error, or a
			// business-level rejection. Returning a sentinel cancels groupCtx,
			// which unblocks any in-flight peer gRPC calls.
			if r.err != nil || (r.resp != nil && !r.resp.GetAuthorized()) {
				return errPrepareShortCircuit
			}

			return nil
		})
	}

	// errgroup.Wait returns the first non-nil error. We deliberately ignore
	// errPrepareShortCircuit (expected signalling) and surface only genuine
	// ctx cancellation from the parent.
	if err := group.Wait(); err != nil && !errors.Is(err, errPrepareShortCircuit) {
		executedResults := collectExecutedPrepareResults(slots, executed)

		return nil, s.handlePrepareCtxCancelled(ctx, req, executedResults, err)
	}

	// errgroup.Wait returns nil when all goroutines return nil — but a
	// goroutine may return nil after observing groupCtx.Err() (no-op short
	// circuit). Surface parent ctx cancellation explicitly so the coordinator
	// aborts any partial preparations rather than silently proceeding to
	// commit with a partial participant set.
	if err := ctx.Err(); err != nil {
		executedResults := collectExecutedPrepareResults(slots, executed)

		return nil, s.handlePrepareCtxCancelled(ctx, req, executedResults, err)
	}

	return collectExecutedPrepareResults(slots, executed), nil
}

// collectExecutedPrepareResults returns only the slots that actually ran,
// preserving the pre-sorted (shard-deterministic) index order.
func collectExecutedPrepareResults(slots []prepareResult, executed []bool) []prepareResult {
	out := make([]prepareResult, 0, len(slots))

	for i := range slots {
		if executed[i] {
			out = append(out, slots[i])
		}
	}

	return out
}

// handlePrepareCtxCancelled drives abort for any participant that managed to
// prepare before the parent context was cancelled, and returns the gRPC status
// to propagate to the caller. The parent ctx is accepted (and unused for the
// abort RPC itself) to satisfy contextcheck: abort must use a fresh
// background context because the parent is already cancelled.
func (s *authorizerService) handlePrepareCtxCancelled(
	_ context.Context,
	req *authorizerv1.AuthorizeRequest,
	partial []prepareResult,
	ctxErr error,
) error {
	//nolint:contextcheck // abort MUST use a fresh context because the inherited ctx is already cancelled
	abortErr := s.abortAllPrepared(context.Background(), partial)
	if abortErr != nil {
		s.logger.Errorf(
			"cross-shard prepare cancellation rollback failed: tx_id=%s err=%v ctx_err=%v",
			req.GetTransactionId(), abortErr, ctxErr,
		)

		return status.Error(codes.Internal, "cross-shard rollback failed") //nolint:wrapcheck // gRPC status error
	}

	return status.Error(codes.DeadlineExceeded, "cross-shard prepare deadline exceeded") //nolint:wrapcheck // gRPC status error
}

func (s *authorizerService) evaluateDecisionAndAbort(
	_ context.Context,
	req *authorizerv1.AuthorizeRequest,
	results []prepareResult,
) (*authorizerv1.AuthorizeResponse, error) {
	allAuthorized := true

	var rejectionResp *authorizerv1.AuthorizeResponse

	var firstError error

	for i := range results {
		r := &results[i]
		if r.err != nil {
			allAuthorized = false
			firstError = r.err

			break
		}

		if r.resp != nil && !r.resp.GetAuthorized() {
			allAuthorized = false
			rejectionResp = r.resp

			break
		}
	}

	if allAuthorized {
		return nil, nil
	}

	//nolint:contextcheck // abort uses fresh context to ensure cleanup completes
	abortErr := s.abortAllPrepared(context.Background(), results)

	if rejectionResp != nil {
		if abortErr != nil {
			s.logger.Errorf(
				"cross-shard prepare rejected but rollback failed: tx_id=%s err=%v",
				req.GetTransactionId(),
				abortErr,
			)

			return nil, status.Error(codes.Internal, "cross-shard rollback failed") //nolint:wrapcheck // gRPC status error
		}

		return rejectionResp, nil
	}

	s.logger.Errorf(
		"cross-shard prepare failed: tx_id=%s err=%v",
		req.GetTransactionId(), firstError,
	)

	return nil, status.Error(codes.Internal, "cross-shard prepare failed") //nolint:wrapcheck // gRPC status error
}

// localCommitResult captures the mutable state produced by commitLocalParticipants.
type localCommitResult struct {
	snapshots    []*authorizerv1.BalanceSnapshot
	failed       bool
	committedAny bool
}

// commitLocalParticipants drives the commit phase for all local (engine)
// participants. It is shared between the async and sync commit paths to avoid
// duplicating the commit/error-handling/idempotency logic.
func (s *authorizerService) commitLocalParticipants(
	results []prepareResult,
	intent *commitIntent,
	txID string,
	committedAny bool,
	publishCommittedStatus func(),
) localCommitResult {
	res := localCommitResult{committedAny: committedAny}

	for i := range results {
		r := &results[i]
		if r.txID == "" || !r.isLocal {
			continue
		}

		s.engine.TagCrossShard(r.txID, walParticipantsFromIntent(intent))

		commitResp, err := s.engine.CommitPrepared(r.txID)
		if err != nil {
			if errors.Is(err, engine.ErrPreparedTxNotFound) {
				s.logger.Warnf(
					"cross-shard local commit reported prepared_tx not found; treating as already committed: tx_id=%s prepared_tx_id=%s",
					txID,
					r.txID,
				)

				markParticipantCommitted(intent, r.txID)

				if !res.committedAny {
					res.committedAny = true

					publishCommittedStatus()
				}

				continue
			}

			s.logger.Errorf(
				"CRITICAL: cross-shard local commit failed after prepare: tx_id=%s prepared_tx_id=%s err=%v",
				txID, r.txID, err,
			)

			res.failed = true

			continue
		}

		markParticipantCommitted(intent, r.txID)

		if !res.committedAny {
			res.committedAny = true

			publishCommittedStatus()
		}

		res.snapshots = append(res.snapshots, commitResp.GetBalances()...)
	}

	return res
}

// asyncCommitPhase runs the async commit path: publish in background with retry,
// commit local immediately, then gate on publish completion.
func (s *authorizerService) asyncCommitPhase(
	_ context.Context,
	results []prepareResult,
	intent *commitIntent,
	req *authorizerv1.AuthorizeRequest,
	committedAny bool,
	publishCommittedStatus func(),
) (*localCommitResult, error) {
	intentCh := s.startAsyncPublish(intent.clone()) //nolint:contextcheck // intentionally uses background context for fire-and-forget async publish

	localResult := s.commitLocalParticipants(results, intent, req.GetTransactionId(), committedAny, publishCommittedStatus)

	if publishErr := <-intentCh; publishErr != nil {
		return s.handleAsyncPublishFailure(results, req, publishErr, &localResult) //nolint:contextcheck // abort in failure path uses background context intentionally
	}

	return &localResult, nil
}

func (s *authorizerService) startAsyncPublish(intent *commitIntent) <-chan error {
	intentCh := make(chan error, 1)

	go func() {
		publishCtx, cancel := context.WithTimeout(context.Background(), s.resolveCommitDeadline())
		defer cancel()

		retryDelays := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond}

		lastErr := s.publishCommitIntent(publishCtx, intent)
		if lastErr == nil {
			intentCh <- nil
			return
		}

		for _, delay := range retryDelays {
			select {
			case <-publishCtx.Done():
				intentCh <- lastErr
				return
			case <-time.After(delay):
			}

			lastErr = s.publishCommitIntent(publishCtx, intent)
			if lastErr == nil {
				intentCh <- nil
				return
			}
		}

		intentCh <- lastErr
	}()

	return intentCh
}

func (s *authorizerService) handleAsyncPublishFailure(
	results []prepareResult,
	req *authorizerv1.AuthorizeRequest,
	publishErr error,
	localResult *localCommitResult,
) (*localCommitResult, error) {
	if !localResult.committedAny {
		abortErr := s.abortAllPrepared(context.Background(), results)
		s.logger.Errorf(
			"cross-shard commit intent write failed (aborting): tx_id=%s err=%v rollback_err=%v",
			req.GetTransactionId(), publishErr, abortErr,
		)

		if abortErr != nil {
			return nil, status.Error(codes.Internal, "failed to write commit intent and rollback prepared participants") //nolint:wrapcheck // gRPC status error
		}

		return nil, status.Error(codes.Internal, "failed to write commit intent") //nolint:wrapcheck // gRPC status error
	}

	s.logger.Errorf(
		"CRITICAL: cross-shard commit intent write failed after local commit: tx_id=%s err=%v",
		req.GetTransactionId(), publishErr,
	)

	localResult.failed = true

	return localResult, nil
}

// syncCommitPhase runs the synchronous commit path: publish before any commits.
func (s *authorizerService) syncCommitPhase(
	ctx context.Context,
	results []prepareResult,
	intent *commitIntent,
	req *authorizerv1.AuthorizeRequest,
	committedAny bool,
	publishCommittedStatus func(),
) (*localCommitResult, error) {
	if err := s.publishCommitIntent(ctx, intent); err != nil {
		abortErr := s.abortAllPrepared(context.Background(), results) //nolint:contextcheck // abort uses fresh context to ensure cleanup completes
		s.logger.Errorf(
			"cross-shard commit intent write failed (aborting): tx_id=%s err=%v rollback_err=%v",
			req.GetTransactionId(), err, abortErr,
		)

		if abortErr != nil {
			return nil, status.Error(codes.Internal, "failed to write commit intent and rollback prepared participants") //nolint:wrapcheck // gRPC status error
		}

		return nil, status.Error(codes.Internal, "failed to write commit intent") //nolint:wrapcheck // gRPC status error
	}

	localResult := s.commitLocalParticipants(results, intent, req.GetTransactionId(), committedAny, publishCommittedStatus)

	if localResult.failed {
		s.logger.Warnf(
			"cross-shard commit requires recovery after local commit failure: tx_id=%s",
			req.GetTransactionId(),
		)
	}

	return &localResult, nil
}

func (s *authorizerService) runLocalCommitPhase(
	ctx context.Context,
	results []prepareResult,
	intent *commitIntent,
	req *authorizerv1.AuthorizeRequest,
	committedAny bool,
	publishCommittedStatus func(),
) (*localCommitResult, error) {
	if s.asyncCommitIntent {
		return s.asyncCommitPhase(ctx, results, intent, req, committedAny, publishCommittedStatus)
	}

	return s.syncCommitPhase(ctx, results, intent, req, committedAny, publishCommittedStatus)
}

// remoteCommitOutcome captures the result of a single remote peer commit so
// that parallel goroutines can report back through a single channel without
// sharing mutable slices across goroutines.
type remoteCommitOutcome struct {
	idx       int
	snapshots []*authorizerv1.BalanceSnapshot
	failed    bool
}

// commitAllRemotePeers fans out CommitPrepared to every remote peer in
// parallel. Unlike prepare, commit MUST attempt every peer even if one fails —
// partial-commit recovery relies on a clear signal of which peers committed
// versus which need recovery. Therefore this function does not short-circuit
// and does not propagate cancellation between peers on failure. Per-peer
// deadlines are set inside commitRemotePeerGuarded via context.WithTimeout.
//
// Concurrency safety:
//   - intent mutation (markParticipantCommitted) and the committedAny flag
//     are protected by intentMu so the first-committed publish fires exactly
//     once even under parallel completion.
//   - Per-peer snapshot slices are collected via per-goroutine locals and
//     reassembled in deterministic (input) order after Wait.
func (s *authorizerService) commitAllRemotePeers(
	ctx context.Context,
	results []prepareResult,
	txID string,
	intent *commitIntent,
	committedAny *bool,
	publishCommittedStatus func(),
) ([]*authorizerv1.BalanceSnapshot, bool) {
	// Gate intent mutation + committedAny toggling so the "first committed"
	// publish fires exactly once even when peers complete concurrently.
	var intentMu sync.Mutex

	guardedPublish := func() {
		intentMu.Lock()
		defer intentMu.Unlock()

		if !*committedAny {
			*committedAny = true

			publishCommittedStatus()
		}
	}

	guardedMark := func(preparedTxID string) {
		intentMu.Lock()
		defer intentMu.Unlock()

		markParticipantCommitted(intent, preparedTxID)
	}

	// Per-slot outcome storage preserves deterministic ordering of snapshots
	// in the final response (matches original sequential behaviour).
	outcomes := make([]remoteCommitOutcome, len(results))

	var wg sync.WaitGroup

	for i := range results {
		r := &results[i]
		if r.txID == "" || r.isLocal || r.peer == nil {
			continue
		}

		idx := i
		participant := r

		wg.Add(1)

		go func() {
			defer wg.Done()

			snapshots, failed := s.commitRemotePeerGuarded(
				ctx,
				participant,
				txID,
				intent,
				guardedPublish,
				guardedMark,
			)

			outcomes[idx] = remoteCommitOutcome{idx: idx, snapshots: snapshots, failed: failed}
		}()
	}

	wg.Wait()

	var (
		allSnapshots []*authorizerv1.BalanceSnapshot
		anyFailed    bool
	)

	for _, o := range outcomes {
		if o.failed {
			anyFailed = true

			continue
		}

		allSnapshots = append(allSnapshots, o.snapshots...)
	}

	return allSnapshots, anyFailed
}

// commitRemotePeerGuarded is the concurrency-safe wrapper around the per-peer
// commit path. It delegates the RPC to commitRemotePeerRPC and routes
// intent mutation through the caller-provided guarded functions so that the
// "first committed" publish fires exactly once across all parallel peers.
func (s *authorizerService) commitRemotePeerGuarded(
	ctx context.Context,
	r *prepareResult,
	txID string,
	intent *commitIntent,
	guardedPublish func(),
	guardedMark func(string),
) ([]*authorizerv1.BalanceSnapshot, bool) {
	commitCtx, cancel := context.WithTimeout(ctx, s.resolveCommitDeadline())
	defer cancel()

	commitReq := &authorizerv1.CommitPreparedRequest{PreparedTxId: r.txID}

	authCtx, authErr := withPeerAuth(commitCtx, s.peerAuthToken, peerRPCMethodCommitPrepared, commitReq)
	if authErr != nil {
		s.logger.Errorf(
			"CRITICAL: cross-shard peer commit auth failed (PARTIAL COMMIT): tx_id=%s peer=%s prepared_tx_id=%s err=%v",
			txID, r.peer.addr, r.txID, authErr,
		)

		return nil, true
	}

	commitClient := r.peer.pickClient()
	if commitClient == nil {
		s.logger.Errorf(
			"CRITICAL: cross-shard peer commit has no available gRPC client: tx_id=%s peer=%s prepared_tx_id=%s",
			txID, r.peer.addr, r.txID,
		)

		return nil, true
	}

	commitResp, err := commitClient.CommitPrepared(authCtx, commitReq)
	if err != nil {
		s.handleRemotePeerCommitError(ctx, err, r, txID, intent)

		return nil, true
	}

	guardedMark(r.txID)
	guardedPublish()

	return commitResp.GetBalances(), false
}

func (s *authorizerService) handleIncompleteCommit(
	req *authorizerv1.AuthorizeRequest,
	intent *commitIntent,
) error {
	committedCount := 0

	for _, participant := range intent.Participants {
		if participant.Committed {
			committedCount++
		}
	}

	s.logger.Warnf(
		"cross-shard commit incomplete; recovery required: tx_id=%s committed_participants=%d total_participants=%d",
		req.GetTransactionId(),
		committedCount,
		len(intent.Participants),
	)

	return status.Error( //nolint:wrapcheck // gRPC status error
		codes.Internal,
		"transaction processing incomplete; recovery in progress",
	)
}

func (s *authorizerService) finalizeCommit(
	ctx context.Context,
	req *authorizerv1.AuthorizeRequest,
	intent *commitIntent,
	start time.Time,
	shardOps map[int][]*authorizerv1.BalanceOperation,
) {
	intent.Status = commitIntentStatusCompleted

	if err := s.publishCommitIntent(context.Background(), intent); err != nil { //nolint:contextcheck // best-effort publish must survive context cancellation
		s.logger.Warnf(
			"cross-shard completion record write failed (non-fatal): tx_id=%s err=%v",
			req.GetTransactionId(), err,
		)
	}

	if s.walReconciler != nil {
		s.walReconciler.markCompleted(intent.TransactionID)
	}

	latency := time.Since(start)

	if s.metrics.Enabled() {
		s.metrics.RecordAuthorize(
			ctx,
			"authorize_cross_shard",
			"authorized",
			"",
			req.GetPending(),
			req.GetTransactionStatus(),
			len(req.GetOperations()),
			len(shardOps),
			latency,
			true,
		)
	}
}

// handleRemotePeerCommitError logs and escalates errors from a remote peer
// commit attempt. All paths through this function result in a failed commit.
func (s *authorizerService) handleRemotePeerCommitError(
	ctx context.Context,
	err error,
	r *prepareResult,
	txID string,
	intent *commitIntent,
) {
	if status.Code(err) == codes.NotFound {
		s.logger.Warnf(
			"CRITICAL: cross-shard peer commit reported prepared_tx not found (possible data loss): tx_id=%s peer=%s prepared_tx_id=%s",
			txID, r.peer.addr, r.txID,
		)

		intent.Status = commitIntentStatusManualIntervention

		if publishErr := s.publishCommitIntent(ctx, intent); publishErr != nil {
			s.logger.Errorf(
				"cross-shard manual intervention status publish failed: tx_id=%s err=%v",
				txID, publishErr,
			)
		}

		return
	}

	s.logger.Errorf(
		"CRITICAL: cross-shard peer commit failed (PARTIAL COMMIT): tx_id=%s peer=%s prepared_tx_id=%s err=%v",
		txID, r.peer.addr, r.txID, err,
	)
}

// abortAllPrepared sends AbortPrepared to all participants that returned a
// prepared transaction ID. Used when any participant rejects or errors during
// the prepare phase.
func (s *authorizerService) abortAllPrepared(ctx context.Context, results []prepareResult) error {
	var abortErr error

	for i := range results {
		r := &results[i]
		if r.txID == "" {
			continue
		}

		if r.isLocal {
			if err := s.engine.AbortPrepared(r.txID); err != nil {
				s.logger.Warnf("cross-shard abort local failed: prepared_tx_id=%s err=%v", r.txID, err)
				abortErr = errors.Join(abortErr, err)
			}

			continue
		}

		if r.peer == nil {
			continue
		}

		if err := s.abortRemotePeer(ctx, r); err != nil {
			abortErr = errors.Join(abortErr, err)
		}
	}

	return abortErr
}

func (s *authorizerService) abortRemotePeer(ctx context.Context, r *prepareResult) error {
	abortCtx, cancel := context.WithTimeout(ctx, s.resolveAbortDeadline())
	defer cancel()

	abortReq := &authorizerv1.AbortPreparedRequest{PreparedTxId: r.txID}

	authCtx, authErr := withPeerAuth(abortCtx, s.peerAuthToken, peerRPCMethodAbortPrepared, abortReq)
	if authErr != nil {
		s.logger.Warnf("cross-shard abort peer auth failed: peer=%s prepared_tx_id=%s err=%v", r.peer.addr, r.txID, authErr)

		return authErr
	}

	abortClient := r.peer.pickClient()
	if abortClient == nil {
		s.logger.Warnf("cross-shard abort peer has no available gRPC client: peer=%s prepared_tx_id=%s", r.peer.addr, r.txID)

		return fmt.Errorf("%w: %s", errNoGRPCClientForPeer, r.peer.addr)
	}

	_, err := abortClient.AbortPrepared(authCtx, abortReq)
	if err != nil {
		s.logger.Warnf("cross-shard abort peer failed: peer=%s prepared_tx_id=%s err=%v", r.peer.addr, r.txID, err)

		return fmt.Errorf("abort peer %s prepared_tx_id=%s: %w", r.peer.addr, r.txID, err)
	}

	return nil
}
