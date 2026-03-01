// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *authorizerService) resolveCommitDeadline() time.Duration {
	if s == nil || s.commitRPCDeadline <= 0 {
		return 10 * time.Second
	}

	return s.commitRPCDeadline
}

func (s *authorizerService) resolveAbortDeadline() time.Duration {
	if s == nil || s.abortRPCDeadline <= 0 {
		return 5 * time.Second
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
//     remote peers) in parallel. Each participant validates operations, acquires
//     shard locks, and returns a prepared transaction ID.
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

	// Partition operations by owner: local engine vs. each remote peer.
	localOps := make([]*authorizerv1.BalanceOperation, 0)
	peerOpsMap := make(map[*peerClient][]*authorizerv1.BalanceOperation)

	for shardID, ops := range shardOps {
		if s.isLocalShard(shardID) {
			localOps = append(localOps, ops...)
		} else {
			peer := s.peerForShard(shardID)
			if peer == nil {
				return nil, status.Errorf(codes.Internal, "no peer configured for shard %d", shardID)
			}

			peerOpsMap[peer] = append(peerOpsMap[peer], ops...)
		}
	}

	// Build a sub-request that preserves all transaction metadata but swaps operations.
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

	// ─── PHASE 1: PREPARE (sequential, globally ordered) ─────────────────
	//
	// Critical: all participants (local + remote peers) must prepare in the same
	// global shard order to prevent distributed deadlocks. We sort participants by
	// owned shard range (start then end), then execute prepares sequentially.
	//
	// This extends the engine's local deadlock prevention (sort.Ints(orderedShards))
	// to the distributed case.

	participantCount := len(peerOpsMap)
	if len(localOps) > 0 {
		participantCount++
	}

	results := make([]prepareResult, 0, participantCount)

	// prepareFuncs is an ordered list of prepare operations.
	type prepareFn func() prepareResult
	type orderedPrepare struct {
		shardStart int
		shardEnd   int
		run        prepareFn
	}

	orderedPrepares := make([]orderedPrepare, 0, participantCount)

	prepareLocal := func() prepareResult {
		ptx, resp, err := s.engine.PrepareAuthorize(buildSubRequest(localOps))

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

	if len(localOps) > 0 {
		orderedPrepares = append(orderedPrepares, orderedPrepare{
			shardStart: s.ownedShardStart,
			shardEnd:   s.ownedShardEnd,
			run:        prepareLocal,
		})
	}

	// Build remote prepare functions.
	remotePeers := make([]*peerClient, 0, len(peerOpsMap))
	for peer := range peerOpsMap {
		remotePeers = append(remotePeers, peer)
	}

	sort.Slice(remotePeers, func(i, j int) bool {
		if remotePeers[i].shardStart == remotePeers[j].shardStart {
			return remotePeers[i].shardEnd < remotePeers[j].shardEnd
		}

		return remotePeers[i].shardStart < remotePeers[j].shardStart
	})

	for _, peer := range remotePeers {
		ops := peerOpsMap[peer]
		currentPeer := peer
		currentOps := ops

		orderedPrepares = append(orderedPrepares, orderedPrepare{
			shardStart: currentPeer.shardStart,
			shardEnd:   currentPeer.shardEnd,
			run: func() prepareResult {
				subReq := buildSubRequest(currentOps)
				authCtx, authErr := withPeerAuth(ctx, s.peerAuthToken, peerRPCMethodPrepareAuthorize, subReq)
				if authErr != nil {
					return prepareResult{peer: currentPeer, err: authErr}
				}

				pResp, err := currentPeer.client.PrepareAuthorize(
					authCtx,
					subReq,
				)

				r := prepareResult{
					peer: currentPeer,
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
			},
		})
	}

	sort.SliceStable(orderedPrepares, func(i, j int) bool {
		if orderedPrepares[i].shardStart == orderedPrepares[j].shardStart {
			return orderedPrepares[i].shardEnd < orderedPrepares[j].shardEnd
		}

		return orderedPrepares[i].shardStart < orderedPrepares[j].shardStart
	})

	// Execute prepares sequentially in shard order. Abort on first failure.
	for _, ordered := range orderedPrepares {
		if err := ctx.Err(); err != nil {
			if abortErr := s.abortAllPrepared(results); abortErr != nil {
				s.logger.Errorf("cross-shard prepare cancellation rollback failed: tx_id=%s err=%v", req.GetTransactionId(), abortErr)
				return nil, status.Error(codes.Internal, "cross-shard rollback failed")
			}

			return nil, status.Error(codes.DeadlineExceeded, "cross-shard prepare deadline exceeded")
		}

		r := ordered.run()
		results = append(results, r)

		if r.err != nil || (r.resp != nil && !r.resp.GetAuthorized()) {
			// Early exit: abort all previously prepared participants.
			break
		}
	}

	// ─── DECISION ───────────────────────────────────────────────────────

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

	// ─── ABORT PATH (any failure) ───────────────────────────────────────

	if !allAuthorized {
		abortErr := s.abortAllPrepared(results)

		if rejectionResp != nil {
			if abortErr != nil {
				s.logger.Errorf(
					"cross-shard prepare rejected but rollback failed: tx_id=%s err=%v",
					req.GetTransactionId(),
					abortErr,
				)

				return nil, status.Error(codes.Internal, "cross-shard rollback failed")
			}

			// Business rejection (insufficient funds, etc.) — return as-is to caller.
			return rejectionResp, nil
		}

		s.logger.Errorf(
			"cross-shard prepare failed: tx_id=%s err=%v",
			req.GetTransactionId(), firstError,
		)

		return nil, status.Error(codes.Internal, "cross-shard prepare failed")
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

	if err := s.publishCommitIntent(ctx, &intent); err != nil {
		// Cannot durably record the commit decision → abort everything.
		// This is safe: no participant has committed yet.
		abortErr := s.abortAllPrepared(results)
		s.logger.Errorf(
			"cross-shard commit intent write failed (aborting): tx_id=%s err=%v rollback_err=%v",
			req.GetTransactionId(), err, abortErr,
		)

		if abortErr != nil {
			return nil, status.Error(codes.Internal, "failed to write commit intent and rollback prepared participants")
		}

		return nil, status.Error(codes.Internal, "failed to write commit intent")
	}

	// ─── PHASE 2: COMMIT (sequential for correctness) ───────────────────
	//
	// Commit order: local first, then peers. If the local commit fails we
	// still have the durable commit intent in Redpanda — recovery can retry.
	// The intent makes partial-commit states recoverable rather than
	// requiring manual intervention.

	var allSnapshots []*authorizerv1.BalanceSnapshot
	commitFailed := false
	committedAny := false
	publishCommittedStatus := func() {
		intent.Status = commitIntentStatusCommitted
		if err := s.publishCommitIntent(context.Background(), &intent); err != nil {
			s.logger.Warnf(
				"cross-shard COMMITTED intent write failed (non-fatal): tx_id=%s err=%v",
				req.GetTransactionId(),
				err,
			)
		}
	}

	for i := range results {
		r := &results[i]
		if r.txID == "" {
			continue
		}

		if r.isLocal {
			commitResp, err := s.engine.CommitPrepared(r.txID)
			if err != nil {
				if errors.Is(err, engine.ErrPreparedTxNotFound) {
					s.logger.Warnf(
						"cross-shard local commit reported prepared_tx not found; treating as already committed: tx_id=%s prepared_tx_id=%s",
						req.GetTransactionId(),
						r.txID,
					)

					markParticipantCommitted(&intent, r.txID)
					if !committedAny {
						committedAny = true
						publishCommittedStatus()
					}

					continue
				}

				s.logger.Errorf(
					"CRITICAL: cross-shard local commit failed after prepare: tx_id=%s prepared_tx_id=%s err=%v",
					req.GetTransactionId(), r.txID, err,
				)

				commitFailed = true

				continue
			}

			markParticipantCommitted(&intent, r.txID)
			if !committedAny {
				committedAny = true
				publishCommittedStatus()
			}

			allSnapshots = append(allSnapshots, commitResp.GetBalances()...)
		}
	}

	if commitFailed {
		s.logger.Warnf(
			"cross-shard commit requires recovery after local commit failure: tx_id=%s",
			req.GetTransactionId(),
		)
	}

	for i := range results {
		r := &results[i]
		if r.txID == "" || r.isLocal {
			continue
		}

		if r.peer != nil {
			commitCtx, cancel := context.WithTimeout(context.Background(), s.resolveCommitDeadline())
			commitReq := &authorizerv1.CommitPreparedRequest{PreparedTxId: r.txID}
			authCtx, authErr := withPeerAuth(commitCtx, s.peerAuthToken, peerRPCMethodCommitPrepared, commitReq)
			if authErr != nil {
				cancel()
				s.logger.Errorf(
					"CRITICAL: cross-shard peer commit auth failed (PARTIAL COMMIT): tx_id=%s peer=%s prepared_tx_id=%s err=%v",
					req.GetTransactionId(), r.peer.addr, r.txID, authErr,
				)
				commitFailed = true
				continue
			}

			commitResp, err := r.peer.client.CommitPrepared(authCtx, commitReq)
			cancel()
			if err != nil {
				if status.Code(err) == codes.NotFound {
					s.logger.Warnf(
						"cross-shard peer commit reported prepared_tx not found; treating as already committed: tx_id=%s peer=%s prepared_tx_id=%s",
						req.GetTransactionId(),
						r.peer.addr,
						r.txID,
					)

					markParticipantCommitted(&intent, r.txID)
					if !committedAny {
						committedAny = true
						publishCommittedStatus()
					}

					continue
				}

				// CRITICAL: local already committed but peer failed.
				// This is a partial commit — requires manual recovery.
				// Phase E (Redpanda commit intent log) addresses this.
				s.logger.Errorf(
					"CRITICAL: cross-shard peer commit failed (PARTIAL COMMIT): tx_id=%s peer=%s prepared_tx_id=%s err=%v",
					req.GetTransactionId(), r.peer.addr, r.txID, err,
				)

				commitFailed = true
				continue
			}

			markParticipantCommitted(&intent, r.txID)
			if !committedAny {
				committedAny = true
				publishCommittedStatus()
			}

			allSnapshots = append(allSnapshots, commitResp.GetBalances()...)
		}
	}

	if commitFailed {
		s.logger.Warnf(
			"cross-shard commit incomplete; recovery required: tx_id=%s",
			req.GetTransactionId(),
		)

		return nil, status.Error(codes.Aborted, "cross-shard commit incomplete; recovery in progress")
	}

	// ─── PHASE E: COMPLETION RECORD ─────────────────────────────────────
	//
	// Best-effort write of the COMPLETED status. This closes the commit
	// intent lifecycle. If this write fails, recovery will see a PREPARED
	// intent and re-check participants — they'll report already-committed
	// (idempotent), so no harm done.
	intent.Status = commitIntentStatusCompleted
	if err := s.publishCommitIntent(context.Background(), &intent); err != nil {
		s.logger.Warnf(
			"cross-shard completion record write failed (non-fatal): tx_id=%s err=%v",
			req.GetTransactionId(), err,
		)
	}

	// Record metrics for the cross-shard transaction.
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
		)
	}

	return &authorizerv1.AuthorizeResponse{
		Authorized: true,
		Balances:   allSnapshots,
	}, nil
}

// abortAllPrepared sends AbortPrepared to all participants that returned a
// prepared transaction ID. Used when any participant rejects or errors during
// the prepare phase.
func (s *authorizerService) abortAllPrepared(results []prepareResult) error {
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
		} else if r.peer != nil {
			abortCtx, cancel := context.WithTimeout(context.Background(), s.resolveAbortDeadline())
			abortReq := &authorizerv1.AbortPreparedRequest{PreparedTxId: r.txID}
			authCtx, authErr := withPeerAuth(abortCtx, s.peerAuthToken, peerRPCMethodAbortPrepared, abortReq)
			if authErr != nil {
				cancel()
				s.logger.Warnf("cross-shard abort peer auth failed: peer=%s prepared_tx_id=%s err=%v", r.peer.addr, r.txID, authErr)
				abortErr = errors.Join(abortErr, authErr)
				continue
			}

			_, err := r.peer.client.AbortPrepared(authCtx, abortReq)
			cancel()
			if err != nil {
				s.logger.Warnf("cross-shard abort peer failed: peer=%s prepared_tx_id=%s err=%v", r.peer.addr, r.txID, err)
				abortErr = errors.Join(abortErr, err)
			}
		}
	}

	return abortErr
}
