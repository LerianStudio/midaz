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

func TestOwnerlessAsyncLossDetected(t *testing.T) { asyncLoss(t, false) }

// Owner D closes and its lease expires on the database clock before owner P takes over past the cut.
func TestOwnerAsyncLossDetected(t *testing.T) { asyncLoss(t, true) }

// Opt-in negative control on the owned topology. D's writer commits D through standby1 under synchronous_commit=on and
// closes; with replication cut, P's writer starts and commits P under local on the primary alone. The primary is killed
// and the standby promoted: a fresh session there finds the durable baseline and D in full, and exactly P lost.
func asyncLoss(t *testing.T, owner bool) {
	t.Helper()

	o := upOwnedTopology(t)

	g := newGate(t, owner)
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
	require.NoError(t, d.Close(g.ctx), "close the durable writer")
	awaitPrimary(g.ctx, t, g.conn, "no live lease before P's writer", fenceExpired, benchTenantID)

	local := *g.env
	local.primary = g.env.primary.Copy()
	local.primary.RuntimeParams["synchronous_commit"] = "local"
	local.primary.AfterConnect = requireCommitSetting(t, "local")

	// No restore is registered: a pass ends with this primary killed, and a failure keeps the cut as evidence.
	require.NoError(t, quietProxy(t, g.env, replProxyName, ":5433").Disable(), "disable %s through the toxiproxy API", replProxyName)
	awaitPrimary(g.ctx, t, g.conn, "the standby's WAL sender is gone", noWalSender)

	var cut, inserted, received string
	require.NoError(t, g.conn.QueryRow(g.ctx, `SELECT pg_current_wal_insert_lsn()::text`).Scan(&cut))

	p, _ := g.startOn(t, &local, "bench-async-loss", 1, gateMaxBatch, 1)
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

	require.NoError(t, p.Close(g.ctx), "close the probe writer")

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
	t.Logf("kill=%s promote=%s checks=%s", promoteAt.Sub(killAt), checkAt.Sub(promoteAt), time.Since(checkAt))
}

const promotionApp = "bench-promotion" // the gate names the killed primary's writer session promotionApp-0

// Without a synchronous standby the promoted node commits W and D's replay only locally and answers both unknown. With
// the replacement streaming, past every cutoff, U resolves expired, W and D replay, and a new transfer N approves.
func TestOwnerlessPromotion(t *testing.T) {
	o, g, old, durable, lost := losePrimary(t, false)
	app := promotionApp + "-next-0"
	window := batchReq(t, "promotion/window", "promotion/window", g.clock.now(), ordinaryOf(1)(-5), ordinaryOf(3)(5))
	touches := touchOf(g.ctx, t, g.conn, durable)
	w, inner := g.startOn(t, g.env, promotionApp+"-next", 1, gateMaxBatch, 6)

	for _, req := range []request{window, durable} {
		answering := submitEach(g.ctx, t, w, []request{req})
		awaitBackend(g.ctx, t, g.conn, app, "SyncRep")
		assertUnknown(t, answering.wait(t), commitUnknown)
		require.False(t, t.Failed(), "retain the failed window proof before another write")
		awaitPrimary(g.ctx, t, g.conn, "pgx cancelled the wait and closed its connection", noBackend, app)
	}

	require.Equal(t, 1, countTransactions(g.ctx, t, g.conn, []request{window}), "the cancelled wait committed W locally")
	require.Equal(t, touches+1, touchOf(g.ctx, t, g.conn, durable), "the waiting replay committed only its touch")

	o.replace(g.ctx, t, g.conn)
	assert.Equal(t, []string{"expired", "replay/approved+tx", "replay/approved+tx", "approved+tx"},
		resolveAfter(t, g, w, lost, window, durable))
	assert.Equal(t, touches+2, touchOf(g.ctx, t, g.conn, durable))
	require.ErrorIs(t, w.Close(g.ctx), context.DeadlineExceeded, "Close reports W's unknown COMMIT")
	assert.Equal(t, 6, inner.workers[0].passes, "one pass per batch: nothing retried an unknown COMMIT")
	assert.Empty(t, lostApprovals(g.ctx, t, g.conn, slices.Concat(old, w.calls)), "lost approvals")
	assert.Equal(t, map[string]int{"approved": 2, "unknown": 3, "expired": 1, "replay/approved": 2, unansweredApproval: 1},
		verifyLedger(g.ctx, t, g.conn, slices.Concat(old, w.calls)))
	assert.Zero(t, g.broker.publishes.Load(), "obligations stay in the outbox")
}

