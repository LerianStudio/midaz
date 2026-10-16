//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// answerGrace is how far past its deadline an answer may arrive before it fails the run.
const answerGrace = time.Second

// recorder holds one run's counters and whole latency series, measured from scheduledAt for exact quantiles.
type recorder struct {
	mu                               sync.Mutex
	capacity, late                   int64
	outcomes                         map[outcome]int64 // deciding answers and replies alike
	replays                          map[outcome]int64 // by the original outcome a replay repeats
	refusals                         map[string]int64  // by reason, deciding refusals only
	lag, latency                     []time.Duration   // one per dispatch and per answer: their lengths are the counts
	decisionStart, dbAck, brokerAck  []time.Duration   // only answers carrying the instant; acks of approvals
	inflight                         []int             // open-loop samples; the last at dispatch end
	started, lastDispatch, lastReply time.Time
}

// newRecorder allocates the five series before the run, 40 B per request; appends within capacity do not copy.
// First-touch page faults may still occur during the run.
func newRecorder(capacity int64) *recorder {
	r := &recorder{capacity: capacity, outcomes: map[outcome]int64{}, replays: map[outcome]int64{}, refusals: map[string]int64{}}
	for _, series := range []*[]time.Duration{&r.lag, &r.latency, &r.decisionStart, &r.dbAck, &r.brokerAck} {
		*series = make([]time.Duration, 0, capacity)
	}

	return r
}

// begin stamps the measured start.
func (r *recorder) begin(started time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.started = started
}

func (r *recorder) dispatch(scheduledAt, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lag = append(r.lag, at.Sub(scheduledAt))

	if at.After(r.lastDispatch) {
		r.lastDispatch = at
	}
}

func (r *recorder) observe(req request, res result, repliedAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.outcomes[res.outcome]++
	r.latency = append(r.latency, repliedAt.Sub(req.scheduledAt))

	if repliedAt.After(r.lastReply) {
		r.lastReply = repliedAt
	}

	if repliedAt.After(req.deadline.Add(answerGrace)) {
		r.late++
	}

	r.decisionStart = appendSince(r.decisionStart, res.decisionStartedAt, req.scheduledAt)

	switch res.outcome {
	case outcomeRefused:
		r.refusals[res.reason]++
	case outcomeReplay:
		r.replays[res.original]++
	case outcomeApproved:
		r.dbAck = appendSince(r.dbAck, res.dbAckAt, req.scheduledAt)
		r.brokerAck = appendSince(r.brokerAck, res.brokerAckAt, req.scheduledAt)
	}
}

// appendSince skips a zero instant: the path did not measure it, which is not a zero latency.
func appendSince(series []time.Duration, at, since time.Time) []time.Duration {
	if at.IsZero() {
		return series
	}

	return append(series, at.Sub(since))
}

func (r *recorder) sample() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.inflight = append(r.inflight, len(r.lag)-len(r.latency))
}

func (r *recorder) lateErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.late == 0 {
		return nil
	}

	return fmt.Errorf("%d answers arrived after deadline + %s: the writer overran it or dispatch lag spent the budget", r.late, answerGrace)
}

// quantiles returns the nearest-rank p50, p99, p99.9 and max of a sorted copy; ok is false for an empty series.
func quantiles(series []time.Duration) (q [4]time.Duration, ok bool) {
	if len(series) == 0 {
		return q, false
	}

	sorted := slices.Sorted(slices.Values(series))
	for k, perMille := range [4]int{500, 990, 999, 1000} {
		q[k] = sorted[(perMille*len(sorted)+999)/1000-1]
	}

	return q, true
}

func perSecond(n int, d time.Duration) string {
	if n == 0 || d <= 0 {
		return "unmeasured"
	}

	return strconv.FormatFloat(float64(n)/d.Seconds(), 'f', 0, 64)
}

func (r *recorder) summary() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var b strings.Builder

	fmt.Fprintf(&b, "capacity=%d dispatched=%d answered=%d late=%d outcomes=%v replays=%v refusals=%v sent/s(cohort)=%s answered/s(cohort)=%s",
		r.capacity, len(r.lag), len(r.latency), r.late, r.outcomes, r.replays, r.refusals,
		perSecond(len(r.lag), r.lastDispatch.Sub(r.started)), perSecond(len(r.latency), r.lastReply.Sub(r.started)))

	for _, s := range []struct {
		name   string
		series []time.Duration
	}{
		{"dispatchLag", r.lag},
		{"answer(all outcomes)", r.latency},
		{"decisionStart", r.decisionStart},
		{"dbAck(approvals)", r.dbAck},
		{"brokerAck(approvals)", r.brokerAck},
	} {
		if q, ok := quantiles(s.series); ok {
			fmt.Fprintf(&b, " %s n=%d p50/p99/p99.9/max=%s/%s/%s/%s", s.name, len(s.series), q[0], q[1], q[2], q[3])
		} else {
			fmt.Fprintf(&b, " %s=unmeasured", s.name)
		}
	}

	if r.lastReply.IsZero() {
		b.WriteString(" drain=unmeasured")
	} else {
		fmt.Fprintf(&b, " drain(lastAnswer-lastDispatch)=%s", r.lastReply.Sub(r.lastDispatch))
	}

	if len(r.inflight) > 0 {
		fmt.Fprintf(&b, " inflight max=%d atDispatchEnd=%d", slices.Max(r.inflight), r.inflight[len(r.inflight)-1])
	}

	return b.String()
}
