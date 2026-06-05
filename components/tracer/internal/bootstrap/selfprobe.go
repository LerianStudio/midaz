// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	libLog "github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/observability"
)

// pgKVRedactKeys lists the libpq keyword/value keys whose values must be
// redacted in self-probe error logs. Order is irrelevant — we walk the slice
// once per sanitize call.
var pgKVRedactKeys = []string{"host=", "user=", "password=", "dbname=", "sslmode="}

// selfProbeCheckTimeout caps individual self-probe Check() invocations so a
// single hanging dependency cannot block the boot path indefinitely. The
// underlying adapters (e.g. PingContext) already honour the context deadline,
// but this wrapper adds a hard ceiling for adapters that don't enforce one
// internally — defence in depth for the startup gate.
//
// Declared as var (not const) so tests can shorten it to keep the timeout
// path fast — production code never reassigns it.
var selfProbeCheckTimeout = 5 * time.Second

// pgURLCreds matches the userinfo segment of a postgres:// or postgresql://
// URL. The capture group preserves the scheme so we can rewrite the segment
// to `postgres://[redacted]@` (or `postgresql://[redacted]@`) without
// dropping the scheme prefix the operator needs to identify the source.
var pgURLCreds = regexp.MustCompile(`(postgres(?:ql)?://)[^@\s/]*:[^@\s/]*@`)

// selfProbeOK gates /health: it starts at 0 (false) and is flipped to 1
// (true) only when RunSelfProbe finds every required dependency reachable.
// /health reads this atomic via IsSelfProbeOK and returns 503 while it is 0,
// so the K8s livenessProbe restarts the pod when startup never succeeds.
//
// Package-level by design — single source of truth for the entire process.
// Reads happen on the hot HTTP path (every /health request), so the atomic
// avoids any locking overhead. Writes happen exactly once at boot.
var selfProbeOK atomic.Int32

// IsSelfProbeOK is the single read point for the /health handler. Returns
// false until RunSelfProbe completes successfully.
func IsSelfProbeOK() bool {
	return selfProbeOK.Load() == 1
}

// SelfProbeChecker is satisfied by anything that can probe a single
// dependency at startup. The Check method is one-shot — it must return
// promptly (or honour ctx cancellation) so RunSelfProbe does not block the
// boot path.
type SelfProbeChecker interface {
	Check(ctx context.Context) error
}

// SelfProbeChecks is the input to RunSelfProbe — a name→checker map. Each
// entry is probed exactly once before RunSelfProbe returns.
type SelfProbeChecks map[string]SelfProbeChecker

