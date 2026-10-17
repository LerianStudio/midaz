//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"net"
	"os"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Opt-in: builds the owned promotion topology, binds every endpoint to its containers, then rebuilds and seeds it through
// the gate and approves one transfer that a fresh connection checks against a durable baseline. No kill or promotion yet.
func TestPromotionTopology(t *testing.T) {
	switch v := os.Getenv("BENCH_FAILOVER_PROJECT"); v {
	case "":
		t.Skip("set BENCH_FAILOVER_PROJECT=" + failoverProject + " to create, check and remove the owned promotion topology")
	case failoverProject:
	default:
		t.Fatalf("BENCH_FAILOVER_PROJECT=%q: want %s or unset", v, failoverProject)
	}

	upOwnedTopology(t)

	setupAt := time.Now()
	g := newGate(t, false)
	balances, controls := assertSeeded(g.ctx, t, g.conn, 4)
	w, _ := g.start(t, "bench-promotion", 1, gateMaxBatch, 1)

	// One implicit transaction, committed through standby1 under the guarded synchronous_commit=on.
	_, err := g.conn.Exec(g.ctx, `CREATE TABLE public.base_balance AS TABLE pg_temp.base_balance;
		CREATE TABLE public.base_control AS TABLE pg_temp.base_control`)
	require.NoError(t, err)

	writeAt := time.Now()
	req := batchReq(t, "promotion/topology", "promotion/topology", g.clock.now(), ordinaryOf(0)(-5), ordinaryOf(1)(5))
	require.Equal(t, []string{"approved+tx"}, summarize(t, []request{req}, submitEach(g.ctx, t, w, []request{req}).wait(t)))
	require.NoError(t, w.Close(g.ctx))

	checkAt := time.Now()
	fresh, err := pgx.ConnectConfig(g.ctx, g.env.primary)
	require.NoError(t, err)

	defer fresh.Close(g.ctx) // the gate context outlives the test body

	var base [2]int64
	require.NoError(t, fresh.QueryRow(g.ctx, `SELECT (SELECT count(*) FROM base_balance), (SELECT count(*) FROM base_control)`).
		Scan(&base[0], &base[1]))
	assert.Equal(t, [2]int64{balances, controls}, base, "durable baseline read by a session without TEMP copies")
	assert.Equal(t, map[string]int{string(outcomeApproved): 1}, verifyLedger(g.ctx, t, fresh, w.calls))
	assert.Zero(t, g.broker.publishes.Load(), "obligations stay in the outbox")
	t.Logf("gate=%s writer=%s checker=%s", writeAt.Sub(setupAt), checkAt.Sub(writeAt), time.Since(checkAt))
}

// ownedTopology is the bound record of one owned project: its directory and its three inspected containers.
type ownedTopology struct {
	dir        string
	containers [len(ownedServices)]dockerContainer
}

// upOwnedTopology refuses before any mutation, registers the retaining cleanup, starts only the three services, binds
// them, and only then points the bench DSNs at them for this test.
func upOwnedTopology(t *testing.T) {
	t.Helper()

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

		if err := c.bound(svc); err != nil {
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

// unchanged re-reads each bound ID: the same start and PID, still bound, capped and free of OOM and swap, and the owned
// resources are still exactly the record.
func (o *ownedTopology) unchanged(ctx context.Context, t *testing.T) error {
	t.Helper()

	for i, svc := range ownedServices {
		was := o.containers[i]

		c, err := inspectContainer(ctx, was.ID)
		if err != nil {
			return err
		}

		if err := c.bound(svc); err != nil {
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

		t.Logf("owned %s cgroup at pass %+v", svc.service, s)

		if err := s.capped(svc); err != nil {
			return err
		}
	}

	return o.sameResources(ctx)
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