// A takeover on the promoted node alone commits locally and returns no owner. With the replacement streaming and that
// lease expired, owner-c takes epoch 3 and reloads: U resolves expired, D replays and N approves on D's after-state.
func TestOwnerPromotion(t *testing.T) {
	o, g, old, durable, lost := losePrimary(t, true)

	const abandonedApp, successorApp = promotionApp + "-abandoned", promotionApp + "-successor"

	awaitPrimary(g.ctx, t, g.conn, "the lost owner's last replicated lease expires", fenceExpired, benchTenantID)

	acquiring, cancelAcquire := context.WithTimeout(g.ctx, batchBudget)
	defer cancelAcquire()

	abandoned := startOwner(acquiring, t, namedBatcher(t, g.b, g.env, abandonedApp, nil), "owner-b", gateMaxBatch)
	awaitBackend(g.ctx, t, g.conn, abandonedApp, "SyncRep")
	require.False(t, abandoned.returned(), "no ownership while its takeover COMMIT waits for a standby")
	cancelAcquire()

	none, err := abandoned.wait(t)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorContains(t, err, "acquire fence: "+commitUnknown)
	require.Nil(t, none, "a takeover of unknown outcome returns no owner")
	awaitPrimary(g.ctx, t, g.conn, "pgx cancelled the takeover's wait and closed its connection", noBackend, abandonedApp)

	tok, _ := readFence(g.ctx, t, g.conn)
	require.Equal(t, ownerToken{owner: "owner-b", epoch: 2}, tok, "the abandoned takeover committed locally")

	o.replace(g.ctx, t, g.conn)
	awaitPrimary(g.ctx, t, g.conn, "the abandoned takeover's lease expires", fenceExpired, benchTenantID)

	innerC, err := startOwner(g.ctx, t, namedBatcher(t, g.b, g.env, successorApp, nil), "owner-c", gateMaxBatch).wait(t)
	require.NoError(t, err)

	tok, _ = readFence(g.ctx, t, g.conn)
	require.Equal(t, ownerToken{owner: "owner-c", epoch: 3}, tok, "a new successor, not the abandoned takeover, owns the fence")

	w := captureCalls(innerC, 3)
	assert.Equal(t, []string{"expired", "replay/approved+tx", "approved+tx"}, resolveAfter(t, g, w, lost, durable))
	require.NoError(t, w.Close(g.ctx))
	assert.Empty(t, lostApprovals(g.ctx, t, g.conn, slices.Concat(old, w.calls)), "lost approvals")
	assert.Equal(t, map[string]int{"approved": 2, "unknown": 1, "expired": 1, "replay/approved": 1},
		verifyLedger(g.ctx, t, g.conn, slices.Concat(old, w.calls)))
	assert.Zero(t, g.broker.publishes.Load(), "obligations stay in the outbox")
}

