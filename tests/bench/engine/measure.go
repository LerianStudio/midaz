//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"runtime/metrics"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
)

// snapshotTimeout bounds each settings or boundary read on the verification connection.
const snapshotTimeout = 10 * time.Second

// pgSettingNames identify the run's durability, WAL timing and checkpoint configuration.
var pgSettingNames = []string{
	"server_version_num", "synchronous_commit", "synchronous_standby_names", "wal_sync_method", "full_page_writes",
	"wal_buffers", "track_wal_io_timing", "checkpoint_timeout", "checkpoint_completion_target", "max_wal_size",
}

// walStats and checkpointStats are PG17 shared counters with a non-NULL stats_reset identity.
type walStats struct {
	records, fpi, bytes, buffersFull, syncs int64
	syncMS                                  float64
	reset                                   time.Time
}

type checkpointStats struct {
	timed, requested, buffersWritten int64
	reset                            time.Time
}

// boundary is one phase boundary: PG positions and counters from one statement, then this process's cumulative counters.
type boundary struct {
	at                   time.Time // PG clock_timestamp()
	written, inserted    int64     // pg_current_wal_lsn (the write position) and the insert position, bytes from 0/0
	wal                  walStats
	ckpt                 checkpointStats
	ckptWait             string         // checkpointer wait_event, or running
	replication          *string        // standby1 state/sync_state; NULL when absent
	flushLag             *time.Duration // standby1's last-sample LSN acknowledgement lag; NULL without a recent sample
	gcCycles, heapAllocs uint64         // runtime/metrics, cumulative
	userCPU, sysCPU      time.Duration  // getrusage(RUSAGE_SELF), cumulative
	maxRSSKiB            int64          // peak RSS over the process lifetime, KiB on Linux
}

// pgSettings reads the once-per-run settings record; current_setting fails on a name the server lacks.
func pgSettings(ctx context.Context, conn *pgx.Conn) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()

	var line string
	if err := conn.QueryRow(ctx, `SELECT string_agg(n || '=' || current_setting(n), ' ' ORDER BY i)
		FROM unnest($1::text[]) WITH ORDINALITY AS s(n, i)`, pgSettingNames).Scan(&line); err != nil {
		return "", fmt.Errorf("read PG settings: %w", err)
	}

	return line, nil
}

// readBoundary must run on the verification connection, never a writer's, with nothing else using it.
func readBoundary(ctx context.Context, conn *pgx.Conn) (boundary, error) {
	ctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()

	var b boundary
	if err := conn.QueryRow(ctx, `SELECT clock_timestamp(), (pg_current_wal_lsn() - '0/0')::int8,
			(pg_current_wal_insert_lsn() - '0/0')::int8, w.wal_records, w.wal_fpi, w.wal_bytes::int8, w.wal_buffers_full,
			w.wal_sync, w.wal_sync_time, w.stats_reset, c.num_timed, c.num_requested, c.buffers_written, c.stats_reset,
			(SELECT coalesce(wait_event, 'running') FROM pg_stat_activity WHERE backend_type = 'checkpointer'), r.state || '/' || r.sync_state, r.flush_lag
		FROM pg_stat_wal w CROSS JOIN pg_stat_checkpointer c LEFT JOIN pg_stat_replication r ON r.application_name = $1`,
		standbyName).Scan(&b.at, &b.written, &b.inserted, &b.wal.records, &b.wal.fpi, &b.wal.bytes, &b.wal.buffersFull,
		&b.wal.syncs, &b.wal.syncMS, &b.wal.reset, &b.ckpt.timed, &b.ckpt.requested, &b.ckpt.buffersWritten, &b.ckpt.reset,
		&b.ckptWait, &b.replication, &b.flushLag); err != nil {
		return b, fmt.Errorf("read PG boundary: %w", err)
	}

	samples := []metrics.Sample{{Name: "/gc/cycles/total:gc-cycles"}, {Name: "/gc/heap/allocs:bytes"}}
	metrics.Read(samples)

	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return b, fmt.Errorf("getrusage: %w", err)
	}

	b.gcCycles, b.heapAllocs = samples[0].Value.Uint64(), samples[1].Value.Uint64()
	b.userCPU, b.sysCPU, b.maxRSSKiB = time.Duration(ru.Utime.Nano()), time.Duration(ru.Stime.Nano()), ru.Maxrss

	return b, nil
}

// until reports a to b. Written WAL and PG counters include drain, background work and lazily flushed backend stats.
// A view's reset leaves only its counters unmeasured; standby1 not streaming sync at either end invalidates the cell.
func (a boundary) until(b boundary) (string, error) {
	var s strings.Builder

	fmt.Fprintf(&s, "interval=%s walWritten=%dB insertAheadOfWrite@end=%dB", b.at.Sub(a.at), b.written-a.written, b.inserted-b.written)

	if w, v := a.wal, b.wal; w.reset.Equal(v.reset) {
		fmt.Fprintf(&s, " pg_stat_wal records=%d fpi=%d bytes=%d buffersFull=%d syncs=%d syncTime=%.3fms",
			v.records-w.records, v.fpi-w.fpi, v.bytes-w.bytes, v.buffersFull-w.buffersFull, v.syncs-w.syncs, v.syncMS-w.syncMS)
	} else {
		s.WriteString(" pg_stat_wal=unmeasured(reset)")
	}

	c, d := a.ckpt, b.ckpt
	counted := c.reset.Equal(d.reset)

	if counted {
		fmt.Fprintf(&s, " checkpointer timed=%d requested=%d buffersWritten=%d",
			d.timed-c.timed, d.requested-c.requested, d.buffersWritten-c.buffersWritten)
	} else {
		s.WriteString(" checkpointer=unmeasured(reset)")
	}

	// Absence of every signal is not proof that no checkpoint overlapped.
	activity := "unobserved"
	if a.ckptWait != "CheckpointerMain" || b.ckptWait != "CheckpointerMain" ||
		counted && (d.timed > c.timed || d.requested > c.requested || d.buffersWritten > c.buffersWritten) {
		activity = "observed"
	}

	lag := "unmeasured(NULL)"
	if b.flushLag != nil {
		lag = b.flushLag.String()
	}

	fmt.Fprintf(&s, " checkpointWait(start,end)=%s,%s checkpointerActivity=%s flushLag@end=%s replication(start,end)=%s,%s",
		a.ckptWait, b.ckptWait, activity, lag, orNone(a.replication), orNone(b.replication))
	fmt.Fprintf(&s, " process gcCycles=%d heapAllocs=%dB userCPU=%s sysCPU=%s maxRSS(lifetime)=%dKiB",
		b.gcCycles-a.gcCycles, b.heapAllocs-a.heapAllocs, b.userCPU-a.userCPU, b.sysCPU-a.sysCPU, b.maxRSSKiB)

	if orNone(a.replication) != "streaming/sync" || orNone(b.replication) != "streaming/sync" {
		return s.String(), fmt.Errorf("%s replication %s at start, %s at end, want streaming/sync",
			standbyName, orNone(a.replication), orNone(b.replication))
	}

	return s.String(), nil
}

func orNone(s *string) string {
	if s == nil {
		return "none"
	}

	return *s
}
