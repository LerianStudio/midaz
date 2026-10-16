// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// The formats below cross releases: completion plans and records, write-behind
// messages and execution results are written by one pod and read by another, which
// during a rolling deploy or after a rollback runs v4.2.2, and v4.2.2 decodes them
// with unknown fields refused. The API responses are the public contract. The
// fixtures in testdata/wire_v4_2_2 were produced by v4.2.2 itself from the values
// wireFormatFixtures builds, so any field this release adds to these encodings,
// configuration versions included, shows up as a byte difference here.
func TestWireFormats_EncodeExactlyAsTheRollbackRelease(t *testing.T) {
	t.Parallel()

	formats := wireFormatFixtures()

	// Every configuration version these types carry is set, so a version that
	// leaked into JSON would change the bytes.
	formats.balance.ConfigVersion = 11
	formats.balance.AccountConfigVersion = 15
	formats.account.ConfigVersion = 12
	formats.snapshot.ConfigVersion = 13
	formats.snapshot.AccountConfigVersion = 14
	formats.result.Final[0].ConfigVersion = 13
	formats.result.Final[0].AccountConfigVersion = 14
	formats.record.Result.Final[0].ConfigVersion = 13
	formats.record.Result.Final[0].AccountConfigVersion = 14
	formats.plan.OperationSpecs[0].Balance.ConfigVersion = 11
	formats.plan.OperationSpecs[0].Balance.AccountConfigVersion = 15

	encodings, err := formats.encodings()
	require.NoError(t, err)
	require.Len(t, encodings, 6)

	for name, encoded := range encodings {
		expected, readErr := os.ReadFile(filepath.Join("testdata", "wire_v4_2_2", name))
		require.NoError(t, readErr, name)

		assert.Equal(t, string(expected), string(encoded)+"\n", "%s must encode exactly as v4.2.2 encodes it", name)
	}
}