// losePrimary is the positive prologue on a fresh owned topology. D commits through standby1 under on; U's COMMIT waits
// in SyncRep with replication cut when the primary is killed, and its live pass, not its budget, ends unknown. A fresh
// session on the promoted node finds the baseline and D, never U. It returns the topology, round, old calls, D and U.
func losePrimary(t *testing.T, owner bool) (*ownedTopology, *gate, []call, request, request) {
	t.Helper()

	o := upOwnedTopology(t)
	g := newGate(t, owner)
	balances, controls := assertSeeded(g.ctx, t, g.conn, 4)

	passes := make(passContexts, 1)
	synced, sg := *g.env, *g
	synced.primary = g.env.primary.Copy()
	synced.primary.AfterConnect = requireCommitSetting(t, "on")
	synced.sessionTracer = func() pgx.QueryTracer { return passes } // the old writer's session only
	sg.env = &synced
	w, inner := sg.start(t, promotionApp, 1, gateMaxBatch, 2)
	app := promotionApp + "-0"

	// One implicit transaction, committed through standby1 under the guarded synchronous_commit=on.
	_, err := g.conn.Exec(g.ctx, `CREATE TABLE public.base_balance AS TABLE pg_temp.base_balance;
		CREATE TABLE public.base_control AS TABLE pg_temp.base_control`)
	require.NoError(t, err)

	durable := batchReq(t, "promotion/durable", "promotion/durable", g.clock.now(), ordinaryOf(0)(-5), ordinaryOf(1)(5))
	require.Equal(t, []string{"approved+tx"}, summarize(t, []request{durable}, []result{w.Submit(g.ctx, durable)}))
	<-passes // D's finalize filled it, and its pass has ended

	lost := batchReq(t, "promotion/lost", "promotion/lost", g.clock.now(), ordinaryOf(1)(-5), ordinaryOf(2)(5))
	outbox := holdLock(g.ctx, t, g.env, `LOCK TABLE outbox_events IN SHARE MODE`)
	paying := submitEach(g.ctx, t, w, []request{lost})

	awaitBackend(g.ctx, t, g.conn, app, "relation") // inside its pass, so an owner's idle renewals never meet the cut
	// No restore is registered: a pass ends with this primary killed, and a failure keeps the cut as evidence.
	require.NoError(t, quietProxy(t, g.env, replProxyName, ":5433").Disable(), "disable %s through the toxiproxy API", replProxyName)
	awaitPrimary(g.ctx, t, g.conn, "the standby's WAL sender is gone", noWalSender)
	require.NoError(t, outbox.Rollback(g.ctx))
	awaitBackend(g.ctx, t, g.conn, app, "SyncRep") // U's commit record now exists only on this primary
	require.Len(t, passes, 1, "U's waiting pass finalized its claim")

	pass := <-passes
	budget, _ := pass.Deadline()
	killAt := time.Now()

	require.NoError(t, pass.Err(), "U's pass is live when the primary is killed")
	o.killPrimary(g.ctx, t)
	assertUnknown(t, paying.wait(t), commitUnknown)
	require.False(t, t.Failed(), "retain the failed unknown proof before promotion")
	require.ErrorIs(t, pass.Err(), context.Canceled, "U's pass returned before its budget: the kill, not a deadline, ended its wait")
	t.Logf("U's budget before kill revalidation=%s revalidation to answer=%s", budget.Sub(killAt), time.Since(killAt))
	require.ErrorContains(t, w.Close(g.ctx), commitUnknown, "Close reports U's unknown COMMIT")
	require.Equal(t, 2, inner.workers[0].passes, "one pass per batch: nothing retried the unknown COMMIT")

	promoted := o.promote(g.ctx, t, g.env.standby)

	var base [2]int64
	require.NoError(t, promoted.QueryRow(g.ctx, `SELECT (SELECT count(*) FROM base_balance), (SELECT count(*) FROM base_control)`).
		Scan(&base[0], &base[1]))
	require.Equal(t, [2]int64{balances, controls}, base, "durable baseline read by a session without TEMP copies")
	require.Empty(t, lostApprovals(g.ctx, t, promoted, w.calls), "lost approvals")
	require.Zero(t, countTransactions(g.ctx, t, promoted, []request{lost}), "U's local commit died with the primary")
	require.Equal(t, map[string]int{"approved": 1, "unknown": 1}, verifyLedger(g.ctx, t, promoted, w.calls))
	require.False(t, t.Failed(), "retain the failed ledger proof before another write")

	env, pg := *g.env, *g // untraced: later writers dial the promoted node under on
	env.primary = g.env.standby.Copy()
	env.primary.AfterConnect = requireCommitSetting(t, "on")
	pg.env, pg.conn = &env, promoted

	return o, &pg, w.calls, durable, lost
}

// resolveAfter moves the request clock past every original cutoff, submits reqs one by one under fresh response
// deadlines, then a new transfer N that debits balance 1, which D and U both moved. It summarizes their answers.
func resolveAfter(t *testing.T, g *gate, w *captureWriter, reqs ...request) []string {
	t.Helper()
	g.clock.advance(time.Minute)

	reqs = append(reqs, batchReq(t, "promotion/next", "promotion/next", g.clock.now(), ordinaryOf(1)(-5), ordinaryOf(0)(5)))
	res := make([]result, len(reqs))

	for i := range reqs {
		reqs[i].deadline = g.clock.now().Add(usualBudget.caller)
		res[i] = w.Submit(g.ctx, reqs[i])
	}

	return summarize(t, reqs, res)
}

