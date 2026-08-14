// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrationsimage

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	migrateentrypoint "github.com/LerianStudio/midaz/v4/tests/utils/migrateentrypoint"
)

func TestEntrypoint_DSNAssembly(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available on PATH; skipping shell entrypoint test")
	}

	entrypoint, err := filepath.Abs("entrypoint.sh")
	require.NoError(t, err, "failed to resolve entrypoint.sh path")

	if _, err := os.Stat(entrypoint); err != nil {
		t.Skipf("entrypoint.sh not found at %s: %v", entrypoint, err)
	}

	tests := []struct {
		name string
		env  map[string]string
		// assert receives the two captured invocations in call order
		// (onboarding first, transaction second).
		assert func(t *testing.T, calls []migrateentrypoint.Invocation)
	}{
		{
			name: "url overrides are used verbatim",
			env: map[string]string{
				"ONBOARDING_DATABASE_URL":  "postgres://verbatim-onb@host:1/onb?sslmode=require",
				"TRANSACTION_DATABASE_URL": "postgres://verbatim-txn@host:2/txn?sslmode=require",
			},
			assert: func(t *testing.T, calls []migrateentrypoint.Invocation) {
				onbDB, ok := calls[0].Flag("-database")
				require.True(t, ok, "onboarding invocation must carry -database")
				require.Equal(t, "postgres://verbatim-onb@host:1/onb?sslmode=require", onbDB)

				txnDB, ok := calls[1].Flag("-database")
				require.True(t, ok, "transaction invocation must carry -database")
				require.Equal(t, "postgres://verbatim-txn@host:2/txn?sslmode=require", txnDB)

				// Overrides are passed through verbatim: encode_password is
				// bypassed entirely, so the literal `@` is NOT percent-encoded.
				require.NotContains(t, onbDB, "%40",
					"onboarding override DSN must not be percent-encoded")
				require.NotContains(t, txnDB, "%40",
					"transaction override DSN must not be percent-encoded")
			},
		},
		{
			name: "assembled dsn percent-encodes the password",
			env: map[string]string{
				"DB_ONBOARDING_HOST":     "onb-host",
				"DB_ONBOARDING_PORT":     "6001",
				"DB_ONBOARDING_USER":     "onb_user",
				"DB_ONBOARDING_PASSWORD": "p@ss:/% word?#&+[]",
				"DB_ONBOARDING_NAME":     "onb_db",
				"DB_ONBOARDING_SSLMODE":  "require",

				"DB_TRANSACTION_HOST":     "txn-host",
				"DB_TRANSACTION_PORT":     "6002",
				"DB_TRANSACTION_USER":     "txn_user",
				"DB_TRANSACTION_PASSWORD": "p@ss:/% word?#&+[]",
				"DB_TRANSACTION_NAME":     "txn_db",
				"DB_TRANSACTION_SSLMODE":  "require",
			},
			assert: func(t *testing.T, calls []migrateentrypoint.Invocation) {
				// encode_password substitutes `%` FIRST so already-inserted
				// escapes are not double-encoded, then the remaining reserved
				// characters in order:
				//   % -> %25  @ -> %40  : -> %3A  / -> %2F  ? -> %3F
				//   # -> %23  & -> %26  + -> %2B  space -> %20
				//   [ -> %5B  ] -> %5D
				// All 11 reserved characters the sed chain handles are covered.
				wantPwd := "p%40ss%3A%2F%25%20word%3F%23%26%2B%5B%5D"

				onbDB, ok := calls[0].Flag("-database")
				require.True(t, ok, "onboarding invocation must carry -database")
				require.Equal(t,
					"postgres://onb_user:"+wantPwd+"@onb-host:6001/onb_db?sslmode=require",
					onbDB,
				)

				txnDB, ok := calls[1].Flag("-database")
				require.True(t, ok, "transaction invocation must carry -database")
				require.Equal(t,
					"postgres://txn_user:"+wantPwd+"@txn-host:6002/txn_db?sslmode=require",
					txnDB,
				)
			},
		},
		{
			name: "port and sslmode default when unset",
			env: map[string]string{
				"DB_ONBOARDING_HOST":     "onb-host",
				"DB_ONBOARDING_USER":     "onb_user",
				"DB_ONBOARDING_PASSWORD": "simple",
				"DB_ONBOARDING_NAME":     "onb_db",

				"DB_TRANSACTION_HOST":     "txn-host",
				"DB_TRANSACTION_USER":     "txn_user",
				"DB_TRANSACTION_PASSWORD": "simple",
				"DB_TRANSACTION_NAME":     "txn_db",
			},
			assert: func(t *testing.T, calls []migrateentrypoint.Invocation) {
				onbDB, ok := calls[0].Flag("-database")
				require.True(t, ok, "onboarding invocation must carry -database")
				require.Equal(t,
					"postgres://onb_user:simple@onb-host:5432/onb_db?sslmode=disable",
					onbDB,
				)

				txnDB, ok := calls[1].Flag("-database")
				require.True(t, ok, "transaction invocation must carry -database")
				require.Equal(t,
					"postgres://txn_user:simple@txn-host:5432/txn_db?sslmode=disable",
					txnDB,
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			calls := migrateentrypoint.RunEntrypoint(t, entrypoint, tt.env)

			// Ordering + path contract holds for every case: onboarding first,
			// then transaction.
			require.Len(t, calls, 2, "entrypoint must invoke migrate exactly twice")

			onbPath, ok := calls[0].Flag("-path")
			require.True(t, ok, "first invocation must carry -path")
			require.Equal(t, "/migrations/onboarding", onbPath,
				"first migrate call must target onboarding")

			txnPath, ok := calls[1].Flag("-path")
			require.True(t, ok, "second invocation must carry -path")
			require.Equal(t, "/migrations/transaction", txnPath,
				"second migrate call must target transaction")

			// Both invocations must be `up` calls (trailing arg).
			onbUp := calls[0].Args[len(calls[0].Args)-1]
			require.Equal(t, "up", onbUp, "onboarding migrate call must be an `up` invocation")

			txnUp := calls[1].Args[len(calls[1].Args)-1]
			require.Equal(t, "up", txnUp, "transaction migrate call must be an `up` invocation")

			tt.assert(t, calls)
		})
	}
}
