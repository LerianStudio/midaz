//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/backoff"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Opt-in negative control on the owned topology. Approval D commits through standby1 under synchronous_commit=on; with
// replication cut, probe P then commits under local on the primary alone. The primary is killed and the standby promoted:
// a fresh session there finds the durable baseline and D in full, and exactly P lost.
func TestOwnerlessAsyncLossDetected(t *testing.T) {
	o := upOwnedTopology(t)

	setupAt := time.Now()
	g := newGate(t, false)
	balances, controls := assertSeeded(g.ctx, t, g.conn, 4)

	durable, dg := *g.env, *g
	durable.primary = g.env.primary.Copy()
	durable.primary.AfterConnect = requireCommitSetting(t, "on")
	dg.env = &durable
	d, _ := dg.start(t, "bench-durable", 1, gateMaxBatch, 1)

	// One implicit transaction, committed through standby1 under the guarded synchronous_commit=on.
	_, err := g.conn.Exec(g.ctx, `CREATE TABLE public.base_balance AS TABLE pg_temp.base_balance;
		CREATE TABLE public.base_control AS TABLE pg_temp.base_control`)
	require.NoError(t, err)

	durableReq := batchReq(t, "promotion/durable", "promotion/durable", g.clock.now(), ordinaryOf(0)(-5), ordinaryOf(1)(5))
	require.Equal(t, []string{"approved+tx"}, summarize(t, []request{durableReq}, submitEach(g.ctx, t, d, []request{durableReq}).wait(t)))

	local := *g.env
	local.primary = g.env.primary.Copy()
	local.primary.RuntimeParams["synchronous_commit"] = "local"
	local.primary.AfterConnect = requireCommitSetting(t, "local")
	p := captureCalls(newTestOwnerless(g.ctx, t, &local, g.b, "bench-async-loss", 1, gateMaxBatch), 1)

	// No restore is registered: a pass ends with this primary killed, and a failure keeps the cut as evidence.
	cutAt := time.Now()

	require.NoError(t, quietProxy(t, g.env, replProxyName, ":5433").Disable(), "disable %s through the toxiproxy API", replProxyName)
	awaitPrimary(g.ctx, t, g.conn, "the standby's WAL sender is gone", noWalSender)

	var cut, inserted, received string
	require.NoError(t, g.conn.QueryRow(g.ctx, `SELECT pg_current_wal_insert_lsn()::text`).Scan(&cut))

	probeAt := time.Now()
	probe := batchReq(t, "promotion/async-loss", "promotion/async-loss", g.clock.now(), ordinaryOf(1)(-5), ordinaryOf(2)(5))
	// P overlaps balance 1, so the promoted row must show D's after-state, never P's.
	require.Equal(t, []string{"approved+tx"}, summarize(t, []request{probe}, submitEach(g.ctx, t, p, []request{probe}).wait(t)))
	require.Equal(t, 1, countTransactions(g.ctx, t, g.conn, []request{probe}), "the primary holds the probe")

	var advanced, unsent, behind, held bool
	require.NoError(t, g.conn.QueryRow(g.ctx, `SELECT pg_current_wal_insert_lsn()::text, pg_current_wal_insert_lsn() > $1::pg_lsn,
		NOT EXISTS (SELECT 1 FROM pg_stat_replication)`, cut).Scan(&inserted, &advanced, &unsent))
	require.True(t, advanced && unsent, "WAL inserted=%s cut=%s advanced=%t no WAL sender=%t", inserted, cut, advanced, unsent)

	// The order of observations is the proof: the sender left before the cut position, and P was written after it.
	standby, _, err := connectNode(g.ctx, g.env.standby, standbyClusterName, true)
	require.NoError(t, err)

	err = standby.QueryRow(g.ctx, `SELECT coalesce(pg_last_wal_receive_lsn()::text, ''), coalesce(pg_last_wal_receive_lsn() <= $1::pg_lsn, false),
		EXISTS (SELECT FROM "transaction" WHERE id = $2)`, cut, probe.transactionID).Scan(&received, &behind, &held)
	standby.Close(g.ctx)
	require.NoError(t, err)
	t.Logf("standby received=%s cut=%s primary inserted=%s", received, cut, inserted)
	require.True(t, behind, "the standby received WAL past the cut")
	require.False(t, held, "the cut standby holds the probe")

	require.NoError(t, errors.Join(d.Close(g.ctx), p.Close(g.ctx)), "close both writers")

	killAt := time.Now()

	o.killPrimary(g.ctx, t)

	promoteAt := time.Now()
	promoted := o.promote(g.ctx, t, g.env.standby)
	checkAt := time.Now()

	var base [2]int64
	require.NoError(t, promoted.QueryRow(g.ctx, `SELECT (SELECT count(*) FROM base_balance), (SELECT count(*) FROM base_control)`).
		Scan(&base[0], &base[1]))
	assert.Equal(t, [2]int64{balances, controls}, base, "durable baseline read by a session without TEMP copies")
	assert.Equal(t, []string{probe.id.String()}, lostApprovals(g.ctx, t, promoted, slices.Concat(d.calls, p.calls)), "lost approvals")
	assert.Equal(t, map[string]int{string(outcomeApproved): 1}, verifyLedger(g.ctx, t, promoted, d.calls))
	assert.Zero(t, g.broker.publishes.Load(), "obligations stay in the outbox")
	t.Logf("setup+durable=%s cut+position=%s probe+evidence+close=%s kill=%s promote=%s checks=%s", cutAt.Sub(setupAt), probeAt.Sub(cutAt),
		killAt.Sub(probeAt), promoteAt.Sub(killAt), checkAt.Sub(promoteAt), time.Since(checkAt))
}

