// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"

	libRedis "github.com/LerianStudio/lib-commons/v6/commons/redis"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
)

const (
	// RevertUpdateFreezeKey is the deployment-wide rollout marker. It is
	// deliberately not tenant-prefixed so every pod observes the same fence.
	// The hash tag keeps the marker and every in-flight lease in one Redis
	// Cluster slot, allowing phase transitions to prove drain atomically.
	revertRolloutHashTag            = "{transaction-revert-rollout:v1}"
	RevertUpdateFreezeKey           = "rollout:" + revertRolloutHashTag + ":state"
	RevertRolloutGenerationKey      = "rollout:" + revertRolloutHashTag + ":dataset-generation"
	revertApprovedUpdateLeaseKey    = "rollout:" + revertRolloutHashTag + ":approved-updates"
	revertPhaseZeroRequestLeaseKey  = "rollout:" + revertRolloutHashTag + ":phase-zero-revert-origins"
	revertBridgeRequestLeaseKey     = "rollout:" + revertRolloutHashTag + ":bridge-revert-origins"
	revertPhaseZeroCompletedKey     = "rollout:" + revertRolloutHashTag + ":phase-zero-completed-origins"
	revertBridgeCompletedKey        = "rollout:" + revertRolloutHashTag + ":bridge-completed-origins"
	RevertUpdateFreezeInitialize    = "initialize"
	RevertUpdateFreezeUninitialized = "uninitialized"
	RevertUpdateFreezePrepared      = "prepared"
	RevertUpdateFreezeActive        = "active"
	RevertUpdateFreezeDrained       = "phase-zero-drained"
	RevertUpdateFreezeFinalized     = "finalized"
	// FinancialDatasetGenerationKey is a non-expiring identity for the Redis
	// financial dataset. Its {transactions} tag lets money-path Lua compare it
	// atomically with balances, backups, execution owners, and outcomes.
	FinancialDatasetGenerationKey = "financial:{transactions}:dataset-generation"
)

const inspectRevertRolloutInitializationScript = `
local current = redis.call('GET', KEYS[1])
local witness = redis.call('GET', KEYS[2])
if not current and not witness and redis.call('SCARD', KEYS[3]) == 0 then
  return 1
end
if current == 'prepared' and witness == ARGV[1] then
  return 2
end
return 0
`

const completeRevertRolloutInitializationScript = `
local current = redis.call('GET', KEYS[1])
local witness = redis.call('GET', KEYS[2])
if not current and not witness and redis.call('SCARD', KEYS[3]) == 0 then
  redis.call('SET', KEYS[2], ARGV[1])
  redis.call('SET', KEYS[1], 'prepared')
  return 1
end
if current == 'prepared' and witness == ARGV[1] then
  return 1
end
return 0
`

const activateRevertUpdateFreezeScript = `
local current = redis.call('GET', KEYS[1])
local witness = redis.call('GET', KEYS[4])
if current == 'prepared' and witness == ARGV[2] then
  if redis.call('SCARD', KEYS[2]) ~= 0 or redis.call('SCARD', KEYS[3]) ~= 0 then
    return 0
  end
  redis.call('SET', KEYS[1], ARGV[1])
  return 1
end
if current == ARGV[1] and witness == ARGV[2] then
  return 1
end
return 0
`

