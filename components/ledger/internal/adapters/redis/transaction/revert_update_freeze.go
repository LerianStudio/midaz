// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"fmt"

	libRedis "github.com/LerianStudio/lib-commons/v6/commons/redis"
	redislib "github.com/redis/go-redis/v9"
)

const (
	// RevertUpdateFreezeKey is the deployment-wide rollout marker. It is
	// deliberately not tenant-prefixed so every pod observes the same fence.
	// The hash tag keeps the marker and every in-flight lease in one Redis
	// Cluster slot, allowing phase transitions to prove drain atomically.
	revertRolloutHashTag           = "{transaction-revert-rollout:v1}"
	RevertUpdateFreezeKey          = "rollout:" + revertRolloutHashTag + ":state"
	revertApprovedUpdateLeaseKey   = "rollout:" + revertRolloutHashTag + ":approved-updates"
	revertPhaseZeroRequestLeaseKey = "rollout:" + revertRolloutHashTag + ":phase-zero-revert-origins"
	revertBridgeRequestLeaseKey    = "rollout:" + revertRolloutHashTag + ":bridge-revert-origins"
	revertPhaseZeroCompletedKey    = "rollout:" + revertRolloutHashTag + ":phase-zero-completed-origins"
	revertBridgeCompletedKey       = "rollout:" + revertRolloutHashTag + ":bridge-completed-origins"
	RevertUpdateFreezeActive       = "active"
	RevertUpdateFreezeDrained      = "phase-zero-drained"
	RevertUpdateFreezeFinalized    = "finalized"
)

const activateRevertUpdateFreezeScript = `
local current = redis.call('GET', KEYS[1])
if not current then
  if redis.call('SCARD', KEYS[2]) ~= 0 or redis.call('SCARD', KEYS[3]) ~= 0 then
    return 0
  end
  redis.call('SET', KEYS[1], ARGV[1])
  return 1
end
if current == ARGV[1] then
  return 1
end
return 0
`

const advanceRevertUpdateFreezeScript = `
local current = redis.call('GET', KEYS[1])
if current == ARGV[1] then
  if redis.call('SCARD', KEYS[2]) ~= 0 then
    return 0
  end
  redis.call('SET', KEYS[1], ARGV[2])
  redis.call('DEL', KEYS[3])
  return 1
end
if current == ARGV[2] then
  return 1
end
return 0
`

const acquireRevertRolloutRequestScript = `
local phase = redis.call('GET', KEYS[1]) or ''
local mode = ARGV[1]
local kind = ARGV[2]
local origin = ARGV[3]
local attempt = ARGV[4]

local ready = false
if mode == 'legacy' then
  ready = phase == '' or phase == 'active'
elseif mode == 'bridge' then
  ready = phase == 'active' or phase == 'phase-zero-drained'
elseif mode == 'final' then
  ready = phase == 'phase-zero-drained' or phase == 'finalized'
end

if kind == 'approved-update' then
  local frozen = phase == 'active' or phase == 'phase-zero-drained'
  if frozen then
    return 2
  end
  if not ready then
    return 0
  end
  if mode == 'final' and phase == 'finalized' then
    return 3
  end
  redis.call('SADD', KEYS[2], origin)
  return 1
end

if kind ~= 'revert' or not ready then
  return 0
end
if mode == 'legacy' then
  if redis.call('SISMEMBER', KEYS[5], origin) == 1 then
    return 5
  end
  redis.call('HSET', KEYS[7], attempt, 1)
  redis.call('SADD', KEYS[3], origin)
  if phase == '' then
    return 4
  end
  return 1
end
if mode == 'bridge' then
  if redis.call('SISMEMBER', KEYS[6], origin) == 1 then
    return 5
  end
  redis.call('HSET', KEYS[8], attempt, 1)
  redis.call('SADD', KEYS[4], origin)
  return 1
end

-- Final requests remain safe across finalization and need no drain lease.
return 3
`

