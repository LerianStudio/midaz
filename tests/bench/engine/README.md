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

Environment readiness only: a synchronous standby that streams through the proxy and
inherits the primary's settings. It proves nothing about financial correctness,
failover survival, replay of unknown outcomes or throughput.