const advanceRevertUpdateFreezeScript = `
local current = redis.call('GET', KEYS[1])
local witness = redis.call('GET', KEYS[4])
if current == ARGV[1] and witness == ARGV[3] then
  if redis.call('SCARD', KEYS[2]) ~= 0 then
    return 0
  end
  redis.call('SET', KEYS[1], ARGV[2])
  return 1
end
if current == ARGV[2] and witness == ARGV[3] then
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
local target = ARGV[5]
local expected_generation = ARGV[6]
local rollout_generation = redis.call('GET', KEYS[9]) or ''

local ready = false
if mode == 'legacy' then
  ready = phase == '' or phase == 'prepared' or phase == 'active'
elseif mode == 'bridge' then
  ready = phase == 'active' or phase == 'phase-zero-drained'
elseif mode == 'final' then
  ready = phase == 'phase-zero-drained' or phase == 'finalized'
end

local target_ready = (target == '' and phase == '')
  or (target == 'prepared' and (phase == 'prepared' or phase == 'active'))
  or (target == 'active' and (phase == 'active' or phase == 'phase-zero-drained' or phase == 'finalized'))
  or (target == 'phase-zero-drained' and (phase == 'phase-zero-drained' or phase == 'finalized'))
  or (target == 'finalized' and phase == 'finalized')
ready = ready and target_ready
if target ~= '' then
  ready = ready and expected_generation ~= '' and rollout_generation == expected_generation
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
  if mode == 'legacy' and phase == '' then
    return 3
  end
  redis.call('SADD', KEYS[2], origin)
  return 1
end

if kind ~= 'revert' or not ready then
  return 0
end
if mode == 'legacy' then
  if phase == '' then
    redis.call('HSET', KEYS[7], attempt, 1)
    redis.call('SADD', KEYS[3], origin)
    return 6
  end
  if redis.call('SISMEMBER', KEYS[5], origin) == 1 then
    if phase == 'prepared' then
      return 7
    end
    return 5
  end
  redis.call('HSET', KEYS[7], attempt, 1)
  redis.call('SADD', KEYS[3], origin)
  if phase == 'prepared' then
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

const proveRevertRolloutCompletionScript = `
local terminal = redis.call('SISMEMBER', KEYS[3], ARGV[1])
local attempts = redis.call('HLEN', KEYS[1])
local active = redis.call('SISMEMBER', KEYS[2], ARGV[1])
if terminal == 1 and attempts == 0 and active == 0 then
  return 1
end
if terminal == 0 then
  return 0
