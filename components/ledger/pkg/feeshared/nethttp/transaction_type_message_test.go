// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// The invalid-transaction-type message carries TWO arguments — the option set and the
// offending field reference — and this package's ValidateStruct is one of the three places
// that supplies them. The other two are asserted elsewhere; without the assertions below an
// omitted or reordered argument here renders fmt's MISSING marker into a released message and
// nothing in the suite notices, because the only test that reaches this branch inspects the
// violated TAG and never the string ValidateStruct handed back.

// detailedSourceTypeMessage / detailedDestinationTypeMessage are the released renderings of the
// detailed transaction body's invalid-transaction-type rejection, captured byte for byte. The
// field reference is derived per side, so both sides are pinned: a call site that hardcoded one
// of them would satisfy only one row.
const (
	detailedSourceTypeMessage = "Only one transaction type ('amount', 'share', or 'remaining') " +
		"must be specified in the 'send.source.from' field for each entry. Please review your input and try again."

	detailedDestinationTypeMessage = "Only one transaction type ('amount', 'share', or 'remaining') " +
		"must be specified in the 'send.distribute.to' field for each entry. Please review your input and try again."
)

// usdAmount is a leg-level explicit amount in the shared asset.
func usdAmount(value int64) *transaction.Amount {
	return &transaction.Amount{Asset: "USD", Value: decimal.NewFromInt(value)}
}

// wholeShare is a leg-level share claiming the whole transaction total.
func wholeShare() *transaction.Share {
	return &transaction.Share{Percentage: 100}
}

// detailedTransaction assembles the canonical detailed transaction shape around the given
// source and destination legs, so each case below differs only by the value expressions its
// offending leg fills.
func detailedTransaction(from, to []transaction.FromTo) *transaction.Transaction {
	return &transaction.Transaction{
		Send: transaction.Send{
			Asset:      "USD",
			Value:      decimal.NewFromInt(100),
			Source:     transaction.Source{From: from},
			Distribute: transaction.Distribute{To: to},
		},
	}
}

// TestValidateStruct_SingleTransactionTypeRendersTheReleasedMessage drives the
// `singletransactiontype` violation through THIS package's ValidateStruct — the fees decode
// boundary — and reads the message it produced. The tag is only the trigger; what a client
// receives is the rendered string, and this is the only place on this surface where that string
// is asserted.
//
// A call site that dropped either argument, or passed them in the wrong order, still violates
// the same tag and still returns a 0072 ValidationError, so status-and-code assertions cannot
// see it. The byte-exact comparison can, and the MISSING/`%!` guards state the failure mode
// directly for a reader.
func TestValidateStruct_SingleTransactionTypeRendersTheReleasedMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tx   *transaction.Transaction
		want string
	}{
		{
			name: "a source leg filling amount and share",
			tx: detailedTransaction(
				[]transaction.FromTo{{AccountAlias: "@a", Amount: usdAmount(100), Share: wholeShare()}},
				[]transaction.FromTo{{AccountAlias: "@b", Amount: usdAmount(100)}},
			),
			want: detailedSourceTypeMessage,
		},
		{
			name: "a source leg filling amount and remaining",
			tx: detailedTransaction(
				[]transaction.FromTo{{AccountAlias: "@a", Amount: usdAmount(100), Remaining: "remaining"}},
				[]transaction.FromTo{{AccountAlias: "@b", Amount: usdAmount(100)}},
			),
			want: detailedSourceTypeMessage,
		},
		{
			name: "a source leg filling no value expression at all",
			tx: detailedTransaction(
				[]transaction.FromTo{{AccountAlias: "@a"}},
				[]transaction.FromTo{{AccountAlias: "@b", Amount: usdAmount(100)}},
			),
			want: detailedSourceTypeMessage,
		},
		{
			// Only the DESTINATION side is offending here, so the field reference in the
			// message has to come from the violated field rather than from a constant.
			name: "a destination leg filling share and remaining",
			tx: detailedTransaction(
				[]transaction.FromTo{{AccountAlias: "@a", Amount: usdAmount(100)}},
				[]transaction.FromTo{{AccountAlias: "@b", Share: wholeShare(), Remaining: "remaining"}},
			),
			want: detailedDestinationTypeMessage,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateStruct(tt.tx)
			require.Error(t, err, "a leg that does not fill exactly one value expression must be rejected")

			var vErr pkg.ValidationError
			require.ErrorAs(t, err, &vErr,
				"the rejection must be the invalid-transaction-type ValidationError, not the generic malformed-request error")

			assert.Equal(t, constant.ErrInvalidTransactionType.Error(), vErr.Code,
				"the fees decode boundary must answer with the canonical invalid-transaction-type code")
			assert.Equal(t, "Invalid Transaction Type", vErr.Title,
				"the title is shared across every surface that raises this code")

			assert.Equal(t, tt.want, vErr.Message,
				"this is released wire text: the option set and the offending field reference must both be supplied, in that order")
			assert.NotContains(t, vErr.Message, "%!",
				"a formatting marker means an argument was omitted or is of the wrong type")
			assert.NotContains(t, vErr.Message, "MISSING",
				"a MISSING marker means the call site supplied fewer arguments than the message has placeholders")
		})
	}
}