// ownedTopology is the bound record of one owned project: its directory and its three inspected containers.
type ownedTopology struct {
	dir        string
	containers [len(ownedServices)]dockerContainer
}

// upOwnedTopology is opt-in: unset skips, any other value than the owned project fails, both before Docker or SQL. It
// refuses before any mutation, registers the retaining cleanup, starts only the three services, binds them, and only then
// points the bench DSNs at them for this test. It returns the bound record.
func upOwnedTopology(t *testing.T) *ownedTopology {
	t.Helper()

	switch v := os.Getenv("BENCH_FAILOVER_PROJECT"); v {
	case "":
		t.Skip("set BENCH_FAILOVER_PROJECT=" + failoverProject + " to create, fault and remove the owned promotion topology")
	case failoverProject:
	default:
		t.Fatalf("BENCH_FAILOVER_PROJECT=%q: want %s or unset", v, failoverProject)
	}

	for _, name := range [...]string{"COMPOSE_PROJECT_NAME", "COMPOSE_FILE", "COMPOSE_PROFILES", "DOCKER_HOST", "DOCKER_CONTEXT"} {
		require.Empty(t, os.Getenv(name), "%s set: unexpected environment for the owned project", name)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Leftovers first: a retained failed project also holds the owned ports.
	left, err := ownedResources(ctx)
	require.NoError(t, err)
	require.Empty(t, left, "retained resources of %s: an operator decides their removal after reading the evidence", failoverProject)

	for _, port := range [...]string{ownedPrimaryPort, ownedClientPort, ownedStandbyPort, ownedAPIPort} {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, time.Second)
		if err == nil {
			conn.Close()
		}

		require.ErrorIs(t, err, syscall.ECONNREFUSED, "owned port %s must be free", port)
	}

	dir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, compose(ctx, dir, "config", "-q"))

	o := &ownedTopology{dir: dir}

	t.Cleanup(func() { o.cleanup(t) })

	upAt := time.Now()

	upCtx, cancelUp := context.WithTimeout(t.Context(), 150*time.Second)
	defer cancelUp()

	require.NoError(t, compose(upCtx, dir, "up", "-d", "--wait", "--wait-timeout", "120", "--pull", "never",
		ownedServices[0].service, ownedServices[1].service, ownedServices[2].service))

	bindAt := time.Now()

	bindCtx, cancelBind := context.WithTimeout(t.Context(), time.Minute)
	defer cancelBind()

	require.NoError(t, o.bindDocker(bindCtx, t))
	require.NoError(t, o.bindSQL(bindCtx))

	t.Setenv("BENCH_PG_PRIMARY_DSN", ownedDSN(ownedPrimaryPort))
	t.Setenv("BENCH_PG_CLIENT_PROXY_DSN", ownedDSN(ownedClientPort))
	t.Setenv("BENCH_PG_STANDBY_DSN", ownedDSN(ownedStandbyPort))
	t.Setenv("BENCH_TOXIPROXY_URL", "http://127.0.0.1:"+ownedAPIPort)
	t.Logf("up=%s bind=%s", bindAt.Sub(upAt), time.Since(bindAt))

	return o
}