end
return -1
`

// RevertUpdateFreezeGuard reads the deployment-wide rollout marker without a
// tenant prefix. Phase-zero, bridge, readiness, and every tenant must observe
// one shared barrier while old and bridge revert algorithms coexist.
type RevertUpdateFreezeGuard struct {
	connection              *libRedis.Client
	target                  string
	expectedGeneration      string
	initializationRequestID string
	initializationWitness   RevertRolloutInitializationWitness
}

// FinancialRedisDurabilityGuard verifies the shared Redis persistence posture
// independently of any feature rollout. Revert idempotency and Tracer Outcome
// V2 both delegate to this one trust-boundary check.
type FinancialRedisDurabilityGuard struct {
	connection *libRedis.Client
}

func NewFinancialRedisDurabilityGuard(connection *libRedis.Client) *FinancialRedisDurabilityGuard {
	return &FinancialRedisDurabilityGuard{connection: connection}
}

// RevertRolloutInitializationWitness is the deployment-scoped PostgreSQL
// birth certificate for a Redis financial dataset. Redis can prove the exact
// contents it still has, but it cannot distinguish first installation from
// loss of all of its own markers without this independent primary record.
type RevertRolloutInitializationWitness interface {
	BeginRolloutInitialization(context.Context, uuid.UUID, uuid.UUID) (prepared, created bool, err error)
	CompleteRolloutInitialization(context.Context, uuid.UUID, uuid.UUID) error
	ValidatePreparedRollout(context.Context, uuid.UUID) error
	InspectRolloutInitialization(context.Context) (exists bool, generation uuid.UUID, state string, err error)
}

// NewRevertUpdateFreezeGuard creates a deployment-wide revert rollout guard.
func NewRevertUpdateFreezeGuard(connection *libRedis.Client, settings ...string) *RevertUpdateFreezeGuard {
	guard := &RevertUpdateFreezeGuard{connection: connection}
	if len(settings) > 0 {
		guard.target = strings.ToLower(strings.TrimSpace(settings[0]))
	}

	if len(settings) > 1 {
		guard.expectedGeneration = strings.TrimSpace(settings[1])
	}

	return guard
}

// WithRolloutInitializationWitness binds the deployment PostgreSQL birth
// certificate. Initialization refuses to run without this binding.
func (g *RevertUpdateFreezeGuard) WithRolloutInitializationWitness(
	witness RevertRolloutInitializationWitness,
	initializationRequestID string,
) *RevertUpdateFreezeGuard {
	g.initializationWitness = witness
	g.initializationRequestID = strings.TrimSpace(initializationRequestID)

	return g
}

// FinancialDurability verifies the Redis trust boundary required while Redis
// is the authoritative pre-PostgreSQL money-path store. Every cluster shard
// must reject eviction and have healthy AOF persistence. This cannot promise a
// smaller RPO than the configured appendfsync policy; it only fails closed when
// the required durability mechanism is absent or unhealthy.
func (g *RevertUpdateFreezeGuard) FinancialDurability(ctx context.Context) error {
	if g == nil {
		return fmt.Errorf("revert rollout Redis connection not configured")
	}

	return NewFinancialRedisDurabilityGuard(g.connection).FinancialDurability(ctx)
}

func (g *FinancialRedisDurabilityGuard) FinancialDurability(ctx context.Context) error {
	if g == nil || g.connection == nil {
		return fmt.Errorf("financial Redis connection not configured")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get financial Redis client: %w", err)
	}

	if cluster, ok := client.(*redislib.ClusterClient); ok {
		if err := cluster.ForEachShard(ctx, func(ctx context.Context, shard *redislib.Client) error {
			return validateFinancialRedisNode(ctx, shard)
		}); err != nil {
			return fmt.Errorf("validate financial Redis cluster durability: %w", err)
		}

		return nil
	}

	return validateFinancialRedisNode(ctx, client)
}

// InitializeFinancialDatasetGeneration is the one-shot transition from an
// uninitialized Redis dataset to prepared phase zero. The deployment supplies
// the immutable generation; every serving target validates it and can never
// recreate missing shared state.
func (g *RevertUpdateFreezeGuard) InitializeFinancialDatasetGeneration(ctx context.Context) error {
	if g == nil || g.connection == nil {
		return fmt.Errorf("revert rollout Redis connection not configured")
	}

	if g.initializationWitness == nil {
		return fmt.Errorf("revert rollout PostgreSQL birth certificate is required")
	}

	redisGeneration, err := uuid.Parse(g.expectedGeneration)
	if err != nil {
		return fmt.Errorf("financial Redis dataset generation must be a UUID: %w", err)
	}

	initializationRequestID, err := uuid.Parse(g.initializationRequestID)
	if err != nil {
		return fmt.Errorf("revert rollout initialization request id must be a UUID: %w", err)
	}

	prepared, _, err := g.initializationWitness.BeginRolloutInitialization(ctx, redisGeneration,
		initializationRequestID)
	if err != nil {
		return fmt.Errorf("begin revert rollout initialization: %w", err)
	}

	if prepared {
		return g.ValidatePrepared(ctx)
	}

	if err := g.prepareFinancialDatasetGeneration(ctx); err != nil {
		return err
	}

	if err := g.initializationWitness.CompleteRolloutInitialization(ctx, redisGeneration,
		initializationRequestID); err != nil {
		return fmt.Errorf("complete revert rollout PostgreSQL birth certificate: %w", err)
	}

	return g.ValidatePrepared(ctx)
}

func (g *RevertUpdateFreezeGuard) prepareFinancialDatasetGeneration(ctx context.Context) error {
	if err := g.FinancialDurability(ctx); err != nil {
		return fmt.Errorf("initialize revert rollout without durable financial Redis: %w", err)
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get revert rollout Redis client: %w", err)
	}

	initializationState, err := client.Eval(ctx, inspectRevertRolloutInitializationScript,
		[]string{RevertUpdateFreezeKey, RevertRolloutGenerationKey, revertPhaseZeroRequestLeaseKey},
		g.expectedGeneration).Int64()
	if err != nil {
		return fmt.Errorf("inspect revert rollout initialization: %w", err)
	}

	if initializationState == 0 {
		return fmt.Errorf("initialize revert rollout: existing marker or generation witness differs")
	}

	if initializationState == 2 {
		if err := g.validateRedisFinancialDatasetGeneration(ctx); err != nil {
			return fmt.Errorf("validate completed revert rollout initialization: %w", err)
		}

		return nil
	}

	created, err := client.SetNX(ctx, FinancialDatasetGenerationKey, g.expectedGeneration, 0).Result()
	if err != nil {
		return fmt.Errorf("prepare financial Redis dataset generation: %w", err)
	}

	if !created {
		generation, generationErr := g.financialDatasetGeneration(ctx)
		if generationErr != nil {
			return generationErr
		}

		if generation != g.expectedGeneration {
			return fmt.Errorf("financial Redis dataset generation differs from configured witness")
		}
	}

	initialized, err := client.Eval(ctx, completeRevertRolloutInitializationScript,
		[]string{RevertUpdateFreezeKey, RevertRolloutGenerationKey, revertPhaseZeroRequestLeaseKey},
		g.expectedGeneration).Bool()
	if err != nil {
		return fmt.Errorf("complete revert rollout initialization: %w", err)
	}

	if !initialized {
		return fmt.Errorf("complete revert rollout initialization: state changed concurrently")
	}

	return nil
}

// ValidatePrepared verifies the immutable financial generation and its
// rollout-slot witness. It never writes either key.
func (g *RevertUpdateFreezeGuard) ValidatePrepared(ctx context.Context) error {
	if err := g.validateGeneration(ctx); err != nil {
		return err
	}

	phase, witness, err := g.rolloutStateAndGeneration(ctx)
	if err != nil {
		return err
	}

	if phase != RevertUpdateFreezePrepared || witness != g.expectedGeneration {
		return fmt.Errorf("prepared revert rollout marker or generation witness differs")
	}

	return nil
}

// FinancialDatasetGeneration returns the current non-expiring financial Redis
// identity. Absence is a trust-boundary failure once durable revert claims can
// exist; callers must never create it as part of recovery.
func (g *RevertUpdateFreezeGuard) FinancialDatasetGeneration(ctx context.Context) (string, error) {
	if err := g.validateRolloutBirthCertificate(ctx); err != nil {
		return "", err
	}

	return g.financialDatasetGeneration(ctx)
}

func (g *RevertUpdateFreezeGuard) financialDatasetGeneration(ctx context.Context) (string, error) {
	if g == nil || g.connection == nil {
		return "", fmt.Errorf("revert rollout Redis connection not configured")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return "", fmt.Errorf("get revert rollout Redis client: %w", err)
	}

	generation, err := client.Get(ctx, FinancialDatasetGenerationKey).Result()
	if err != nil {
		if errors.Is(err, redislib.Nil) {
			return "", fmt.Errorf("financial Redis dataset generation is missing")
		}

		return "", fmt.Errorf("read financial Redis dataset generation: %w", err)
	}

	if strings.TrimSpace(generation) == "" {
		return "", fmt.Errorf("financial Redis dataset generation is missing")
	}

	return generation, nil
}

func (g *RevertUpdateFreezeGuard) validateGeneration(ctx context.Context) error {
	if strings.TrimSpace(g.expectedGeneration) == "" {
		return fmt.Errorf("financial Redis dataset generation is required")
	}

	if err := g.FinancialDurability(ctx); err != nil {
		return fmt.Errorf("financial Redis durability: %w", err)
	}

	return g.ValidateFinancialDatasetGeneration(ctx)
}

// ValidateFinancialDatasetGeneration compares the financial-slot identity to
// the immutable deployment witness without creating either value.
func (g *RevertUpdateFreezeGuard) ValidateFinancialDatasetGeneration(ctx context.Context) error {
	if err := g.validateRolloutBirthCertificate(ctx); err != nil {
		return err
	}

	return g.validateRedisFinancialDatasetGeneration(ctx)
}

func (g *RevertUpdateFreezeGuard) validateRedisFinancialDatasetGeneration(ctx context.Context) error {
	if strings.TrimSpace(g.expectedGeneration) == "" {
		return fmt.Errorf("financial Redis dataset generation is required")
	}

	generation, err := g.financialDatasetGeneration(ctx)
	if err != nil {
		return err
	}

	if generation != g.expectedGeneration {
		return fmt.Errorf("financial Redis dataset generation differs from configured witness")
	}

	return nil
}

func (g *RevertUpdateFreezeGuard) validateRolloutBirthCertificate(ctx context.Context) error {
	if g == nil || g.initializationWitness == nil {
		return fmt.Errorf("revert rollout PostgreSQL birth certificate is required")
	}

	if g.target == "" {
		exists, _, state, err := g.initializationWitness.InspectRolloutInitialization(ctx)
		if err != nil {
			return fmt.Errorf("inspect revert rollout PostgreSQL birth certificate: %w", err)
		}

		if exists {
			return fmt.Errorf("released legacy rollout is fenced by %s PostgreSQL birth certificate", state)
		}

		return nil
	}

	generation, err := uuid.Parse(g.expectedGeneration)
	if err != nil {
		return fmt.Errorf("financial Redis dataset generation must be a UUID: %w", err)
	}

	if err := g.initializationWitness.ValidatePreparedRollout(ctx, generation); err != nil {
		return fmt.Errorf("validate revert rollout PostgreSQL birth certificate: %w", err)
	}

	return nil
}

func (g *RevertUpdateFreezeGuard) rolloutStateAndGeneration(ctx context.Context) (string, string, error) {
	if g == nil || g.connection == nil {
		return "", "", fmt.Errorf("revert rollout Redis connection not configured")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return "", "", fmt.Errorf("get revert rollout Redis client: %w", err)
	}

	values, err := client.MGet(ctx, RevertUpdateFreezeKey, RevertRolloutGenerationKey).Result()
	if err != nil {
		return "", "", fmt.Errorf("read revert rollout marker and generation witness: %w", err)
	}

	phase := ""
	witness := ""

	if values[0] != nil {
		phase = fmt.Sprint(values[0])
	}

	if values[1] != nil {
		witness = fmt.Sprint(values[1])
	}

	return phase, witness, nil
}

func validateFinancialRedisNode(ctx context.Context, client redislib.Cmdable) error {
	policy, err := client.ConfigGet(ctx, "maxmemory-policy").Result()
	if err != nil {
		return fmt.Errorf("read maxmemory policy: %w", err)
	}

	if policy["maxmemory-policy"] != "noeviction" {
		return fmt.Errorf("maxmemory-policy must be noeviction")
	}

	aof, err := client.ConfigGet(ctx, "appendonly").Result()
	if err != nil {
		return fmt.Errorf("read appendonly policy: %w", err)
	}

	if aof["appendonly"] != "yes" {
		return fmt.Errorf("appendonly must be enabled")
	}

	fsync, err := client.ConfigGet(ctx, "appendfsync").Result()
	if err != nil {
		return fmt.Errorf("read appendfsync policy: %w", err)
	}

	if fsync["appendfsync"] != "always" && fsync["appendfsync"] != "everysec" {
		return fmt.Errorf("appendfsync must be always or everysec")
	}

	info, err := client.Info(ctx, "persistence").Result()
	if err != nil {
		return fmt.Errorf("read persistence health: %w", err)
	}

	if redisInfoField(info, "aof_enabled") != "1" || redisInfoField(info, "aof_last_write_status") != "ok" {
		return fmt.Errorf("AOF persistence is not healthy")
	}

	return nil
}

func redisInfoField(info, name string) string {
	for _, line := range strings.Split(info, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if found && key == name {
			return strings.TrimSpace(value)
		}
	}

	return ""
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
	if err := g.validateRolloutBirthCertificate(ctx); err != nil {
		return false, false, err
	}

	value, witness, err := g.rolloutStateAndGeneration(ctx)
	if err != nil {
		return false, false, err
	}

	frozen := value == RevertUpdateFreezeActive || value == RevertUpdateFreezeDrained
	witnessReady := g.target == "" || (g.expectedGeneration != "" && witness == g.expectedGeneration)

	return frozen, witnessReady && revertModeReadyForPhase(value, mode) && revertTargetReached(value, g.target), nil
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
		return true, true, RevertUpdateFreezePrepared, nil
	case 5:
		if mode == "legacy" {
			return true, false, RevertUpdateFreezeActive, nil
		}

		return true, false, "", nil
	case 6:
		return true, true, RevertUpdateFreezeUninitialized, nil
	case 7:
		return true, false, RevertUpdateFreezePrepared, nil
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

	if err := g.validateRolloutBirthCertificate(ctx); err != nil {
		return 0, err
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return 0, fmt.Errorf("get revert update freeze Redis client: %w", err)
	}

	if g.target != "" {
		generation, generationErr := client.Get(ctx, FinancialDatasetGenerationKey).Result()
		if errors.Is(generationErr, redislib.Nil) || (generationErr == nil && generation != g.expectedGeneration) {
			return 0, nil
		}

		if generationErr != nil {
			return 0, fmt.Errorf("read financial Redis dataset generation before rollout admission: %w", generationErr)
		}
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
		RevertRolloutGenerationKey,
	}, mode, kind, token, attemptID, g.target, g.expectedGeneration).Int64()
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

// RevertTerminalHandoffComplete proves in one rollout-slot Lua read that the
// exact generation is terminal and cannot retain or recreate an active attempt.
func (g *RevertUpdateFreezeGuard) RevertTerminalHandoffComplete(
	ctx context.Context,
	mode, token string,
) (bool, error) {
	key, err := revertRolloutLeaseKey(mode)
	if err != nil {
		return false, err
	}

	if key == "" {
		return true, nil
	}

	if g == nil || g.connection == nil {
		return false, fmt.Errorf("revert update freeze Redis connection not configured")
	}

	if token == "" {
		return false, fmt.Errorf("revert rollout request token is required")
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return false, fmt.Errorf("get revert update freeze Redis client: %w", err)
	}

	completedKey := revertPhaseZeroCompletedKey
	if mode == "bridge" {
		completedKey = revertBridgeCompletedKey
	}

	result, err := client.Eval(ctx, proveRevertRolloutCompletionScript, []string{
		revertRolloutOriginAttemptKey(mode, token), key, completedKey,
	}, token).Int64()
	if err != nil {
		return false, fmt.Errorf("prove revert rollout terminal handoff: %w", err)
	}

	if result < 0 {
		return false, fmt.Errorf("revert rollout terminal handoff is inconsistent")
	}

	return result == 1, nil
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
	if err := g.validateRolloutBirthCertificate(ctx); err != nil {
		return false, err
	}

	value, witness, err := g.rolloutStateAndGeneration(ctx)
	if err != nil {
		return false, err
	}

	witnessReady := g.target == "" || (g.expectedGeneration != "" && witness == g.expectedGeneration)

	return witnessReady && revertModeReadyForPhase(value, mode) && revertTargetReached(value, g.target), nil
}

func revertTargetReached(value, target string) bool {
	switch target {
	case "":
		return value == ""
	case RevertUpdateFreezeInitialize:
		return false
	case RevertUpdateFreezePrepared:
		return value == RevertUpdateFreezePrepared || value == RevertUpdateFreezeActive
	case RevertUpdateFreezeActive:
		return value == RevertUpdateFreezeActive || value == RevertUpdateFreezeDrained ||
			value == RevertUpdateFreezeFinalized
	case RevertUpdateFreezeDrained:
		return value == RevertUpdateFreezeDrained || value == RevertUpdateFreezeFinalized
	case RevertUpdateFreezeFinalized:
		return value == RevertUpdateFreezeFinalized
	default:
		return false
	}
}

func revertModeReadyForPhase(value, mode string) bool {
	switch mode {
	case "legacy":
		return value == "" || value == RevertUpdateFreezePrepared || value == RevertUpdateFreezeActive
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

	if err := g.validateGeneration(ctx); err != nil {
		return fmt.Errorf("activate revert update freeze without prepared financial dataset: %w", err)
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get revert update freeze Redis client: %w", err)
	}

	activated, err := client.Eval(ctx, activateRevertUpdateFreezeScript,
		[]string{RevertUpdateFreezeKey, revertApprovedUpdateLeaseKey, revertPhaseZeroRequestLeaseKey,
			RevertRolloutGenerationKey},
		RevertUpdateFreezeActive, g.expectedGeneration).Bool()
	if err != nil {
		return fmt.Errorf("activate revert update freeze: %w", err)
	}

	if !activated {
		return fmt.Errorf("activate revert update freeze: expected prepared phase, exact generation, and complete drain")
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

	if err := g.validateGeneration(ctx); err != nil {
		return fmt.Errorf("%s without exact financial dataset generation: %w", operation, err)
	}

	client, err := g.connection.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get revert update freeze Redis client: %w", err)
	}

	advanced, err := client.Eval(ctx, advanceRevertUpdateFreezeScript,
		[]string{RevertUpdateFreezeKey, leaseKey, completedKey, RevertRolloutGenerationKey},
		from, to, g.expectedGeneration).Bool()
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	if !advanced {
		return fmt.Errorf("%s: expected %q phase", operation, from)
	}

	return nil
}