const releaseRevertRolloutRequestScript = `
if redis.call('SISMEMBER', KEYS[3], ARGV[1]) == 1 then
  return 0
end
redis.call('HDEL', KEYS[1], ARGV[2])
if redis.call('HLEN', KEYS[1]) == 0 then
  redis.call('DEL', KEYS[1])
  redis.call('SREM', KEYS[2], ARGV[1])
end
return 1
`

const completeRevertRolloutRequestScript = `
redis.call('SADD', KEYS[3], ARGV[1])
redis.call('DEL', KEYS[1])
redis.call('SREM', KEYS[2], ARGV[1])
return 1
`

// RevertUpdateFreezeGuard reads the deployment-wide rollout marker without a
// tenant prefix. Phase-zero, bridge, readiness, and every tenant must observe
// one shared barrier while old and bridge revert algorithms coexist.
type RevertUpdateFreezeGuard struct {
	connection *libRedis.Client
}

// NewRevertUpdateFreezeGuard creates a deployment-wide revert rollout guard.
func NewRevertUpdateFreezeGuard(connection *libRedis.Client) *RevertUpdateFreezeGuard {
	return &RevertUpdateFreezeGuard{connection: connection}
}

// Active reports whether approved transaction updates are currently frozen.
func (g *RevertUpdateFreezeGuard) Active(ctx context.Context) (bool, error) {
	value, err := g.Phase(ctx)
	if err != nil {
		return false, err
	}

	return value == RevertUpdateFreezeActive || value == RevertUpdateFreezeDrained, nil
}

// Phase returns the durable rollout phase observed by all ledger pods.
func (g *RevertUpdateFreezeGuard) Phase(ctx context.Context) (string, error) {
	return g.state(ctx)
}

// ApprovedUpdatePolicy returns one atomic marker snapshot for the update gate.
// Reading freeze and readiness separately would allow an absent-to-active
// transition between reads to admit one APPROVED update after activation.
func (g *RevertUpdateFreezeGuard) ApprovedUpdatePolicy(ctx context.Context, mode string) (bool, bool, error) {
	value, err := g.state(ctx)
	if err != nil {
		return false, false, err
	}

	frozen := value == RevertUpdateFreezeActive || value == RevertUpdateFreezeDrained

	return frozen, revertModeReadyForPhase(value, mode), nil
}

// AcquireApprovedUpdate atomically admits an APPROVED update and records its
// durable in-flight token. Activation cannot complete until every admitted
// update has released its token after the PostgreSQL write returns.
func (g *RevertUpdateFreezeGuard) AcquireApprovedUpdate(ctx context.Context, mode, token string) (bool, bool, bool, error) {
	result, err := g.acquireRequest(ctx, mode, "approved-update", token, token)
	if err != nil {
		return false, false, false, err
	}

	switch result {
	case 1:
		return true, false, true, nil
	case 3:
		return true, false, false, nil
	case 2:
		return false, true, false, nil
	default:
		return false, false, false, nil
	}
}

// AcquireRevert atomically admits a revert and, for a generation that must be
// drained before the next phase, records an idempotent attempt under its
// durable origin token.
func (g *RevertUpdateFreezeGuard) AcquireRevert(ctx context.Context, mode, originToken, attemptID string) (bool, bool, string, error) {
	result, err := g.acquireRequest(ctx, mode, "revert", originToken, attemptID)
	if err != nil {
		return false, false, "", err
	}

	switch result {
	case 1:
		phase := ""
		if mode == "legacy" {
			phase = RevertUpdateFreezeActive
		}
		return true, true, phase, nil
	case 3:
		return true, false, RevertUpdateFreezeFinalized, nil
	case 4:
		return true, true, "", nil
	case 5:
		return true, false, "", nil
	default:
		return false, false, "", nil
	}
}