func ownedDSN(port string) string {
	return "postgresql://bench:bench@127.0.0.1:" + port + "/bench_transaction?sslmode=disable"
}

// bindDocker inspects each owned container once by name and records it; every later read uses its full ID. The owned
// resources must then be exactly these containers, their network and the two data volumes.
func (o *ownedTopology) bindDocker(ctx context.Context, t *testing.T) error {
	t.Helper()

	for i, svc := range ownedServices {
		c, err := inspectContainer(ctx, svc.container)
		if err != nil {
			return err
		}

		if err := c.bound(svc, true); err != nil {
			return err
		}

		s, err := readScope(c.ID)
		if err != nil {
			return err
		}

		if err := s.capped(svc); err != nil {
			return err
		}

		o.containers[i] = c
		t.Logf("owned %s id=%s image=%s pid=%d startedAt=%s labels=%v ports=%+v networks=%+v cgroup=%+v",
			svc.service, c.ID, c.Image, c.State.Pid, c.State.StartedAt, c.Config.Labels, c.NetworkSettings.Ports, c.NetworkSettings.Networks, s)
	}

	return o.sameResources(ctx)
}

// sameResources compares the owned listing with the bound record, IDs included; anything else owned or labelled fails.
func (o *ownedTopology) sameResources(ctx context.Context) error {
	want := []string{
		fmt.Sprintf("volume %[1]s_bench-pg-primary-data %[1]s_bench-pg-primary-data %[1]s", failoverProject),
		fmt.Sprintf("volume %[1]s_bench-pg-standby-data %[1]s_bench-pg-standby-data %[1]s", failoverProject),
	}

	for i, svc := range ownedServices {
		c := o.containers[i]
		want = append(want, fmt.Sprintf("container %s %s %s", c.ID, svc.container, failoverProject),
			fmt.Sprintf("network %s %s %s", c.NetworkSettings.Networks[failoverNetwork].NetworkID, failoverNetwork, failoverProject))
	}

	slices.Sort(want)

	got, err := ownedResources(ctx)
	if err != nil {
		return err
	}

	if want = slices.Compact(want); !slices.Equal(want, got) {
		return fmt.Errorf("owned resources %q, want %q", got, want)
	}

	return nil
}

