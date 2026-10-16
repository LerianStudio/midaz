# Balance-engine bench environment

Disposable local Postgres HA stack for the balance-engine bench (Compose project
`midaz-bench-engine`). Run only one instance per host; other worktrees share these fixed names:

| Service | Role | Host bind (override) |
|---|---|---|
| `bench-pg-primary` | PG17 primary, `cluster_name=bench-pg-primary` | `127.0.0.1:15432` (`BENCH_PG_PRIMARY_PORT`) |
| `bench-pg-standby` | PG17 synchronous standby `standby1`, `cluster_name=bench-pg-standby` | `127.0.0.1:15435` (`BENCH_PG_STANDBY_PORT`) |
| `bench-toxiproxy` | toxiproxy 2.12.0: `pg-client` proxy and API | `127.0.0.1:15434` (`BENCH_PG_CLIENT_PROXY_PORT`), API `127.0.0.1:18474` (`BENCH_TOXIPROXY_PORT`) |

Both proxies point at the primary. `pg-repl` (container port 5433) carries the standby's
replication and is not published on the host; `pg-client` carries client SQL, so client
faults and replication faults can be injected separately through the toxiproxy API.

> **Warning:** `bench`/`bench` are disposable bench credentials for this local stack only.
> The database `bench_transaction` carries the comment `midaz-bench-engine:disposable`
> on both nodes; destructive bench tooling should check that marker and `cluster_name`
> before touching a node.

## Commands

Run from the repository root. Pull once (toxiproxy 2.12.0 is not a common local image),
then the stack starts without network access:

```bash
docker compose -f tests/bench/engine/docker-compose.yml pull
docker compose -f tests/bench/engine/docker-compose.yml up -d --wait --wait-timeout 120
```

DSNs (use `127.0.0.1`; Docker binds IPv4 loopback only):

```text
primary   postgresql://bench:bench@127.0.0.1:15432/bench_transaction
standby   postgresql://bench:bench@127.0.0.1:15435/bench_transaction
pg-client postgresql://bench:bench@127.0.0.1:15434/bench_transaction
toxiproxy http://127.0.0.1:18474
```

Post-up check (requires host `psql` and `timeout`). The healthchecks prove the standby is
in recovery with a streaming WAL receiver; the primary must show exactly one synchronous `standby1`:

```bash
P="postgresql://bench:bench@127.0.0.1:${BENCH_PG_PRIMARY_PORT:-15432}/bench_transaction?connect_timeout=5"
timeout 30 sh -c "until [ \"\$(psql $P -tAXc \"select count(*) filter (where application_name = 'standby1' and state = 'streaming' and sync_state = 'sync'), count(*) from pg_stat_replication\")\" = '1|1' ]; do sleep 1; done"
```

Stop, keeping data: `docker compose -f tests/bench/engine/docker-compose.yml down`.
Remove the bench volumes too: add `-v`. Both commands touch only this project.

## Schema bootstrap

`bootstrap.go` rebuilds the `public` schema of `bench_transaction`: ledger migrations
000000–000046 through the lib-commons Migrator (one file per Exec, so migration 46's
`CREATE INDEX CONCURRENTLY` runs outside a transaction), then the lib-commons `outbox_events`
schema from the module version `go.mod` pins, then `schema_bench.sql`. Before any DDL it refuses
unless:

- the four `BENCH_*` variables resolve to loopback IP literals, user `bench` and database
  `bench_transaction`, after query overrides, fallback hosts and `PG*` defaults;
- primary and client proxy report `cluster_name=bench-pg-primary`, no recovery and
  `synchronous_commit=on`; the standby reports `bench-pg-standby` in recovery; all three carry the
  marker and one `system_identifier`;
- `standby1` is the only replica, `streaming | sync`, and the standby's WAL receiver streams.

Limits: the guard waits at most 120 s for `standby1`. `lock_timeout=30s` turns a lock held by
another session into a failure; no session is terminated. The Migrator stops watching the context
once it starts, so `go test -timeout` bounds the migrations. Every commit waits for `standby1` and
nothing downgrades `synchronous_commit`: a lost standby stalls the run until that timeout fails it.

```bash
env BENCH_PG_PRIMARY_DSN='postgresql://bench:bench@127.0.0.1:15432/bench_transaction?sslmode=disable' \
  BENCH_PG_CLIENT_PROXY_DSN='postgresql://bench:bench@127.0.0.1:15434/bench_transaction?sslmode=disable' \
  BENCH_PG_STANDBY_DSN='postgresql://bench:bench@127.0.0.1:15435/bench_transaction?sslmode=disable' \
  BENCH_TOXIPROXY_URL='http://127.0.0.1:18474' \
  go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestBenchEnv|TestBootstrapSchema|TestBootstrapRejects' -timeout 10m -v
```

Without the four variables the database tests skip; a partial set fails them. Each database test
sets `ALLOW_INSECURE_TLS=true` in the test process for its own duration, as the Migrator requires
on this `sslmode=disable` stack. The refusal test changes the marker only after the guard proves
the stack, and restores it on cleanup. A run killed before cleanup fails closed: every later
bootstrap refuses the stack until it is recreated with `down -v` and `up`.

## Seed fixtures

`seed.go` re-runs the guard, then in one synchronous transaction streams N ordinary balances plus
hot, fee and two multikey balances (N+4) and their controls (N+3: multikey's two keys share one
account) with `COPY`, and inserts an ownerless fence whose lease already expired. IDs derive from
class, index and key, so every run writes the same rows. `fillfactor=70` on `balance` and
`account_control` is an untested HOT-update hypothesis; `VACUUM (ANALYZE)` follows the commit.
`TestBootstrap` seeds 10k twice (rows must hash alike) and 1M once, with 60 s and 10 min seed
deadlines. At 1M it checks counts, sums, constant columns and named rows, not every row's IDs:

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBootstrap$' -timeout 30m -v
```

## How it is built

- `init-primary.sql` runs once on a fresh primary volume. It writes the marker and
  persists the settings listed in that file with `ALTER SYSTEM`, without reload.
- The standby runs `pg_basebackup -R --no-slot --checkpoint=fast` through `pg-repl` with
  `application_name=standby1` on its first start, so it inherits the same
  `postgresql.auto.conf`. Only `cluster_name` differs, set on each command line.
- Either primary, including a promoted standby, requires `standby1` to confirm synchronous
  commits. Without that replica, commits wait.
- No service has a restart policy: a stopped primary stays stopped.
- `down` without `-v` keeps both volumes; the next `up` skips init and base backup, and
  both nodes keep the same system identifier.

## What this proves

Environment readiness, guarded schema rebuild and deterministic seed fixtures. The synchronous
standby streams through the proxy and inherits the primary's settings. Seed timing is setup cost,
not transaction throughput. Financial correctness, failover survival and unknown replay are untested.