func (g *RevertUpdateFreezeGuard) acquireRequest(ctx context.Context, mode, kind, token, attemptID string) (int64, error) {
	if g == nil || g.connection == nil {
		return 0, fmt.Errorf("revert update freeze Redis connection not configured")
	}
	if token == "" {
		return 0, fmt.Errorf("revert rollout request token is required")
	}
	if attemptID == "" {
		return 0, fmt.Errorf("revert rollout attempt id is required")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return 0, fmt.Errorf("get revert update freeze Redis client: %w", err)
	}

	result, err := client.Eval(ctx, acquireRevertRolloutRequestScript, []string{
		RevertUpdateFreezeKey,
		revertApprovedUpdateLeaseKey,
		revertPhaseZeroRequestLeaseKey,
		revertBridgeRequestLeaseKey,
		revertPhaseZeroCompletedKey,
		revertBridgeCompletedKey,
		revertRolloutOriginAttemptKey("legacy", token),
		revertRolloutOriginAttemptKey("bridge", token),
	}, mode, kind, token, attemptID).Int64()
	if err != nil {
		return 0, fmt.Errorf("acquire revert rollout request lease: %w", err)
	}

	return result, nil
}

// ReleaseApprovedUpdate removes only the caller's durable update token.
func (g *RevertUpdateFreezeGuard) ReleaseApprovedUpdate(ctx context.Context, token string) error {
	return g.releaseRequest(ctx, revertApprovedUpdateLeaseKey, token)
}

// ReleaseRevert removes only the caller's generation-scoped attempt ID.
func (g *RevertUpdateFreezeGuard) ReleaseRevert(ctx context.Context, mode, originToken, attemptID string) error {
	key, err := revertRolloutLeaseKey(mode)
	if err != nil {
		return err
	}

	return g.releaseRevertAttempt(ctx, mode, key, originToken, attemptID)
}

// CompleteRevert removes the entire origin-scoped generation admission only
// after terminal persistence proves that no same-origin HTTP attempt can still
// create an economic mutation. It seals the origin before cleanup, so later
// admissions and releases are idempotent no-ops.
func (g *RevertUpdateFreezeGuard) CompleteRevert(ctx context.Context, mode, token string) error {
	key, err := revertRolloutLeaseKey(mode)
	if err != nil {
		return err
	}
	if key == "" {
		return nil
	}
	if g == nil || g.connection == nil {
		return fmt.Errorf("revert update freeze Redis connection not configured")
	}
	if token == "" {
		return fmt.Errorf("revert rollout request token is required")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get revert update freeze Redis client: %w", err)
	}
	completedKey := revertPhaseZeroCompletedKey
	if mode == "bridge" {
		completedKey = revertBridgeCompletedKey
	}
	if err := client.Eval(ctx, completeRevertRolloutRequestScript, []string{
		revertRolloutOriginAttemptKey(mode, token), key, completedKey,
	}, token).Err(); err != nil {
		return fmt.Errorf("complete revert rollout origin lease: %w", err)
	}

	return nil
}

func revertRolloutOriginAttemptKey(mode, originToken string) string {
	return "rollout:" + revertRolloutHashTag + ":" + mode + "-revert-attempts:" + originToken
}

func revertRolloutLeaseKey(mode string) (string, error) {
	switch mode {
	case "legacy":
		return revertPhaseZeroRequestLeaseKey, nil
	case "bridge":
		return revertBridgeRequestLeaseKey, nil
	case "final":
		return "", nil
	default:
		return "", fmt.Errorf("release revert rollout request lease: invalid mode %q", mode)
	}
}

func (g *RevertUpdateFreezeGuard) releaseRevertAttempt(ctx context.Context, mode, key, originToken, attemptID string) error {
	if key == "" {
		return nil
	}
	if g == nil || g.connection == nil {
		return fmt.Errorf("revert update freeze Redis connection not configured")
	}
	if originToken == "" {
		return fmt.Errorf("revert rollout request token is required")
	}
	if attemptID == "" {
		return fmt.Errorf("revert rollout attempt id is required")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get revert update freeze Redis client: %w", err)
	}
	completedKey := revertPhaseZeroCompletedKey
	if mode == "bridge" {
		completedKey = revertBridgeCompletedKey
	}
	if err := client.Eval(ctx, releaseRevertRolloutRequestScript, []string{
		revertRolloutOriginAttemptKey(mode, originToken), key, completedKey,
	}, originToken, attemptID).Err(); err != nil {
		return fmt.Errorf("release revert rollout request lease: %w", err)
	}

	return nil
}

