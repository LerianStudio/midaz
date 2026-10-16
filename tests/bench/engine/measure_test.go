//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deltas run from the earlier boundary to the later. A view's reset leaves only its own counters unmeasured; standby1
// not streaming sync at either boundary invalidates the interval.
func TestBoundaryUntil(t *testing.T) {
	at := time.Date(2026, 10, 16, 21, 0, 0, 0, time.UTC)
	reset, later := at.Add(-time.Hour), at.Add(time.Second)
	streaming, potential, writeDelay, lag := "streaming/sync", "streaming/potential", "CheckpointWriteDelay", 1500*time.Microsecond

	start := boundary{
		at: at, written: 1000, inserted: 1100, replication: &streaming, ckptWait: "CheckpointerMain",
		wal:      walStats{records: 10, fpi: 1, bytes: 900, buffersFull: 2, syncs: 3, syncMS: 1.5, reset: reset},
		ckpt:     checkpointStats{timed: 74, requested: 9, buffersWritten: 500, reset: reset},
		gcCycles: 5, heapAllocs: 1 << 20, userCPU: time.Second, sysCPU: 100 * time.Millisecond, maxRSSKiB: 1000,
	}
	end := start
	end.at, end.written, end.inserted, end.flushLag = at.Add(20*time.Second), 5000, 5200, &lag
	end.wal = walStats{records: 40, fpi: 4, bytes: 4900, buffersFull: 2, syncs: 13, syncMS: 4, reset: reset}
	end.gcCycles, end.heapAllocs, end.userCPU, end.sysCPU, end.maxRSSKiB = 8, 3<<20, 3*time.Second, 300*time.Millisecond, 4000

	measured := []string{
		"interval=20s walWritten=4000B insertAheadOfWrite@end=200B",
		"pg_stat_wal records=30 fpi=3 bytes=4000 buffersFull=0 syncs=10 syncTime=2.500ms",
		"checkpointer timed=0 requested=0 buffersWritten=0",
		"flushLag@end=1.5ms",
		"process gcCycles=3 heapAllocs=2097152B userCPU=2s sysCPU=200ms maxRSS(lifetime)=4000KiB",
	}

	for name, tc := range map[string]struct {
		edit         func(a, b *boundary)
		want, absent []string
		invalid      bool
	}{
		"measured":      {edit: func(_, _ *boundary) {}, want: append(measured, "checkpointerActivity=unobserved")},
		"wal reset":     {edit: func(_, b *boundary) { b.wal.reset = later }, want: []string{measured[0], "pg_stat_wal=unmeasured(reset)", measured[2]}, absent: []string{"records="}},
		"ckpt reset":    {edit: func(_, b *boundary) { b.ckpt.reset, b.ckpt.buffersWritten = later, 9 }, want: []string{measured[1], "checkpointer=unmeasured(reset)", "checkpointerActivity=unobserved"}},
		"ckpt counters": {edit: func(_, b *boundary) { b.ckpt.buffersWritten = 900 }, want: []string{"buffersWritten=400", "checkpointerActivity=observed"}},
		"ckpt wait":     {edit: func(a, _ *boundary) { a.ckptWait = writeDelay }, want: []string{"checkpointWait(start,end)=CheckpointWriteDelay,CheckpointerMain", "checkpointerActivity=observed"}},
		"ckpt running":  {edit: func(_, b *boundary) { b.ckptWait = "running" }, want: []string{"checkpointWait(start,end)=CheckpointerMain,running", "checkpointerActivity=observed"}},
		"no lag sample": {edit: func(_, b *boundary) { b.flushLag = nil }, want: []string{"flushLag@end=unmeasured(NULL)"}},
		"standby gone":  {edit: func(a, _ *boundary) { a.replication = nil }, want: measured, invalid: true},
		"standby async": {edit: func(_, b *boundary) { b.replication = &potential }, want: measured, invalid: true},
	} {
		t.Run(name, func(t *testing.T) {
			a, b := start, end
			tc.edit(&a, &b)

			line, err := a.until(b)
			assert.Equal(t, tc.invalid, err != nil, "invalid: %v", err)

			for _, want := range tc.want {
				assert.Contains(t, line, want)
			}

			for _, absent := range tc.absent {
				assert.NotContains(t, line, absent)
			}
		})
	}
}

// Two boundary reads on a live connection scan PG17's real column and NULL types around a streaming sync standby1.
func TestReadBoundary(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	conn := connectPrimary(ctx, t, env)

	settings, err := pgSettings(ctx, conn)
	require.NoError(t, err)
	t.Log(settings)
	assert.Regexp(t, `^server_version_num=17\d{4} `, settings, "the boundary query reads PG17 views")

	start, err := readBoundary(ctx, conn)
	require.NoError(t, err)

	end, err := readBoundary(ctx, conn)
	require.NoError(t, err)

	line, err := start.until(end)
	require.NoError(t, err)
	t.Log(line)
	assert.True(t, end.at.After(start.at))
	assert.Positive(t, start.written)
	assert.GreaterOrEqual(t, end.written, start.written)
	assert.GreaterOrEqual(t, end.inserted, end.written, "the insert position is never behind the write position")
	assert.Positive(t, end.heapAllocs)
	assert.Positive(t, end.maxRSSKiB)
}
