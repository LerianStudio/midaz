// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

func TestFeeDebtOpeningsMetadataShape(t *testing.T) {
	t.Parallel()

	openings := []FeeDebtOpening{{
		DebtID: "1a2cf884-cf82-4520-9833-07d85c73bc14:from:0:debit", DebtorRef: "@payer#default",
		CreditRef: "@fees#default", Opened: decimal.RequireFromString("70.0000000000000000001"), Seq: 3,
	}}
	want := `[{"debtId":"1a2cf884-cf82-4520-9833-07d85c73bc14:from:0:debit","debtorRef":"@payer#default",` +
		`"creditRef":"@fees#default","opened":"70.0000000000000000001","seq":"3"}]`

	encoded, err := json.Marshal(openings)
	if err != nil || string(encoded) != want {
		t.Fatalf("feeDebtOpenings value = %s (%v), want %s", encoded, err, want)
	}

	var decoded []FeeDebtOpening
	if err := json.Unmarshal([]byte(want), &decoded); err != nil {
		t.Fatalf("decode feeDebtOpenings: %v", err)
	}
	if reencoded, err := json.Marshal(decoded); err != nil || string(reencoded) != want {
		t.Fatalf("feeDebtOpenings round trip = %s (%v)", reencoded, err)
	}
}
