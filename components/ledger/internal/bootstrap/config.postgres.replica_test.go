// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildOptionalReplicaDSN_AbsentConfigurationYieldsEmptyDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                        string
		host, user, password, dbname, port, sslmode string
	}{
		{name: "all six fields empty"},
		{
			name: "all six fields whitespace only",
			host: "  ", user: "\t", password: " ", dbname: "\n", port: " ", sslmode: "  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dsn, err := buildOptionalReplicaDSN("onboarding", tt.host, tt.user, tt.password, tt.dbname, tt.port, tt.sslmode)
			require.NoError(t, err)
			assert.Equal(t, "", dsn, "absent replica must surface as the empty DSN so lib-commons opens a single pool")
		})
	}
}

func TestBuildOptionalReplicaDSN_FullConfigurationKeepsLegacyFormat(t *testing.T) {
	t.Parallel()

	dsn, err := buildOptionalReplicaDSN("transaction", "replica-host", "ruser", "rp@ss", "rdb", "5434", "verify-full")
	require.NoError(t, err)

	// Byte-identical to the format the builders emitted before the helper existed:
	// values are not trimmed or reordered.
	assert.Equal(t, "host=replica-host user=ruser password=rp@ss dbname=rdb port=5434 sslmode=verify-full", dsn)
}

func TestBuildOptionalReplicaDSN_FullConfigurationDoesNotTrimValues(t *testing.T) {
	t.Parallel()

	dsn, err := buildOptionalReplicaDSN("transaction", " h ", "u", "p", "d", "5432 ", "disable")
	require.NoError(t, err)
	assert.Equal(t, "host= h  user=u password=p dbname=d port=5432  sslmode=disable", dsn)
}

func TestBuildOptionalReplicaDSN_PartialConfigurationIsAnError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                        string
		module                                      string
		host, user, password, dbname, port, sslmode string
		wantMissing                                 string
	}{
		{
			name:        "only host set",
			module:      "onboarding",
			host:        "replica-host",
			wantMissing: "user, password, dbname, port, sslmode",
		},
		{
			name:   "host and port missing",
			module: "transaction",
			user:   "u", password: "p", dbname: "d", sslmode: "disable",
			wantMissing: "host, port",
		},
		{
			name:   "whitespace-only field counts as missing",
			module: "transaction",
			host:   "h", user: "u", password: "   ", dbname: "d", port: "5432", sslmode: "disable",
			wantMissing: "password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dsn, err := buildOptionalReplicaDSN(tt.module, tt.host, tt.user, tt.password, tt.dbname, tt.port, tt.sslmode)
			require.Error(t, err)
			assert.ErrorIs(t, err, errIncompleteReplicaConfig)
			assert.Equal(t, "", dsn, "a partial configuration must never produce a DSN")
			assert.Contains(t, err.Error(), tt.module, "error must name the module")
			assert.Contains(t, err.Error(), "missing: "+tt.wantMissing, "error must name exactly the missing fields, in DSN order")
			assert.NotContains(t, err.Error(), "password=", "error must not echo credential values")
		})
	}
}

func TestBuildOptionalReplicaDSN_PartialErrorNamesTheEnvVars(t *testing.T) {
	t.Parallel()

	_, err := buildOptionalReplicaDSN("onboarding", "h", "", "", "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_ONBOARDING_REPLICA_*", "operators must be told which variables to set or unset")
}
