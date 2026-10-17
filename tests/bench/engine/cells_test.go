//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cell is one closed-loop configuration. Its workload label fixes each index's postings, so the cells of one shape
// send the same draws; transaction IDs and clocks stay per run. DelayMS is the downstream pg-repl toxic, never an RTT.
type cell struct {
	Name        string        `json:"name"`
	Writer      string        `json:"writer"`
	Workers     int           `json:"workers"`
	Shape       string        `json:"shape"`
	DelayMS     int           `json:"downstreamDelayMS"`
	Ordinary    int           `json:"ordinary"`
	Producers   int           `json:"closedProducers"`
	MaxBatch    int           `json:"maxBatch"`
	Warmup      time.Duration `json:"warmupNS"`
	Measured    time.Duration `json:"measuredNS"`
	WarmupCap   int64         `json:"warmupCap"`
	MeasuredCap int64         `json:"measuredCap"`
	Label       string        `json:"workloadLabel"`
	mix         mix
}

// Block A's axes; each writer runs that many batchers.
var (
	cellWriters = map[string]int{"owner": 1, "ownerless": 32}
	cellDelays  = map[string]int{"0": 0, "1": 1, "10": 10}
	cellShapes  = map[string]mix{"outflow": mixOutflow, "inflow": mixInflow, "mixed": mixMixed}
)

var errNoCell = errors.New("set BENCH_MATRIX=quick, or full with BENCH_BLOCK=A, BENCH_WRITER, BENCH_DELAY_MS and BENCH_SHAPE")

// quickCell keeps the first timed cell and its workload label: owner, 1 ms, outflow, 20 s measured.
var quickCell = closedCell("quick", "quick", "owner", "outflow", 1, 20*time.Second)

// closedCell fixes what closed cells share: 10k counterparts, 256 producers, batch cap 256, a 60 s warm-up and memory
// ceilings of 16k attempts/s over each window. Reaching a ceiling invalidates the phase.
func closedCell(name, label, writer, shape string, delayMS int, measured time.Duration) cell {
	const warmup, capPerSecond = 60 * time.Second, 16_000

	return cell{
		Name: name, Writer: writer, Workers: cellWriters[writer], Shape: shape, DelayMS: delayMS, Ordinary: 10_000,
		Producers: 256, MaxBatch: 256, Warmup: warmup, Measured: measured, WarmupCap: capPerSecond * int64(warmup/time.Second),
		MeasuredCap: capPerSecond * int64(measured/time.Second), Label: label, mix: cellShapes[shape],
	}
}

// selectCell reads the cell from getenv and fails closed before any database work: quick takes no axis, full takes
// block A with each axis from its set. Unset BENCH_MATRIX is errNoCell.
func selectCell(getenv func(string) string) (cell, error) {
	block, writer, delay, shape := getenv("BENCH_BLOCK"), getenv("BENCH_WRITER"), getenv("BENCH_DELAY_MS"), getenv("BENCH_SHAPE")

	switch matrix := getenv("BENCH_MATRIX"); {
	case matrix == "":
		return cell{}, errNoCell
	case matrix == "quick" && block+writer+delay+shape == "":
		return quickCell, nil
	case matrix != "full":
		return cell{}, fmt.Errorf("BENCH_MATRIX=%q with BENCH_BLOCK/WRITER/DELAY_MS/SHAPE %q/%q/%q/%q: want quick without them, or full",
			matrix, block, writer, delay, shape)
	}

	_, knownWriter := cellWriters[writer]
	delayMS, knownDelay := cellDelays[delay]
	_, knownShape := cellShapes[shape]

	if block != "A" || !knownWriter || !knownDelay || !knownShape {
		return cell{}, fmt.Errorf("BENCH_MATRIX=full BENCH_BLOCK=%q BENCH_WRITER=%q BENCH_DELAY_MS=%q BENCH_SHAPE=%q: want A, owner|ownerless, 0|1|10 and outflow|inflow|mixed",
			block, writer, delay, shape)
	}

	return closedCell(fmt.Sprintf("A-%s-%sms-%s", writer, delay, shape), "A/"+shape, writer, shape, delayMS, 2*time.Minute), nil
}