func (g *RevertUpdateFreezeGuard) releaseRequest(ctx context.Context, key, token string) error {
	if g == nil || g.connection == nil {
		return fmt.Errorf("revert update freeze Redis connection not configured")
	}
	if token == "" {
		return fmt.Errorf("revert rollout request token is required")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get revert update freeze Redis client: %w", err)
	}
	if err := client.SRem(ctx, key, token).Err(); err != nil {
		return fmt.Errorf("release revert rollout request lease: %w", err)
	}

	return nil
}

// ReadyForMode reports whether the shared rollout marker admits the requested
// revert algorithm phase.
func (g *RevertUpdateFreezeGuard) ReadyForMode(ctx context.Context, mode string) (bool, error) {
	value, err := g.state(ctx)
	if err != nil {
		return false, err
	}

	return revertModeReadyForPhase(value, mode), nil
}

func revertModeReadyForPhase(value, mode string) bool {
	switch mode {
	case "legacy":
		return value == "" || value == RevertUpdateFreezeActive
	case "bridge":
		return value == RevertUpdateFreezeActive || value == RevertUpdateFreezeDrained
	case "final":
		return value == RevertUpdateFreezeDrained || value == RevertUpdateFreezeFinalized
	default:
		return false
	}
}

func (g *RevertUpdateFreezeGuard) state(ctx context.Context) (string, error) {
	if g == nil || g.connection == nil {
		return "", fmt.Errorf("revert update freeze Redis connection not configured")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return "", fmt.Errorf("get revert update freeze Redis client: %w", err)
	}

	value, err := client.Get(ctx, RevertUpdateFreezeKey).Result()
	if errors.Is(err, redislib.Nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read revert update freeze marker: %w", err)
	}

	return value, nil
}

// Activate persists the rollout freeze only from the initial absent state. A
// finalized rollout cannot be reopened accidentally.
func (g *RevertUpdateFreezeGuard) Activate(ctx context.Context) error {
	if g == nil || g.connection == nil {
		return fmt.Errorf("revert update freeze Redis connection not configured")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get revert update freeze Redis client: %w", err)
	}
	activated, err := client.Eval(ctx, activateRevertUpdateFreezeScript,
		[]string{RevertUpdateFreezeKey, revertApprovedUpdateLeaseKey, revertPhaseZeroRequestLeaseKey},
		RevertUpdateFreezeActive).Bool()
	if err != nil {
		return fmt.Errorf("activate revert update freeze: %w", err)
	}
	if !activated {
		return fmt.Errorf("activate revert update freeze: rollout already advanced")
	}

	return nil
}

// MarkPhaseZeroDrained records the machine-verifiable zero-legacy-pod and
// in-flight-request drain gate. Legacy pods become unready while bridge and
// final pods keep approved updates frozen.
func (g *RevertUpdateFreezeGuard) MarkPhaseZeroDrained(ctx context.Context) error {
	return g.advance(ctx, RevertUpdateFreezeActive, RevertUpdateFreezeDrained, revertPhaseZeroRequestLeaseKey,
		revertPhaseZeroCompletedKey, "mark phase-zero revert requests drained")
}

// Finalize atomically disables the update freeze only after phase zero was
// explicitly marked drained. The terminal marker remains durable so final
// pods can prove the rollout completed after a restart.
func (g *RevertUpdateFreezeGuard) Finalize(ctx context.Context) error {
	return g.advance(ctx, RevertUpdateFreezeDrained, RevertUpdateFreezeFinalized, revertBridgeRequestLeaseKey,
		revertBridgeCompletedKey, "finalize revert update freeze")
}

func (g *RevertUpdateFreezeGuard) advance(ctx context.Context, from, to, leaseKey, completedKey, operation string) error {
	if g == nil || g.connection == nil {
		return fmt.Errorf("revert update freeze Redis connection not configured")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get revert update freeze Redis client: %w", err)
	}

	advanced, err := client.Eval(ctx, advanceRevertUpdateFreezeScript,
		[]string{RevertUpdateFreezeKey, leaseKey, completedKey}, from, to).Bool()
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if !advanced {
		return fmt.Errorf("%s: expected %q phase", operation, from)
	}

	return nil
}
