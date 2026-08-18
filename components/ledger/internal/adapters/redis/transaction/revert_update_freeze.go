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
	revertPhaseZeroRequestLeaseKey = "rollout:" + revertRolloutHashTag + ":phase-zero-reverts"
	revertBridgeRequestLeaseKey    = "rollout:" + revertRolloutHashTag + ":bridge-reverts"
	RevertUpdateFreezeActive       = "active"
	RevertUpdateFreezeDrained      = "phase-zero-drained"
	RevertUpdateFreezeFinalized    = "finalized"
)

const activateRevertUpdateFreezeScript = `
local current = redis.call('GET', KEYS[1])
if not current then
  if redis.call('SCARD', KEYS[2]) ~= 0 then
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
local token = ARGV[3]

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
  redis.call('SADD', KEYS[2], token)
  return 1
end

if kind ~= 'revert' or not ready then
  return 0
end
if mode == 'legacy' then
  redis.call('SADD', KEYS[3], token)
  if phase == '' then
    return 4
  end
  return 1
end
if mode == 'bridge' then
  redis.call('SADD', KEYS[4], token)
  return 1
end

-- Final requests remain safe across finalization and need no drain lease.
return 3
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
	result, err := g.acquireRequest(ctx, mode, "approved-update", token)
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
// drained before the next phase, records a durable in-flight token.
func (g *RevertUpdateFreezeGuard) AcquireRevert(ctx context.Context, mode, token string) (bool, bool, string, error) {
	result, err := g.acquireRequest(ctx, mode, "revert", token)
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
	default:
		return false, false, "", nil
	}
}

func (g *RevertUpdateFreezeGuard) acquireRequest(ctx context.Context, mode, kind, token string) (int64, error) {
	if g == nil || g.connection == nil {
		return 0, fmt.Errorf("revert update freeze Redis connection not configured")
	}
	if token == "" {
		return 0, fmt.Errorf("revert rollout request token is required")
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
	}, mode, kind, token).Int64()
	if err != nil {
		return 0, fmt.Errorf("acquire revert rollout request lease: %w", err)
	}

	return result, nil
}

// ReleaseApprovedUpdate removes only the caller's durable update token.
func (g *RevertUpdateFreezeGuard) ReleaseApprovedUpdate(ctx context.Context, token string) error {
	return g.releaseRequest(ctx, revertApprovedUpdateLeaseKey, token)
}

// ReleaseRevert removes only the caller's generation-scoped revert token.
func (g *RevertUpdateFreezeGuard) ReleaseRevert(ctx context.Context, mode, token string) error {
	switch mode {
	case "legacy":
		return g.releaseRequest(ctx, revertPhaseZeroRequestLeaseKey, token)
	case "bridge":
		return g.releaseRequest(ctx, revertBridgeRequestLeaseKey, token)
	case "final":
		return nil
	default:
		return fmt.Errorf("release revert rollout request lease: invalid mode %q", mode)
	}
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
		[]string{RevertUpdateFreezeKey, revertApprovedUpdateLeaseKey},
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
		"mark phase-zero revert requests drained")
}

// Finalize atomically disables the update freeze only after phase zero was
// explicitly marked drained. The terminal marker remains durable so final
// pods can prove the rollout completed after a restart.
func (g *RevertUpdateFreezeGuard) Finalize(ctx context.Context) error {
	return g.advance(ctx, RevertUpdateFreezeDrained, RevertUpdateFreezeFinalized, revertBridgeRequestLeaseKey,
		"finalize revert update freeze")
}

func (g *RevertUpdateFreezeGuard) advance(ctx context.Context, from, to, leaseKey, operation string) error {
	if g == nil || g.connection == nil {
		return fmt.Errorf("revert update freeze Redis connection not configured")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get revert update freeze Redis client: %w", err)
	}

	advanced, err := client.Eval(ctx, advanceRevertUpdateFreezeScript, []string{RevertUpdateFreezeKey, leaseKey}, from, to).Bool()
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if !advanced {
		return fmt.Errorf("%s: expected %q phase", operation, from)
	}

	return nil
}