// cellReport is one passed cell: structured counts and series, the existing log lines as opaque text, and what stays
// unmeasured. The window rate is Measured.InWindow over Measured.Window; nothing parses the text.
type cellReport struct {
	Cell       cell   `json:"cell"`
	PGSettings string `json:"pgSettings"`
	ProbeIdle  probe  `json:"sqlRoundtripPgReplIdle"`
	ProbeAfter probe  `json:"sqlRoundtripPgReplAfterClose"`
	Warmup     phase  `json:"warmup"`
	WarmupJoin struct {
		Unknown int64 `json:"unknown"` // bounds the attempts carried into the measured phase
		Queued  int   `json:"queued"`
	} `json:"warmupJoin"`
	Measured  phase  `json:"measured"`
	S1ToS2    string `json:"s1ToS2"`
	Occupancy struct {
		Batches  int `json:"batches"`
		Attempts int `json:"attempts"`
		Largest  int `json:"largest"`
	} `json:"measuredOccupancy"`
	Writer struct {
		Passes    int   `json:"passes"`
		Deadlocks int   `json:"deadlocks"`
		EntryLate int64 `json:"entryLate"`
		Publishes int64 `json:"publishes"`
	} `json:"writerLifetime"`
	Answers    map[string]int `json:"checkerAnswers"`
	Stages     []stage        `json:"stages"`
	Unmeasured []string       `json:"unmeasured"`
}

// probe is one idle sql_roundtrip_pg_repl sample set: SQL round trips across the delay, never network RTT.
type probe struct {
	N           int           `json:"n"`
	Min         time.Duration `json:"minNS"`
	MedianLower time.Duration `json:"medianLowerNS"`
	Max         time.Duration `json:"maxNS"`
}

// phase is one recorder after its run joined; approvals split by dbAckAt against the window's stop.
type phase struct {
	Summary    string            `json:"summary"`
	Dispatched int               `json:"dispatched"`
	Answered   int               `json:"answered"`
	Late       int64             `json:"late"`
	Outcomes   map[outcome]int64 `json:"outcomes"`
	InWindow   int64             `json:"approvalsDbAckBeforeStop"`
	AfterStop  int64             `json:"approvalsDbAckAtOrAfterStop"`
	NoDbAck    int64             `json:"approvalsWithoutDbAck"`
	Window     time.Duration     `json:"windowNS"`
	Answer     percentiles       `json:"answerAllOutcomes"`
	DbAck      percentiles       `json:"dbAckApprovals"`
}

// percentiles is a series' sample count and nearest-rank p50, p99, p99.9 and max: null without samples, never zero.
type percentiles struct {
	N         int               `json:"n"`
	Quantiles *[4]time.Duration `json:"p50P99P999MaxNS"`
}

type stage struct {
	Name     string        `json:"name"`
	Duration time.Duration `json:"durationNS"`
}

var unmeasured = []string{"Postgres CPU and memory", "cumulative lock wait", "standby fsync", "network RTT", "in-run sampling", "capture cost"}

func percentilesOf(series []time.Duration) percentiles {
	q, ok := quantiles(series)
	if !ok {
		return percentiles{}
	}

	return percentiles{N: len(series), Quantiles: &q}
}

// phaseOf reads rec after its run joined.
func phaseOf(rec *recorder) phase {
	summary := rec.summary()

	rec.mu.Lock()
	defer rec.mu.Unlock()

	return phase{
		Summary: summary, Dispatched: len(rec.lag), Answered: len(rec.latency), Late: rec.late, Outcomes: maps.Clone(rec.outcomes),
		InWindow: rec.inWindow, AfterStop: rec.afterStop, NoDbAck: rec.outcomes[outcomeApproved] - rec.inWindow - rec.afterStop,
		Window: rec.stop.Sub(rec.started), Answer: percentilesOf(rec.latency), DbAck: percentilesOf(rec.dbAck),
	}
}