// ownedTopology is the bound record of one owned project: its directory and its inspected containers, the replacement's
// only once started.
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

	for _, port := range [...]string{ownedPrimaryPort, ownedClientPort, ownedStandbyPort, ownedReplacementPort, ownedAPIPort} {
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

// bindDocker inspects each started container once by name and binds it; every later read uses its full ID. The owned
// resources must then be exactly these containers, their network and their data volumes.
func (o *ownedTopology) bindDocker(ctx context.Context, t *testing.T) error {
	t.Helper()

	for i := range 3 {
		c, err := inspectContainer(ctx, ownedServices[i].container)
		if err != nil {
			return err
		}

		if err := o.bind(t, i, c); err != nil {
			return err
		}
	}

	return o.sameResources(ctx)
}

// bind records inspected container c as owned slot i once it is bound, healthy and capped, and logs what it bound.
func (o *ownedTopology) bind(t *testing.T, i int, c dockerContainer) error {
	t.Helper()

	svc := ownedServices[i]

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

	return nil
}

// sameResources compares the owned listing with the bound record, IDs included; anything else owned or labelled fails.
// A slot never started, the replacement before a promotion, owes no container and no volume.
func (o *ownedTopology) sameResources(ctx context.Context) error {
	var want []string

	for i, svc := range ownedServices {
		c := o.containers[i]
		if c.ID == "" {
			continue
		}

		want = append(want, fmt.Sprintf("container %s %s %s", c.ID, svc.container, failoverProject),
			fmt.Sprintf("network %s %s %s", c.NetworkSettings.Networks[failoverNetwork].NetworkID, failoverNetwork, failoverProject))

		if svc.volume != "" {
			want = append(want, fmt.Sprintf("volume %[1]s_%[2]s %[1]s_%[2]s %[1]s", failoverProject, svc.volume))
		}
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
		conn, err := o.dialBound(ctx, node.port, node.cluster, node.container, node.recovery)
		if err != nil {
			return err
		}

		conn.Close(ctx)
	}

	return nil
}

// dialBound connects to an owned port, checks the node's identity and that it serves from bound container i's IP.
func (o *ownedTopology) dialBound(ctx context.Context, port, cluster string, i int, recovery bool) (*pgx.Conn, error) {
	cfg, err := parseBenchDSN(port, ownedDSN(port))
	if err != nil {
		return nil, err
	}

	conn, _, err := connectNode(ctx, cfg, cluster, recovery)
	if err != nil {
		return nil, err
	}

	var addr string

	err = conn.QueryRow(ctx, `SELECT host(inet_server_addr())`).Scan(&addr)
	want := o.containers[i].NetworkSettings.Networks[failoverNetwork].IPAddress

	switch {
	case err != nil:
		err = fmt.Errorf("port %s server address: %w", port, err)
	case addr != want:
		err = fmt.Errorf("port %s served from %s, want the owned %s", port, addr, want)
	default:
		return conn, nil
	}

	conn.Close(ctx)

	return nil, err
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
// the promoted standby live, a started replacement live and healthy, and the owned resources still exactly the record.
func (o *ownedTopology) unchanged(ctx context.Context, t *testing.T) error {
	t.Helper()

	errs := []error{o.exited(ctx), o.live(ctx, t, 1, true), o.live(ctx, t, 2, false)}

	if o.containers[3].ID != "" {
		errs = append(errs, o.live(ctx, t, 3, true))
	}

	if err := errors.Join(errs...); err != nil {
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

// replace starts only the replacement, a new standby1 copied from the promoted node, while no COMMIT waits for that name
// and the old primary stays exited whatever up returns. It binds the replacement by full ID within 120 s, then proves it
// the promoted node's only synchronous standby, streaming from the replacement's own address.
func (o *ownedTopology) replace(ctx context.Context, t *testing.T, promoted *pgx.Conn) {
	t.Helper()

	var waiters int
	require.NoError(t, promoted.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE wait_event = 'SyncRep'`).Scan(&waiters))
	require.Zero(t, waiters, "commits waiting for standby1 before its name streams again")
	require.NoError(t, errors.Join(o.exited(ctx), o.live(ctx, t, 2, false)), "the topology before the replacement starts")

	svc, startAt := ownedServices[3], time.Now()

	startCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	upErr := compose(startCtx, o.dir, "up", "-d", "--no-deps", "--pull", "never", svc.service)
	require.NoError(t, errors.Join(upErr, o.exited(ctx)), "up of the replacement, and the old primary still exited")

	c, err := inspectContainer(startCtx, svc.container)
	require.NoError(t, err)

	for {
		if err = c.bound(svc, true); err == nil {
			break
		}

		require.NoError(t, backoff.WaitContext(startCtx, time.Second), "%v", err)

		c, err = inspectContainer(startCtx, c.ID)
		require.NoError(t, err)
	}

	require.NoError(t, o.bind(t, 3, c))
	require.NoError(t, o.sameResources(ctx))

	repl, err := o.dialBound(ctx, ownedReplacementPort, replacementClusterName, 3, true)
	require.NoError(t, err)

	defer repl.Close(ctx)

	require.NoError(t, (&verifiedStack{primary: promoted, standby: repl}).awaitStandby(ctx))

	var sender string
	require.NoError(t, promoted.QueryRow(ctx, `SELECT host(client_addr) FROM pg_stat_replication`).Scan(&sender))
	require.Equal(t, c.NetworkSettings.Networks[failoverNetwork].IPAddress, sender, "standby1 streams from the replacement")
	t.Logf("replacement started, bound and streaming in %s", time.Since(startAt))
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