// bindSQL ties each DSN to its inspected container by the server address Docker reports: the owned primary's IP for the
// primary and the client proxy, the standby's for the standby. verifyPeers checks one system identifier before any DDL.
func (o *ownedTopology) bindSQL(ctx context.Context) error {
	for _, node := range [...]struct {
		port, cluster string
		container     int
		recovery      bool
	}{{ownedPrimaryPort, primaryClusterName, 0, false}, {ownedClientPort, primaryClusterName, 0, false}, {ownedStandbyPort, standbyClusterName, 2, true}} {
		cfg, err := parseBenchDSN(node.port, ownedDSN(node.port))
		if err != nil {
			return err
		}

		conn, _, err := connectNode(ctx, cfg, node.cluster, node.recovery)
		if err != nil {
			return err
		}

		var addr string

		err = conn.QueryRow(ctx, `SELECT host(inet_server_addr())`).Scan(&addr)
		conn.Close(ctx)

		if err != nil {
			return fmt.Errorf("port %s server address: %w", node.port, err)
		}

		if want := o.containers[node.container].NetworkSettings.Networks[failoverNetwork].IPAddress; addr != want {
			return fmt.Errorf("port %s served from %s, want the owned %s", node.port, addr, want)
		}
	}

	return nil
}

// cleanup runs last, after the writer, sessions and leak check. A failed test only records what exists and keeps it. A
// passing one removes the project only if it still matches the bound record, then requires nothing owned is left.
func (o *ownedTopology) cleanup(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if t.Failed() {
		o.record(ctx, t)
		return
	}

	if err := o.unchanged(ctx, t); err != nil {
		t.Errorf("%s retained: %v", failoverProject, err)
		o.record(ctx, t)

		return
	}

	downAt := time.Now()

	if err := compose(ctx, o.dir, "down", "-v", "-t", "10"); err != nil {
		t.Errorf("%s down failed, retained without retry: %v", failoverProject, err)
		return
	}

	left, err := ownedResources(ctx)
	assert.NoError(t, err)
	assert.Empty(t, left, "owned resources after down")
	t.Logf("down=%s", time.Since(downAt))
}

// unchanged checks, by bound ID, the end state every passing round leaves: the primary killed, the proxy live and healthy,
// the promoted standby live, and the owned resources still exactly the record.
func (o *ownedTopology) unchanged(ctx context.Context, t *testing.T) error {
	t.Helper()

	if err := errors.Join(o.exited(ctx), o.live(ctx, t, 1, true), o.live(ctx, t, 2, false)); err != nil {
		return err
	}

	return o.sameResources(ctx)
}

// live re-reads bound container i by ID: still bound, healthy when required, the same start and PID, and its cgroup, which
// it logs, capped without swap or OOM.
func (o *ownedTopology) live(ctx context.Context, t *testing.T, i int, healthy bool) error {
	t.Helper()

	svc, was := ownedServices[i], o.containers[i]

	c, err := inspectContainer(ctx, was.ID)
	if err != nil {
		return err
	}

	if err := c.bound(svc, healthy); err != nil {
		return err
	}

	if c.State.StartedAt != was.State.StartedAt || c.State.Pid != was.State.Pid {
		return fmt.Errorf("%s %s restarted: %s pid %d, bound %s pid %d", svc.service, was.ID, c.State.StartedAt, c.State.Pid,
			was.State.StartedAt, was.State.Pid)
	}

	s, err := readScope(c.ID)
	if err != nil {
		return err
	}

	t.Logf("owned %s cgroup %+v", svc.service, s)

	return s.capped(svc)
}

// exited checks the killed primary by bound ID: the same name, image, labels, host config and start, exited by SIGKILL with
// no PID, OOM kill or restart. Its ports, address and cgroup end with its process and are not read.
func (o *ownedTopology) exited(ctx context.Context) error {
	was := o.containers[0]

	c, err := inspectContainer(ctx, was.ID)
	if err != nil {
		return err
	}

	if c.Name != was.Name || c.Image != was.Image || !maps.Equal(c.Config.Labels, was.Config.Labels) || c.HostConfig != was.HostConfig ||
		c.State.StartedAt != was.State.StartedAt || c.State.Status != "exited" || c.State.ExitCode != 137 || c.State.Pid != 0 ||
		c.State.OOMKilled || c.RestartCount != 0 {
		return fmt.Errorf("%s %s: name %q image %s labels %v host config %+v state %+v restarts %d, want it exited by SIGKILL",
			ownedServices[0].service, was.ID, c.Name, c.Image, c.Config.Labels, c.HostConfig, c.State, c.RestartCount)
	}

	return nil
}