// RunSelfProbe probes every entry in checks once, emits the selfprobe_result
// gauge per dep, and flips selfProbeOK to 1 IFF every probe succeeded.
//
// MUST be called from bootstrap BEFORE the HTTP server starts accepting
// traffic. While selfProbeOK is 0, /health returns 503 — K8s sees the pod
// unhealthy and routes around it.
//
// Iteration order is deterministic (sorted by dep name) so log lines come
// out in a stable order across runs and tests can assert on them.
//
// Returns a non-nil error when ANY probe failed; the caller (bootstrap)
// decides whether to abort startup or continue with /health returning 503.
// Tracer aborts: the bootstrap wraps the error and returns it from
// Service.Run, which causes main to exit with a non-zero status.
//
// A nil logger returns an error rather than panicking on logger.With(...) —
// this preserves the contract that RunSelfProbe never crashes the bootstrap
// goroutine and always leaves selfProbeOK in a known state (false).
//
// A nil recorder is treated as a no-op recorder — the probe still runs and
// flips selfProbeOK as expected; only the metric emission is skipped. This
// keeps tests that don't care about metrics simple while production always
// supplies a real recorder via observability.NewRecorder.
func RunSelfProbe(ctx context.Context, checks SelfProbeChecks, recorder *observability.Recorder, logger libLog.Logger) error {
	if logger == nil {
		return fmt.Errorf("self-probe: nil logger")
	}

	logger.With(
		libLog.String("probe", "self"),
		libLog.Int("dep_count", len(checks)),
	).Log(ctx, libLog.LevelInfo, "startup_self_probe_started")

	names := make([]string, 0, len(checks))
	for name := range checks {
		names = append(names, name)
	}

	sort.Strings(names)

	failed := make([]string, 0, len(names))

	for _, name := range names {
		checker := checks[name]
		// Defensive nil-checker guard. A nil entry in SelfProbeChecks would
		// panic on checker.Check(ctx); RunSelfProbe must instead surface a
		// structured error and leave selfProbeOK in its known-bad state so
		// /health keeps returning 503 and K8s recovers the pod.
		if checker == nil {
			err := fmt.Errorf("self-probe: nil checker for %q", name)
			failed = append(failed, name)
			recorder.EmitSelfProbeResult(ctx, name, false)

			logger.With(
				libLog.String("probe", "self"),
				libLog.String("name", name),
				libLog.String("status", "down"),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "self_probe_check")

			continue
		}

		start := time.Now().UTC()
		// Per-check timeout: bound each Check() invocation independently so a
		// single hanging dep cannot stall the rest of the probe sequence.
		// cancel() is called eagerly (not deferred) because we are in a loop —
		// a deferred cancel would accumulate timers across iterations.
		checkCtx, cancel := context.WithTimeout(ctx, selfProbeCheckTimeout)
		err := checker.Check(checkCtx)

		cancel()
		recorder.EmitSelfProbeResult(ctx, name, err == nil)

		if err == nil {
			logger.With(
				libLog.String("probe", "self"),
				libLog.String("name", name),
				libLog.String("status", "up"),
				libLog.Int("duration_ms", int(time.Since(start).Milliseconds())),
			).Log(ctx, libLog.LevelInfo, "self_probe_check")

			continue
		}

		failed = append(failed, name)

		logger.With(
			libLog.String("probe", "self"),
			libLog.String("name", name),
			libLog.String("status", "down"),
			libLog.Int("duration_ms", int(time.Since(start).Milliseconds())),
			libLog.String("error.message", sanitizeProbeError(err)),
		).Log(ctx, libLog.LevelError, "self_probe_check")
	}

	if len(failed) > 0 {
		logger.With(
			libLog.String("probe", "self"),
			libLog.String("failed_deps", strings.Join(failed, ",")),
		).Log(ctx, libLog.LevelError, "startup_self_probe_failed")

		return fmt.Errorf("self-probe failed for [%s]", strings.Join(failed, ","))
	}

	selfProbeOK.Store(1)
	logger.With(libLog.String("probe", "self")).Log(ctx, libLog.LevelInfo, "startup_self_probe_passed")

	return nil
}

// drainGracePeriod returns the operator-tunable drain grace duration. The
// default of 12s covers the K8s readinessProbe defaults
// (periodSeconds=5 × failureThreshold=2 = 10s) plus a small buffer so the
// kubelet has observed the 503 from /readyz before we begin tearing down
// dependencies.
//
// A non-positive ReadyzDrainGraceSeconds (zero or negative) falls back to
// 12s. This keeps misconfigured deployments safe — operators get the
// production-grade default rather than zero seconds (which would skip the
// drain entirely and kill in-flight requests).
func drainGracePeriod(cfg *Config) time.Duration {
	const defaultGrace = 12 * time.Second

	if cfg == nil || cfg.ReadyzDrainGraceSeconds <= 0 {
		return defaultGrace
	}

	return time.Duration(cfg.ReadyzDrainGraceSeconds) * time.Second
}