// wireFormatFixtures builds one fully populated value of every format that
// carries balance or account state across releases. The values are fixed so the
// encoding of each format can be compared byte for byte with the encoding the
// reference release produced for the same values.
func wireFormatFixtures() wireFormats {
	at := time.Date(2026, 10, 14, 12, 30, 0, 0, time.UTC)
	limit := "250.5"
	blocked := true
	alias := "@wire"
	portfolio := "00000000-0000-0000-0000-0000000000a5"
	route := "00000000-0000-0000-0000-0000000000a6"

	balance := mmodel.Balance{
		ID:             "00000000-0000-0000-0000-0000000000a1",
		OrganizationID: "00000000-0000-0000-0000-0000000000a2",
		LedgerID:       "00000000-0000-0000-0000-0000000000a3",
		AccountID:      "00000000-0000-0000-0000-0000000000a4",
		Alias:          alias,
		Key:            "default",
		AssetCode:      "BRL",
		Available:      decimal.RequireFromString("1500.25"),
		OnHold:         decimal.RequireFromString("10"),
		Version:        42,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Blocked:        true,
		Direction:      "credit",
		OverdraftUsed:  decimal.RequireFromString("3.5"),
		Settings: &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        &limit,
		},
		CreatedAt: at,
		UpdatedAt: at,
		DeletedAt: &at,
		Metadata:  map[string]any{"k": "v"},
	}

	account := mmodel.Account{
		ID:                 "00000000-0000-0000-0000-0000000000a4",
		Name:               "Wire",
		AssetCode:          "BRL",
		OrganizationID:     "00000000-0000-0000-0000-0000000000a2",
		LedgerID:           "00000000-0000-0000-0000-0000000000a3",
		PortfolioID:        &portfolio,
		Status:             mmodel.Status{Code: "ACTIVE"},
		Alias:              &alias,
		Type:               "deposit",
		Blocked:            &blocked,
		HolderCheckSkipped: true,
		CreatedAt:          at,
		UpdatedAt:          at,
		ClosedAt:           &at,
		Metadata:           map[string]any{"k": "v"},
	}

	snapshot := accounting.BalanceSnapshot{
		OrganizationID:        uuid.MustParse("00000000-0000-0000-0000-0000000000a2"),
		LedgerID:              uuid.MustParse("00000000-0000-0000-0000-0000000000a3"),
		BalanceRef:            "@wire#default",
		ID:                    uuid.MustParse("00000000-0000-0000-0000-0000000000a1"),
		AccountID:             uuid.MustParse("00000000-0000-0000-0000-0000000000a4"),
		AccountType:           "deposit",
		AssetCode:             "BRL",
		Alias:                 alias,
		Key:                   "default",
		Direction:             "credit",
		BalanceScope:          mmodel.BalanceScopeTransactional,
		Available:             decimal.RequireFromString("1500.25"),
		OnHold:                decimal.RequireFromString("10"),
		OverdraftUsed:         decimal.RequireFromString("3.5"),
		OverdraftLimit:        decimal.RequireFromString("250.5"),
		Version:               42,
		AllowSending:          true,
		AllowReceiving:        true,
		Blocked:               true,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
	}

	result := accounting.ExecutionResult{
		Movements: []accounting.Movement{{
			Ref:           "m1",
			TransactionID: uuid.MustParse("00000000-0000-0000-0000-0000000000a7"),
			PostingRef:    "p1",
			Role:          "primary",
			BalanceRef:    "@wire#default",
			Type:          accounting.PostingType("debit"),
			Amount:        decimal.RequireFromString("1"),
			Before:        accounting.BalanceState{Available: decimal.RequireFromString("1501.25"), Version: 41},
			After:         accounting.BalanceState{Available: decimal.RequireFromString("1500.25"), Version: 42},
		}},
		Final:              []accounting.BalanceSnapshot{snapshot},
		AppliedAtUnixMicro: at.UnixMicro(),
	}

	spec := OperationRecordSpec{
		TransactionID:     uuid.MustParse("00000000-0000-0000-0000-0000000000a7"),
		PostingRef:        "p1",
		OriginRef:         "source:0",
		BalanceRef:        "@wire#default",
		Role:              "primary",
		Ordinal:           1,
		Side:              "source",
		RowType:           "DEBIT",
		Direction:         "debit",
		Description:       "wire",
		RouteID:           &route,
		RouteCode:         "R1",
		RouteDescription:  "route",
		ChartOfAccounts:   "1.1",
		Metadata:          map[string]any{"k": "v"},
		Balance:           OperationBalanceContext(balance),
		RequestedAmount:   decimal.RequireFromString("1"),
		CompatibilityPath: "engine",
	}

	plan := TransactionCompletionPlan{
		FormatVersion:   1,
		TransactionID:   uuid.MustParse("00000000-0000-0000-0000-0000000000a7"),
		OrganizationID:  uuid.MustParse("00000000-0000-0000-0000-0000000000a2"),
		LedgerID:        uuid.MustParse("00000000-0000-0000-0000-0000000000a3"),
		ExecutionID:     uuid.MustParse("00000000-0000-0000-0000-0000000000a8"),
		TTL:             at,
		TransactionDate: at,
		OperationSpecs:  []OperationRecordSpec{spec},
	}

	record := TransactionCompletionRecord{
		FormatVersion:  1,
		OrganizationID: uuid.MustParse("00000000-0000-0000-0000-0000000000a2"),
		LedgerID:       uuid.MustParse("00000000-0000-0000-0000-0000000000a3"),
		ExecutionID:    uuid.MustParse("00000000-0000-0000-0000-0000000000a8"),
		TransactionID:  uuid.MustParse("00000000-0000-0000-0000-0000000000a7"),
		Payload:        "{}",
		Result:         result,
	}

	return wireFormats{balance: balance, account: account, snapshot: snapshot, result: result, plan: plan, record: record}
}

type wireFormats struct {
	balance  mmodel.Balance
	account  mmodel.Account
	snapshot accounting.BalanceSnapshot
	result   accounting.ExecutionResult
	plan     TransactionCompletionPlan
	record   TransactionCompletionRecord
}

func (f wireFormats) encodings() (map[string][]byte, error) {
	values := map[string]any{
		"balance_response.json":  f.balance,
		"account_response.json":  f.account,
		"balance_snapshot.json":  f.snapshot,
		"execution_result.json":  f.result,
		"completion_plan.json":   f.plan,
		"completion_record.json": f.record,
	}

	out := make(map[string][]byte, len(values))

	for name, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", name, err)
		}

		out[name] = encoded
	}

	return out, nil
}