// killPrimary revalidates the bound primary, logging its last cgroup, sends SIGKILL to its full ID and waits until Docker
// reports it exited by that signal. One 30 s budget bounds all of it.
func (o *ownedTopology) killPrimary(ctx context.Context, t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	require.NoError(t, o.live(ctx, t, 0, true), "the primary immediately before the kill")

	_, err := docker(ctx, "kill", "--signal", "KILL", o.containers[0].ID)
	require.NoError(t, err)

	for {
		if err = o.exited(ctx); err == nil {
			return
		}

		require.NoError(t, backoff.WaitContext(ctx, 100*time.Millisecond), "%v", err)
	}
}

// promote revalidates the bound standby, promotes it within 90 s and returns a fresh connection proving the promoted role:
// the same system identifier, out of recovery at synchronous_commit=on, standby1 still the synchronous name and no WAL
// sender. No new approval may be accepted there until a synchronous standby streams again.
func (o *ownedTopology) promote(ctx context.Context, t *testing.T, cfg *pgx.ConnConfig) *pgx.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	require.NoError(t, o.live(ctx, t, 2, false), "the standby immediately before promotion")

	standby, systemID, err := connectNode(ctx, cfg, standbyClusterName, true)
	require.NoError(t, err)

	var promoted, unsent bool

	err = standby.QueryRow(ctx, `SELECT pg_promote(true, 60)`).Scan(&promoted)
	standby.Close(ctx)
	require.NoError(t, err)
	require.True(t, promoted, "pg_promote within 60 s")

	conn, promotedID, err := connectNode(ctx, cfg, standbyClusterName, false)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()

		conn.Close(closeCtx)
	})

	var names string
	require.NoError(t, conn.QueryRow(ctx, `SELECT current_setting('synchronous_standby_names'),
		NOT EXISTS (SELECT 1 FROM pg_stat_replication)`).Scan(&names, &unsent))
	require.Equal(t, systemID, promotedID, "system identifier")
	require.Equal(t, "FIRST 1 ("+standbyName+")", names, "synchronous_standby_names")
	require.True(t, unsent, "no WAL sender on the promoted node")

	return conn
}

// requireCommitSetting returns an AfterConnect hook that logs a writer session's synchronous_commit and refuses any value
// other than want.
func requireCommitSetting(t *testing.T, want string) func(context.Context, *pgconn.PgConn) error {
	t.Helper()

	return func(ctx context.Context, c *pgconn.PgConn) error {
		res, err := c.Exec(ctx, `SHOW synchronous_commit`).ReadAll()
		if err != nil {
			return err
		}

		setting := string(res[0].Rows[0][0])
		t.Logf("writer session %s pid %d: synchronous_commit=%s", c.ParameterStatus("application_name"), c.PID(), setting)

		if setting != want {
			return fmt.Errorf("writer session synchronous_commit %q, want %s", setting, want)
		}

		return nil
	}
}

// record logs, best effort, every owned resource, the full inspect of each owned container and its cgroup. It removes
// nothing: an operator decides on retained evidence.
func (o *ownedTopology) record(ctx context.Context, t *testing.T) {
	t.Helper()

	rows, err := ownedResources(ctx)
	t.Logf("%s retained: resources %q, listing error %v", failoverProject, rows, err)

	for _, row := range rows {
		var kind, id string
		if _, err := fmt.Sscan(row, &kind, &id); err != nil || kind != "container" {
			continue
		}

		out, err := docker(ctx, "inspect", "--type", "container", id)
		s, scopeErr := readScope(id)
		t.Logf("container %s inspect %s (error %v) cgroup %+v (error %v)", id, out, err, s, scopeErr)
	}
}
