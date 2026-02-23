# Midaz 50K TPS Strategy — Adaptive Account Sharding

## Executive Summary

Midaz's path to 50K TPS is a three-phase journey. The fundamental bottleneck is **Redis single-threaded Lua execution** — the `balance_atomic_operation.lua` blocks the entire Redis event loop during execution, creating a hard ceiling of ~3-5K TPS on a single instance.

The solution combines:
1. **Lua optimization** — integer arithmetic replacing string-based decimal math (3-5x speedup)
2. **Adaptive account sharding** — Redis Cluster with 8 shards, transparent external account pre-split, and dynamic account migration based on load
3. **Persistence batching** — bulk PG writes, micro-batching, and connection pooling

```
Current State:    ~3-5K TPS   (single Redis Lua bottleneck)
Phase 1 Target:   ~10-15K TPS (Lua optimization + tuning)
Phase 2A Target:  ~30-50K TPS (static sharding + external pre-split)
Phase 2B Target:  ~50K+ TPS   (dynamic migration + load balancer)
Phase 3 Target:   ~50-100K+   (batch persistence + advanced pipelining)
```

## Why Not Shard by Ledger?

Typical Midaz deployments have 1-3 ledgers. Ledger-based sharding gives 1-3 shards — negligible improvement. The shard boundary must be **sub-ledger**.

## Why Account-Based Sharding?

- Accounts have high cardinality (thousands to millions)
- Each balance belongs to exactly one account
- The `@external/{ASSET}` hotspot (55-75% of all transactions) can be solved by transparent pre-splitting

## Architecture Overview

### Core Principles

1. **Shard by account, not by ledger** — account alias hash gives unlimited shard granularity
2. **External accounts pre-split transparently** — `@external/USD` gets per-shard balance keys automatically. API consumers never see this.
3. **Two-layer routing: static default + dynamic overrides** — 99% of accounts use deterministic hash (zero lookup cost). Only migrated accounts hit the routing table.
4. **Debit-first, credit-second for cross-shard** — debits are the validation gate. Credits always succeed. Compensation reverses debits on failure.
5. **Auto-rebalancing with guardrails** — system detects hotspots and migrates accounts with rate limits and anti-thrash protection.

### System Topology

```
                     +------------------+
                     |   Load Balancer   |
                     |  (Envoy/Traefik)  |
                     +--------+---------+
                              |
              +---------------+---------------+
              v               v               v
     +------------+  +------------+  +------------+
     |  Midaz #1   |  |  Midaz #2   |  |  Midaz #N   |
     |  (stateless)|  |  (stateless)|  |  (stateless)|
     |             |  |             |  |             |
     | ShardRouter |  | ShardRouter |  | ShardRouter |
     | (local cache|  | (local cache|  | (local cache|
     |  + pub/sub) |  |  + pub/sub) |  |  + pub/sub) |
     +------+------+  +------+------+  +------+------+
            |                |                |
     +------v----------------v----------------v------+
     |              Redis Cluster (8 shards)           |
     |                                                 |
     |  Shard 0 ---- Shard 1 ---- Shard 2 ---- ...   |
     |   | Lua         | Lua         | Lua            |
     |   | Balances     | Balances    | Balances       |
     |   | Sync sched   | Sync sched  | Sync sched    |
     |   | Backup queue | Backup queue| Backup queue   |
     |                                                 |
     |  Control plane (any slot):                      |
     |   shard_routing:{org:ledger}  (override table)  |
     |   shard_metrics:{shard_N}     (load tracking)   |
     |   migration:{org:ledger}:alias (freeze locks)   |
     +--------------------+----------------------------+
                          |
     +--------------------v----------------------------+
     |   Redpanda (topic partitions per shard)          |
     |                                                  |
     |  transaction_bto.shard_0  (10 workers)          |
     |  transaction_bto.shard_1  (10 workers)          |
     |  ...                                             |
     |  transaction_bto.shard_7  (10 workers)          |
     +--------------------+----------------------------+
                          |
     +--------------------v----------------------------+
     |            PgBouncer (pool=100)                  |
     |                    |                             |
     |   +----------------v-----------------+          |
     |   |  PostgreSQL Primary               |          |
     |   |  (partitioned by ledger_id)       |          |
     |   |   +-- transaction_p00..p15        |          |
     |   |   +-- operation_p00..p15          |          |
     |   |   +-- balance_p00..p15            |          |
     |   +----------------+-----------------+          |
     |                    | WAL streaming               |
     |   +----------------v-----------------+          |
     |   |  PostgreSQL Replica (reads)       |          |
     |   +----------------------------------+          |
     +--------------------------------------------------+
```