// sanitizeProbeError strips connection-string fragments that pgx and other
// drivers commonly include in error messages (host=..., user=..., password=...,
// dbname=..., sslmode=...) and userinfo embedded in postgres:// URLs.
// Keeps the underlying error class for operator debugging while preventing
// credential leakage into log aggregators that may have weaker access
// controls than the database itself.
//
// Two redaction passes apply in this order:
//
//  1. URL-form credentials. `postgres://user:pass@host/db` is collapsed to
//     `postgres://[redacted]@host/db` via the pgURLCreds regex. Both
//     `postgres://` and `postgresql://` schemes are matched.
//
//  2. libpq keyword/value pairs. For each `key=` prefix in pgKVRedactKeys,
//     the value is redacted in place. Three value forms are handled:
//
//     - unquoted: terminated at the next whitespace or end-of-string;
//     - single-quoted (`key='abc def'`): consumed up to the matching close
//     quote, with backslash-escapes honoured;
//     - double-quoted (`key="abc def"`): same as single-quoted.
//
// The walk advances past the inserted `[redacted]` placeholder so the next
// strings.Index call cannot re-match the literal we just wrote.
func sanitizeProbeError(err error) string {
	if err == nil {
		return ""
	}

	s := err.Error()

	// 1. Redact URL-form credentials before keyword/value pairs so that the
	//    `password=`-in-query-string form (`?sslmode=require&password=foo`)
	//    is still caught by the kv pass below.
	s = pgURLCreds.ReplaceAllString(s, "${1}[redacted]@")

	// 2. Redact keyword/value pairs (handles quoted + unquoted libpq forms).
	for _, key := range pgKVRedactKeys {
		s = redactKVPair(s, key)
	}

	return s
}

// redactKVPair redacts the value following `key=` in s, handling unquoted
// (whitespace-terminated), single-quoted, and double-quoted libpq values.
// Multiple occurrences of the same key in a single error string are all
// redacted (e.g. an aggregated multi-error).
func redactKVPair(s, key string) string {
	const placeholder = "[redacted]"

	offset := 0
	for {
		i := strings.Index(s[offset:], key)
		if i < 0 {
			return s
		}

		valStart := offset + i + len(key)
		if valStart >= len(s) {
			return s[:valStart] + placeholder
		}

		end := scanLibpqValueEnd(s, valStart)
		s = s[:valStart] + placeholder + s[end:]
		offset = valStart + len(placeholder)
	}
}

// scanLibpqValueEnd returns the index in s just past the end of the libpq
// value that starts at valStart. The three branches mirror the libpq
// connection-string grammar:
//
//   - leading single quote ⇒ single-quoted value (backslash-escapes honoured);
//   - leading double quote ⇒ double-quoted value (same rules);
//   - anything else        ⇒ unquoted value, terminated at the first whitespace.
//
// Pulling the value-shape switch out of redactKVPair drops the gocognit
// score below the package threshold without changing semantics.
func scanLibpqValueEnd(s string, valStart int) int {
	switch s[valStart] {
	case '\'':
		return scanQuotedValueEnd(s, valStart, '\'')
	case '"':
		return scanQuotedValueEnd(s, valStart, '"')
	default:
		return scanUnquotedValueEnd(s, valStart)
	}
}

// scanQuotedValueEnd returns the index in s just past the matching close
// quote of the value starting at valStart. Backslash escapes (`\'`, `\"`) are
// skipped over so they cannot prematurely terminate the value.
func scanQuotedValueEnd(s string, valStart int, quote byte) int {
	end := valStart + 1
	for end < len(s) && s[end] != quote {
		if s[end] == '\\' && end+1 < len(s) {
			end++
		}

		end++
	}

	if end < len(s) {
		end++ // consume the closing quote
	}

	return end
}

// scanUnquotedValueEnd returns the index in s just past the end of an
// unquoted libpq value (terminated at the first whitespace or end of string).
func scanUnquotedValueEnd(s string, valStart int) int {
	end := valStart
	for end < len(s) && !unicode.IsSpace(rune(s[end])) {
		end++
	}

	return end
}
