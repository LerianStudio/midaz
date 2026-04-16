// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package loader

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

// fakeRow is a minimal stand-in for pgx.Rows.Scan. It fills each dest pointer
// from values in the same order the real query produces them. A values entry
// of untyped nil means "the column was NULL"; scanner targets that accept
// NULL (pointer-to-string, sql.NullBool) are left in their zero state.
type fakeRow struct {
	values []any
}

func (f *fakeRow) Scan(dest ...any) error {
	if len(dest) != len(f.values) {
		return errScanArity
	}

	for i, d := range dest {
		src := f.values[i]

		// NULL column → leave target at zero value. For *string targets this
		// means the outer pointer stays nil; for sql.NullBool it means Valid
		// stays false. This mirrors how pgx handles NULLs in production.
		if src == nil {
			continue
		}

		dv := reflect.ValueOf(d).Elem()
		sv := reflect.ValueOf(src)

		if !sv.Type().AssignableTo(dv.Type()) {
			return errScanType
		}

		dv.Set(sv)
	}

	return nil
}

// TestLoadBalances_ScansNullAvailable verifies that a NULL available column
// no longer crashes the loader. Previously the scan target was a plain
// string which silently became "" on NULL, then decimal.NewFromString("")
// returned a parse error that bubbled out of the bootstrap load and
// fail-closed the entire cold start. After the fix, a NULL decodes as "0".
func TestLoadBalances_ScansNullAvailable(t *testing.T) {
	row := &fakeRow{values: []any{
		"balance-id-1", // id
		"org",          // organization_id
		"ledger",       // ledger_id
		"account-id-1", // account_id
		"@alice",       // alias
		"default",      // key
		"USD",          // asset_code
		nil,            // available — NULL
		stringPtr("0"), // on_hold
		int64(1),       // version
		"deposit",      // account_type
		sql.NullBool{Bool: true, Valid: true},
		sql.NullBool{Bool: true, Valid: true},
	}}

	balance, err := scanBalance(row, shard.NewRouter(shard.DefaultShardCount), nil)
	require.NoError(t, err, "NULL decimal column must decode without panic")
	require.NotNil(t, balance)
	require.Equal(t, int64(0), balance.Available,
		"NULL available must coalesce to zero, not an invalid decimal parse")
}

// TestLoadBalances_ScansNullOnHold mirrors the available test for the second
// decimal column so the guard is covered on both.
func TestLoadBalances_ScansNullOnHold(t *testing.T) {
	row := &fakeRow{values: []any{
		"balance-id-2",
		"org", "ledger", "account-id-2", "@bob", "default", "USD",
		stringPtr("500"), // available
		nil,              // on_hold — NULL
		int64(2),
		"deposit",
		sql.NullBool{Bool: true, Valid: true},
		sql.NullBool{Bool: true, Valid: true},
	}}

	balance, err := scanBalance(row, shard.NewRouter(shard.DefaultShardCount), nil)
	require.NoError(t, err)
	require.NotNil(t, balance)
	require.Equal(t, int64(0), balance.OnHold)
}

// TestLoadBalances_ScansNullAllowBool verifies that NULL in allow_sending or
// allow_receiving coerces to the fail-safe default (true). The choice of
// true matches the documented coalesceAllowBool rationale: a NULL flag must
// not silently revoke the account's ability to transact.
func TestLoadBalances_ScansNullAllowBool(t *testing.T) {
	row := &fakeRow{values: []any{
		"balance-id-3",
		"org", "ledger", "account-id-3", "@carol", "default", "USD",
		stringPtr("0"), stringPtr("0"),
		int64(3),
		"deposit",
		sql.NullBool{}, // allow_sending   — NULL
		sql.NullBool{}, // allow_receiving — NULL
	}}

	balance, err := scanBalance(row, shard.NewRouter(shard.DefaultShardCount), nil)
	require.NoError(t, err)
	require.NotNil(t, balance)
	require.True(t, balance.AllowSending, "NULL allow_sending must default to true (fail-safe)")
	require.True(t, balance.AllowReceiving, "NULL allow_receiving must default to true (fail-safe)")
}

// TestCoalesceDecimalString_Nil proves the helper maps nil → "0".
func TestCoalesceDecimalString_Nil(t *testing.T) {
	require.Equal(t, "0", coalesceDecimalString(nil))
	require.Equal(t, "42.5", coalesceDecimalString(stringPtr("42.5")))
}

// TestCoalesceAllowBool_Invalid proves NULL→true and honors explicit false.
func TestCoalesceAllowBool_Invalid(t *testing.T) {
	require.True(t, coalesceAllowBool(sql.NullBool{}))
	require.True(t, coalesceAllowBool(sql.NullBool{Bool: true, Valid: true}))
	require.False(t, coalesceAllowBool(sql.NullBool{Bool: false, Valid: true}))
}

func stringPtr(s string) *string { return &s }

var (
	errScanArity = &scanError{msg: "fakeRow: dest arity mismatch"}
	errScanType  = &scanError{msg: "fakeRow: dest type mismatch"}
)

type scanError struct{ msg string }

func (e *scanError) Error() string { return e.msg }