### Transaction Flow

#### Case 1: Inflow (55-75% of traffic) — Same-Shard

```
POST /transactions/inflow  { asset: "USD", amount: 100, to: "@user_123" }

1. BuildInflowEntry: source = @external/USD, dest = @user_123

2. ShardRouter:
   user_shard = Resolve(@user_123)                          -> shard 3
   external_key = resolveExternalBalanceKey(@user_123)      -> "shard_3"
   
   @external/USD#shard_3  -> shard 3  (same!)
   @user_123#default      -> shard 3  (same!)

3. SINGLE Lua call on Shard 3:
   DEBIT @external/USD#shard_3  (no balance check for external debits)
   CREDIT @user_123#default     (add available)
   -> atomic, ~30us

4. Return 201 Created
```

#### Case 2: Internal Transfer (25-45% of traffic) — Cross-Shard

```
POST /transactions/json  { source: @user_A, dest: @user_B, amount: 50 }

1. ShardRouter:
   @user_A -> shard 2
   @user_B -> shard 5

2. Group by shard:
   Shard 2: [DEBIT @user_A]
   Shard 5: [CREDIT @user_B]

3. Execute DEBITs (per-shard Lua):
   Shard 2 Lua: validate available >= 50, subtract -> returns pre-update state
   
   If DEBIT fails -> return 422 Insufficient Funds

4. Execute CREDITs (per-shard Lua):
   Shard 5 Lua: add 50 to @user_B available -> always succeeds

5. Return 201 Created
```

#### Case 3: Complex N-to-M — Mixed Shards

```
1. Group all operations by shard
2. Execute DEBIT shards in parallel (per-shard Lua, atomic within shard)
3. If any DEBIT fails -> compensate successful shards (reverse debits)
4. Execute remaining CREDIT shards in parallel
5. Return success
```

### Redis Key Architecture

| Key | Pattern | Hash Tag | Purpose |
|-----|---------|----------|---------|
| Balance | `balance:{shard_N}:org:ledger:alias#key` | `{shard_N}` | Per-account balance state |
| Backup | `backup:{shard_N}:org:ledger:txnID` | `{shard_N}` | Per-shard crash recovery |
| Sync schedule | `schedule:{shard_N}:balance-sync` | `{shard_N}` | Per-shard TTL sync |
| Sync lock | `lock:{shard_N}:balance-sync:key` | `{shard_N}` | Per-shard sync locking |
| Shard routing | `shard_routing:{org:ledger}` | `{org:ledger}` | Override table |
| Migration lock | `migration:{org:ledger}:alias` | `{org:ledger}` | Account freeze |
| Shard metrics | `shard_metrics:{shard_N}` | `{shard_N}` | Per-shard load tracking |
| Idempotency | `idempotency:{org:ledger:key}` | `{org:ledger:key}` | Unchanged |
| Pending lock | `pending:{org:ledger}:txnID` | `{org:ledger}` | Unchanged |

### Account Migration Protocol

When moving Account X from Shard A to Shard B:

```
Phase 1: Freeze (~0ms)
  SET migration:{org:ledger}:@X  "A->B"  EX 30
  (All new txns touching @X see lock, retry with 1ms backoff)

Phase 2: Drain (~5-10ms)
  Wait for in-flight Lua ops on @X to complete

Phase 3: Copy (~100us)
  balance = GET balance:{shard_A}:org:ledger:@X#key
  SET balance:{shard_B}:org:ledger:@X#key  balance

Phase 4: Switch (~100us)
  HSET shard_routing:{org:ledger}  @X  B
  PUBLISH shard_routing_updates:{org:ledger}  "@X:B"

Phase 5: Cleanup (~100us)
  DEL balance:{shard_A}:org:ledger:@X#key
  DEL migration:{org:ledger}:@X
```

Total migration time: ~10-15ms per account. Invisible at HTTP level.

### Load Balancer Guardrails

- Max 1 migration per 10 seconds per shard
- Anti-thrash cooldown: 5 minutes per account
- Imbalance threshold: max_shard_load > 1.5x avg_shard_load
- Admin API can pause/override migrations

