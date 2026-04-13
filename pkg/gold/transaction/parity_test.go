// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/shopspring/decimal"
)

func TestDSL_Parse_ValidExamples(t *testing.T) {
	cases := []struct {
		name string
		dsl  string
		want mtransaction.Transaction
	}{
		{
			name: "Simple transfer with chart-of-accounts",
			dsl:  "(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 3|0 (source (from @A :amount USD 3|0)) (distribute (to @B :amount USD 3|0))))",
			want: mtransaction.Transaction{
				ChartOfAccountsGroupName: "FUNDING",
				Send: mtransaction.Send{
					Asset: "USD",
					Value: decimal.RequireFromString("3"),
					Source: mtransaction.Source{
						Remaining: "",
						From: []mtransaction.FromTo{{
							AccountAlias: "@A",
							Amount:       &mtransaction.Amount{Asset: "USD", Value: decimal.RequireFromString("3")},
							IsFrom:       true,
						}},
					},
					Distribute: mtransaction.Distribute{
						Remaining: "",
						To: []mtransaction.FromTo{{
							AccountAlias: "@B",
							Amount:       &mtransaction.Amount{Asset: "USD", Value: decimal.RequireFromString("3")},
							IsFrom:       false,
						}},
					},
				},
			},
		},
		{
			name: "Pending with description and code",
			dsl:  "(transaction V1 (chart-of-accounts-group-name FUNDING) (description \"desc\") (code 00000000-0000-0000-0000-000000000000) (pending true) (send USD 1|0 (source (from @A :amount USD 1|0)) (distribute (to @B :amount USD 1|0))))",
			want: func() mtransaction.Transaction {
				return mtransaction.Transaction{
					ChartOfAccountsGroupName: "FUNDING",
					Description:              "desc",
					Code:                     "00000000-0000-0000-0000-000000000000",
					Pending:                  true,
					Send: mtransaction.Send{
						Asset: "USD",
						Value: decimal.RequireFromString("1"),
						Source: mtransaction.Source{From: []mtransaction.FromTo{{
							AccountAlias: "@A",
							Amount:       &mtransaction.Amount{Asset: "USD", Value: decimal.RequireFromString("1")},
							IsFrom:       true,
						}}},
						Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
							AccountAlias: "@B",
							Amount:       &mtransaction.Amount{Asset: "USD", Value: decimal.RequireFromString("1")},
							IsFrom:       false,
						}}},
					},
				}
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.dsl); err != nil {
				t.Fatalf("validate failed: %+v", err)
			}
			got := Parse(tc.dsl)
			tx, ok := got.(mtransaction.Transaction)
			if !ok {
				t.Fatalf("unexpected parse type: %T", got)
			}
			if !reflect.DeepEqual(simplify(tx), simplify(tc.want)) {
				t.Fatalf("mismatch\nwant: %#v\n got: %#v", tc.want, tx)
			}
		})
	}
}

// simplify clears zero-value metadata fields that may be omitted by the parser implementation.
func simplify(t mtransaction.Transaction) mtransaction.Transaction {
	// Normalize metadata empties
	if t.Metadata != nil && len(t.Metadata) == 0 {
		t.Metadata = nil
	}
	// Normalize zero shares to nil for easier comparison
	for i := range t.Send.Source.From {
		if t.Send.Source.From[i].Share != nil && t.Send.Source.From[i].Share.Percentage == 0 && t.Send.Source.From[i].Share.PercentageOfPercentage == 0 {
			t.Send.Source.From[i].Share = nil
		}
	}
	for i := range t.Send.Distribute.To {
		if t.Send.Distribute.To[i].Share != nil && t.Send.Distribute.To[i].Share.Percentage == 0 && t.Send.Distribute.To[i].Share.PercentageOfPercentage == 0 {
			t.Send.Distribute.To[i].Share = nil
		}
	}
	return t
}
