//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"time"

	toxiproxyclient "github.com/Shopify/toxiproxy/v2/client"
)

const (
	replProxyName    = "pg-repl"
	replLatencyToxic = "pg-repl-latency"
	probeWarmup      = 3
	probeSamples     = 32
	// The outer bound kills only the docker CLI; the inner timeout ends psql inside the container.
	probeOuterTimeout = 60 * time.Second
	probeInnerTimeout = "50s"
)

// setReplLatency requires the read-back to contain only the requested downstream delay, or no toxic for 0.
// Removal releases buffered WAL undelayed: call with no workload. After an error the state is unknown;
// the caller's cleanup re-reads it without retrying the add.
func setReplLatency(env *benchEnv, latencyMS int) error {
	proxy, err := toxiproxyclient.NewClient(env.toxiproxy.String()).Proxy(replProxyName)
	if err != nil {
		return fmt.Errorf("read %s: %w", replProxyName, err)
	}

	// toxiproxy.json listens on 0.0.0.0:5433; the server reports the wildcard address it bound.
	if proxy.Upstream != "bench-pg-primary:5432" || !strings.HasSuffix(proxy.Listen, ":5433") || !proxy.Enabled {
		return fmt.Errorf("%s listens %q upstream %q enabled %t, want :5433, bench-pg-primary:5432, enabled",
			replProxyName, proxy.Listen, proxy.Upstream, proxy.Enabled)
	}

	for _, toxic := range proxy.ActiveToxics {
		if err := proxy.RemoveToxic(toxic.Name); err != nil {
			return fmt.Errorf("remove %s toxic %q: %w", replProxyName, toxic.Name, err)
		}
	}

	want := toxiproxyclient.Toxics{}

	if latencyMS > 0 {
		attrs := toxiproxyclient.Attributes{"latency": latencyMS, "jitter": 0}
		if _, err := proxy.AddToxic(replLatencyToxic, "latency", "downstream", 1, attrs); err != nil {
			return fmt.Errorf("add %s latency %d ms: %w", replProxyName, latencyMS, err)
		}

		// JSON numbers read back as float64.
		want = toxiproxyclient.Toxics{{
			Name: replLatencyToxic, Type: "latency", Stream: "downstream", Toxicity: 1,
			Attributes: toxiproxyclient.Attributes{"latency": float64(latencyMS), "jitter": 0.0},
		}}
	}

	got, err := proxy.Toxics()
	if err != nil {
		return fmt.Errorf("read back %s toxics: %w", replProxyName, err)
	}

	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("%s toxics %+v, want %+v", replProxyName, got, want)
	}

	return nil
}

// probeReplRoundtrip measures sql_roundtrip_pg_repl on one connection, excluding startup and warm-up queries.
// Samples include server execution and scheduling, not just network RTT. Run with no workload.
func probeReplRoundtrip(ctx context.Context) ([]time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, probeOuterTimeout)
	defer cancel()

	// The standby's environment already holds the disposable password.
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", "-e", "LC_ALL=C", "bench-pg-standby",
		"timeout", probeInnerTimeout, "psql", "-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1",
		"host=bench-toxiproxy port=5433 user=bench dbname=bench_transaction application_name=bench-sql-roundtrip sslmode=disable connect_timeout=5")
	cmd.Stdin = strings.NewReader(`\timing on` + "\n" + strings.Repeat("SELECT 1;\n", probeWarmup+probeSamples))

	out, err := cmd.Output()
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
		return nil, fmt.Errorf("psql through %s: %w: %s", replProxyName, err, bytes.TrimSpace(exitErr.Stderr))
	}

	if err != nil {
		return nil, fmt.Errorf("psql through %s: %w", replProxyName, err)
	}

	samples, err := parsePsqlTimings(string(out), probeWarmup+probeSamples)
	if err != nil {
		return nil, err
	}

	return samples[probeWarmup:], nil
}

// psqlTiming is psql's \timing line; from 1 s up it appends a human-readable copy in parentheses.
var psqlTiming = regexp.MustCompile(`^Time: ([0-9]+\.[0-9]{3}) ms(?: \(.+\))?$`)

// parsePsqlTimings reads n pairs of a "1" row and its timing line from psql -A -t output under LC_ALL=C. Any other
// line or count is an error: a missing sample is never a zero.
func parsePsqlTimings(out string, n int) ([]time.Duration, error) {
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2*n {
		return nil, fmt.Errorf("psql printed %d lines, want %d: %q", len(lines), 2*n, out)
	}

	samples := make([]time.Duration, n)
	for i := range samples {
		row, timing := lines[2*i], lines[2*i+1]

		m := psqlTiming.FindStringSubmatch(timing)
		if row != "1" || m == nil {
			return nil, fmt.Errorf("psql query %d printed %q, %q; want 1 and its timing", i+1, row, timing)
		}

		d, err := time.ParseDuration(m[1] + "ms")
		if err != nil {
			return nil, fmt.Errorf("psql query %d timing %q: %w", i+1, timing, err)
		}

		samples[i] = d
	}

	return samples, nil
}