// observe fills the writer's lifetime counters, the measured window's occupancy and both phases, after Close joined.
func (r *cellReport) observe(inner *ownerlessWriter, broker *refusingBroker, warm, meas *recorder) {
	for _, s := range inner.workers {
		r.Writer.Passes, r.Writer.Deadlocks = r.Writer.Passes+s.passes, r.Writer.Deadlocks+s.deadlocks
	}

	o := inner.occupancy(meas.started, meas.stop)
	r.Occupancy.Batches, r.Occupancy.Attempts, r.Occupancy.Largest = o.batches, o.attempts, o.largest
	r.Writer.EntryLate, r.Writer.Publishes = inner.entryLate.Load(), broker.publishes.Load()
	r.Warmup, r.Measured = phaseOf(warm), phaseOf(meas)
}

func (r *cellReport) stagesLine() string {
	var b strings.Builder
	for _, s := range r.Stages {
		fmt.Fprintf(&b, " %s=%s", s.Name, s.Duration)
	}

	return b.String()
}

// reportOnPass registers the cleanup that writes rep to path as JSON through a renamed temporary file. Registered
// first, it runs after every later cleanup; it writes only for a test that neither failed nor skipped.
func reportOnPass(t *testing.T, path string, rep any) {
	t.Helper()

	t.Cleanup(func() {
		if t.Failed() || t.Skipped() {
			return
		}

		data, err := json.MarshalIndent(rep, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path+".tmp", data, 0o600))
		require.NoError(t, os.Rename(path+".tmp", path))
	})
}

// Selection fails closed before any database work: quick takes no axis, full takes block A with each axis from its
// set. The quick keeps its label; the cells of one shape share theirs, whatever the writer or delay.
func TestSelectCell(t *testing.T) {
	getenv := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}

		return func(k string) string { return m[k] }
	}
	full := func(writer, delay, shape string) func(string) string {
		return getenv("BENCH_MATRIX", "full", "BENCH_BLOCK", "A", "BENCH_WRITER", writer, "BENCH_DELAY_MS", delay, "BENCH_SHAPE", shape)
	}

	_, err := selectCell(getenv())
	require.ErrorIs(t, err, errNoCell)

	got, err := selectCell(getenv("BENCH_MATRIX", "quick"))
	require.NoError(t, err)
	assert.Equal(t, cell{
		Name: "quick", Writer: "owner", Workers: 1, Shape: "outflow", DelayMS: 1, Ordinary: 10_000, Producers: 256, MaxBatch: 256,
		Warmup: time.Minute, Measured: 20 * time.Second, WarmupCap: 960_000, MeasuredCap: 320_000, Label: "quick", mix: mixOutflow,
	}, got)

	got, err = selectCell(full("ownerless", "10", "mixed"))
	require.NoError(t, err)
	assert.Equal(t, cell{
		Name: "A-ownerless-10ms-mixed", Writer: "ownerless", Workers: 32, Shape: "mixed", DelayMS: 10, Ordinary: 10_000,
		Producers: 256, MaxBatch: 256, Warmup: time.Minute, Measured: 2 * time.Minute, WarmupCap: 960_000,
		MeasuredCap: 1_920_000, Label: "A/mixed", mix: mixMixed,
	}, got)

	names := map[string]bool{}

	for writer := range cellWriters {
		for delay := range cellDelays {
			for shape := range cellShapes {
				c, err := selectCell(full(writer, delay, shape))
				require.NoError(t, err)

				names[c.Name] = true

				assert.Equal(t, "A/"+shape, c.Label, c.Name)
			}
		}
	}

	assert.Len(t, names, 18, "block A's cells")

	for name, env := range map[string]func(string) string{
		"quick with an axis":   getenv("BENCH_MATRIX", "quick", "BENCH_WRITER", "owner"),
		"quick with a block":   getenv("BENCH_MATRIX", "quick", "BENCH_BLOCK", "A"),
		"unknown matrix":       getenv("BENCH_MATRIX", "Quick"),
		"full without a block": getenv("BENCH_MATRIX", "full", "BENCH_WRITER", "owner", "BENCH_DELAY_MS", "1", "BENCH_SHAPE", "outflow"),
		"block B":              getenv("BENCH_MATRIX", "full", "BENCH_BLOCK", "B", "BENCH_WRITER", "owner", "BENCH_DELAY_MS", "1", "BENCH_SHAPE", "outflow"),
		"writer ownerless32":   full("ownerless32", "1", "outflow"),
		"writer unset":         full("", "1", "outflow"),
		"delay 5":              full("owner", "5", "outflow"),
		"delay 01":             full("owner", "01", "outflow"),
		"delay unset":          full("owner", "", "outflow"),
		"shape with fee":       full("owner", "1", "outflow_fee"),
		"shape unset":          full("owner", "1", ""),
	} {
		_, err := selectCell(env)
		require.Error(t, err, name)
		assert.NotErrorIs(t, err, errNoCell, name)
	}
}

