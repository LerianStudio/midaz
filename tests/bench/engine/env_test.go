//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPrimaryDSN   = "postgresql://bench:bench@127.0.0.1:15432/bench_transaction?sslmode=disable"
	testClientDSN    = "postgresql://bench:bench@127.0.0.1:15434/bench_transaction?sslmode=disable"
	testStandbyDSN   = "postgresql://bench:bench@127.0.0.1:15435/bench_transaction?sslmode=disable"
	testToxiproxyURL = "http://127.0.0.1:18474"
)

// pgconnEnv lists the PG* defaults pgx v5.11 reads (pgconn parseEnvSettings); empty means unset.
var pgconnEnv = [...]string{
	"PGHOST", "PGPORT", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGPASSFILE", "PGAPPNAME", "PGCONNECT_TIMEOUT",
	"PGSSLMODE", "PGSSLKEY", "PGSSLCERT", "PGSSLSNI", "PGSSLROOTCERT", "PGSSLPASSWORD", "PGSSLNEGOTIATION",
	"PGTARGETSESSIONATTRS", "PGSERVICE", "PGSERVICEFILE", "PGTZ", "PGOPTIONS", "PGMINPROTOCOLVERSION",
	"PGMAXPROTOCOLVERSION", "PGCHANNELBINDING", "PGREQUIREAUTH",
}

// setBenchEnv also clears the developer's PG* defaults, so only each case's own settings decide the parse.
func setBenchEnv(t *testing.T, primary, clientProxy, standby, toxiproxy string) {
	t.Helper()

	for _, name := range pgconnEnv {
		t.Setenv(name, "")
	}

	t.Setenv("BENCH_PG_PRIMARY_DSN", primary)
	t.Setenv("BENCH_PG_CLIENT_PROXY_DSN", clientProxy)
	t.Setenv("BENCH_PG_STANDBY_DSN", standby)
	t.Setenv("BENCH_TOXIPROXY_URL", toxiproxy)
}

func TestBenchEnvUnsetReturnsSentinel(t *testing.T) {
	setBenchEnv(t, "", "", "", "")

	_, err := loadBenchEnv()
	require.ErrorIs(t, err, errBenchEnvUnset)
}

func TestBenchEnvPartialFails(t *testing.T) {
	setBenchEnv(t, testPrimaryDSN, "", testStandbyDSN, "")

	_, err := loadBenchEnv()
	require.Error(t, err)
	require.NotErrorIs(t, err, errBenchEnvUnset)
	assert.ErrorContains(t, err, "BENCH_PG_CLIENT_PROXY_DSN")
	assert.ErrorContains(t, err, "BENCH_TOXIPROXY_URL")
}

func TestBenchEnvAcceptsLocalStack(t *testing.T) {
	cases := map[string]struct {
		primary  string
		wantPort uint16
	}{
		"compose defaults": {testPrimaryDSN, 15432},
		// The default sslmode=prefer adds a plaintext fallback to the same endpoint, not a new host.
		"default sslmode": {"postgresql://bench:bench@127.0.0.1:15432/bench_transaction", 15432},
		"overridden port": {"host=127.0.0.1 port=25432 user=bench password=bench dbname=bench_transaction", 25432},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			setBenchEnv(t, tc.primary, testClientDSN, testStandbyDSN, testToxiproxyURL)

			env, err := loadBenchEnv()
			require.NoError(t, err)
			assert.Equal(t, tc.wantPort, env.primary.Port)
			assert.Equal(t, uint16(15434), env.clientProxy.Port)
			assert.Equal(t, uint16(15435), env.standby.Port)
			assert.Equal(t, "127.0.0.1:18474", env.toxiproxy.Host)
			assert.Equal(t, 10*time.Second, env.primary.ConnectTimeout)
			assert.Equal(t, lockTimeout, env.primary.RuntimeParams["lock_timeout"])
		})
	}
}

func TestBenchEnvRejectsMisdirectedDestination(t *testing.T) {
	cases := map[string]struct {
		primary   string
		toxiproxy string
		pgHost    string
		wantVar   string
	}{
		"remote host":           {primary: "postgresql://bench:s3cret@10.0.0.5:15432/bench_transaction"},
		"hostname not loopback": {primary: "postgresql://bench:s3cret@localhost:15432/bench_transaction"},
		"query host override":   {primary: "postgresql://bench:s3cret@127.0.0.1:15432/bench_transaction?host=10.0.0.5"},
		"fallback host":         {primary: "postgresql://bench:s3cret@127.0.0.1:15432,10.0.0.5:5432/bench_transaction"},
		"fallback port":         {primary: "postgresql://bench:s3cret@127.0.0.1:15432,127.0.0.1:5432/bench_transaction"},
		"unix socket":           {primary: "host=/var/run/postgresql port=15432 user=bench password=s3cret dbname=bench_transaction"},
		"database override":     {primary: "postgresql://bench:s3cret@127.0.0.1:15432/bench_transaction?dbname=postgres"},
		"user override":         {primary: "postgresql://bench:s3cret@127.0.0.1:15432/bench_transaction?user=postgres"},
		"PGHOST default":        {primary: "port=15432 user=bench password=s3cret dbname=bench_transaction", pgHost: "10.0.0.5"},
		"unparseable":           {primary: "postgresql://bench:s3cret@127.0.0.1:notaport/bench_transaction"},
		"remote toxiproxy":      {toxiproxy: "http://10.0.0.5:18474", wantVar: "BENCH_TOXIPROXY_URL"},
		"toxiproxy credentials": {toxiproxy: "http://admin:s3cret@127.0.0.1:18474", wantVar: "BENCH_TOXIPROXY_URL"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			primary, toxiproxy, wantVar := testPrimaryDSN, testToxiproxyURL, "BENCH_PG_PRIMARY_DSN"
			if tc.primary != "" {
				primary = tc.primary
			}

			if tc.toxiproxy != "" {
				toxiproxy, wantVar = tc.toxiproxy, tc.wantVar
			}

			setBenchEnv(t, primary, testClientDSN, testStandbyDSN, toxiproxy)
			t.Setenv("PGHOST", tc.pgHost)

			_, err := loadBenchEnv()
			require.Error(t, err)
			assert.ErrorContains(t, err, wantVar)
			assert.NotContains(t, err.Error(), "s3cret", "errors must not carry credentials")
		})
	}
}
