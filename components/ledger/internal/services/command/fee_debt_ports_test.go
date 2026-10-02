// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

func TestFeeDebtMetadataShapes(t *testing.T) {
	t.Parallel()

	debtID, opened := "1a2cf884-cf82-4520-9833-07d85c73bc14:from:0:debit", decimal.RequireFromString("70.0000000000000000001")
	assertFeeDebtMetadata(t, []FeeDebtOpening{{DebtID: debtID, DebtorRef: "@payer#default", CreditRef: "@fees#default", Opened: opened, Seq: 3}},
		`[{"debtId":"1a2cf884-cf82-4520-9833-07d85c73bc14:from:0:debit","debtorRef":"@payer#default",`+
			`"creditRef":"@fees#default","opened":"70.0000000000000000001","seq":"3"}]`)
	assertFeeDebtMetadata(t, []FeeDebtSettlement{{
		DebtID: debtID, DebtorRef: "@payer#default", CreditRef: "@fees#default", Amount: decimal.RequireFromString("12.5"), Opened: opened, Seq: 3,
	}}, `[{"debtId":"1a2cf884-cf82-4520-9833-07d85c73bc14:from:0:debit","debtorRef":"@payer#default",`+
		`"creditRef":"@fees#default","amount":"12.5","opened":"70.0000000000000000001","seq":"3"}]`)
}

// assertFeeDebtMetadata locks one fee-debt metadata value and proves it round-trips.
func assertFeeDebtMetadata[T any](t *testing.T, value []T, want string) {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) != want {
		t.Fatalf("metadata value = %s (%v), want %s", encoded, err, want)
	}

	var decoded []T
	if err := json.Unmarshal([]byte(want), &decoded); err != nil {
		t.Fatalf("decode metadata value: %v", err)
	}
	if reencoded, err := json.Marshal(decoded); err != nil || string(reencoded) != want {
		t.Fatalf("metadata round trip = %s (%v)", reencoded, err)
	}
}