// A phase carries exact counts, its window and each series' sample count; a series without samples is null, never
// zero. The window rate's numerator is the approvals with dbAckAt before stop.
func TestPhaseOf(t *testing.T) {
	start, stop := scheduleStart, scheduleStart.Add(time.Second)
	rec := newRecorder(4)
	rec.begin(start, stop)

	req := request{scheduledAt: start, deadline: start.Add(usualBudget.caller)}
	for _, a := range []struct {
		res result
		at  time.Duration
	}{
		{result{outcome: outcomeApproved, dbAckAt: start.Add(10 * time.Millisecond)}, 11 * time.Millisecond},
		{result{outcome: outcomeApproved, dbAckAt: stop}, time.Second + time.Millisecond},
		{result{outcome: outcomeApproved}, 20 * time.Millisecond},
		{result{outcome: outcomeExpired}, usualBudget.caller + answerGrace + time.Millisecond}, // late
	} {
		rec.dispatch(start, start)
		rec.observe(req, a.res, start.Add(a.at))
	}

	assert.Equal(t, phase{
		Summary: rec.summary(), Dispatched: 4, Answered: 4, Late: 1, Outcomes: map[outcome]int64{outcomeApproved: 3, outcomeExpired: 1},
		InWindow: 1, AfterStop: 1, NoDbAck: 1, Window: time.Second,
		Answer: percentiles{4, &[4]time.Duration{20 * time.Millisecond, 31001 * time.Millisecond, 31001 * time.Millisecond, 31001 * time.Millisecond}},
		DbAck:  percentiles{2, &[4]time.Duration{10 * time.Millisecond, time.Second, time.Second, time.Second}},
	}, phaseOf(rec))

	empty, err := json.Marshal(phaseOf(newRecorder(1)).DbAck)
	require.NoError(t, err)
	assert.JSONEq(t, `{"n": 0, "p50P99P999MaxNS": null}`, string(empty))
}

// A report exists only for a test that passed, after every cleanup registered later than its own, those included:
// a later cleanup's failure, or a skip, leaves no report and no temporary file. Each case runs in its own process.
func TestCellReportFinality(t *testing.T) {
	if mode := os.Getenv("BENCH_REPORT_PROBE"); mode != "" {
		var ran []string

		reportOnPass(t, os.Getenv("BENCH_REPORT"), &ran)
		t.Cleanup(func() {
			ran = append(ran, "later cleanup")

			if mode == "fail" {
				t.Error("a later cleanup failed")
			}
		})

		if mode == "skip" {
			t.Skip("skipped before the cleanups")
		}

		return
	}

	for mode, want := range map[string]string{"pass": `["later cleanup"]`, "fail": "", "skip": ""} {
		path := filepath.Join(t.TempDir(), "cell.json")
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCellReportFinality$")

		cmd.Env = append(os.Environ(), "BENCH_REPORT_PROBE="+mode, "BENCH_REPORT="+path)

		out, err := cmd.CombinedOutput()
		assert.Equal(t, mode == "fail", err != nil, "%s exit: %v\n%s", mode, err, out)
		assert.NoFileExists(t, path+".tmp", mode)

		if want == "" {
			assert.NoFileExists(t, path, mode)
		} else if got, err := os.ReadFile(path); assert.NoError(t, err, mode) {
			assert.JSONEq(t, want, string(got), mode)
		}
	}
}