## Implementation Phases

### Phase 1: Lua + Mechanical Tuning (Weeks 1-4)

| # | Task | Impact | Effort |
|---|------|--------|--------|
| 1.1 | Lua integer arithmetic conversion | 3-5x Lua speedup | 5d |
| 1.1a | Precision guard (API layer) | Safety | 1d |
| 1.1b | Precision guard (Lua layer) | Safety | 0.5d |
| 1.1c | Scale factor config per asset | Foundation | 2d |
| 1.2 | Redis pool size (10->200) | Immediate concurrency unlock | 0.5d |
| 1.3 | PG batch writes (CTE + multi-row) | 5-10x PG write throughput | 5d |
| 1.4 | Redpanda consumers (5->50) | 10x consumer throughput | 0.5d |
| 1.5 | PgBouncer + connection pool fix | Prevents connection exhaustion | 2d |
| 1.6 | HTTP server tuning (Fiber config) | Prevents timeout/resource issues | 1d |

### Phase 2A: Static Sharding + External Pre-Split (Weeks 3-6)

| # | Task | Impact | Effort | Status |
|---|------|--------|--------|--------|
| 2A.1 | Shard router (static hash) | Foundation for sharding | 3d | **DONE** |
| 2A.2 | Redis key migration | Enable Redis Cluster | 5d | **DONE** |
| 2A.3 | External account pre-split | 55-75% txns become same-shard | 3d | Pending |
| 2A.4 | Per-shard Lua script | Enable parallel Lua execution | 5d | **DONE** |
| 2A.5 | Cross-shard orchestrator | Enable cross-shard transactions | 5d | **DONE** |
| 2A.6 | PG table partitioning | PG write scalability | 2d | Pending |
| 2A.7 | Redpanda per-shard partitions | Consumer scalability | 3d | Pending |
| 2A.8 | Horizontal scaling infra | Multi-instance deployment | 3d | Pending |

### Phase 2B: Dynamic Migration + Load Balancer (Weeks 7-10)

| # | Task | Impact | Effort |
|---|------|--------|--------|
| 2B.1 | Routing table + cache | Dynamic shard assignment | 5d |
| 2B.2 | Migration protocol | Account mobility | 5d |
| 2B.3 | Load monitoring | Hotspot detection | 2d |
| 2B.4 | Load balancer worker | Auto-rebalancing | 5d |
| 2B.5 | Admin API | Operational control | 3d |

### Phase 3: Advanced Optimizations (Weeks 9-12)

| # | Task | Impact | Effort |
|---|------|--------|--------|
| 3.1 | Redis pipeline pre-Lua ops | 3 RTTs -> 1 RTT | 2d |
| 3.2 | Write-behind micro-batching | 10-20x PG throughput | 5d |
| 3.3 | Read replica routing | Read scalability | 2d |
| 3.4 | Per-shard sync worker | Sync scalability | 3d |

## Performance Projections

| Metric | Current | Phase 1 | Phase 2A | Phase 2B | Phase 3 |
|--------|---------|---------|----------|----------|---------|
| Redis Lua calls/sec | ~5K | ~20-30K | ~160-240K | Same | ~200-300K |
| Same-shard % | 100% (1) | 100% (1) | ~65% | ~70%+ | Same |
| PG writes/sec | ~5-10K | ~30-50K | ~50-100K | Same | ~200K+ |
| Redpanda msg/sec | ~5-10K | ~20-30K | ~80-160K | Same | ~100-200K |
| **End-to-end TPS** | **~3-5K** | **~10-15K** | **~30-50K** | **~50K+** | **~50-100K+** |

## Risk Matrix

| Risk | Phase | Severity | Probability | Mitigation |
|------|-------|----------|-------------|------------|
| Lua integer overflow | 1 | High | Low | 3-layer guard: API + Go boundary + Lua |
| Cross-shard partial commit | 2A | High | Low | Credit always succeeds + compensation + backup |
| Migration data loss | 2B | High | Very Low | Freeze-drain + 30s lock TTL + copy-before-switch |
| Load balancer thrashing | 2B | Medium | Medium | Anti-thrash cooldown (5min) + rate limit |
| Redis Cluster slot imbalance | 2A | Medium | Low | 8 shards = 2048 slots each |
| Routing table inconsistency | 2B | Medium | Low | TTL cache + pub/sub + Redis source of truth |
