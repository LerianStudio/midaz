package transaction

import (
    "reflect"
    "testing"

    libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
    "github.com/shopspring/decimal"
)

func TestDSL_Parse_ValidExamples(t *testing.T) {
    cases := []struct{
        name string
        dsl  string
        want libTransaction.Transaction
    }{
        {
            name: "Simple transfer with chart-of-accounts",
            dsl:  "(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 3|0 (source (from @A :amount USD 3|0)) (distribute (to @B :amount USD 3|0))))",
            want: libTransaction.Transaction{
                ChartOfAccountsGroupName: "FUNDING",
                Send: libTransaction.Send{
                    Asset: "USD",
                    Value: decimal.RequireFromString("3"),
                    Source: libTransaction.Source{
                        Remaining: "",
                        From: []libTransaction.FromTo{{
                            AccountAlias: "@A",
                            Amount: &libTransaction.Amount{Asset: "USD", Value: decimal.RequireFromString("3")},
                            IsFrom: true,
                        }},
                    },
                    Distribute: libTransaction.Distribute{
                        Remaining: "",
                        To: []libTransaction.FromTo{{
                            AccountAlias: "@B",
                            Amount: &libTransaction.Amount{Asset: "USD", Value: decimal.RequireFromString("3")},
                            IsFrom: false,
                        }},
                    },
                },
            },
        },
        {
            name: "Pending with description and code",
            dsl:  "(transaction V1 (chart-of-accounts-group-name FUNDING) (description \"desc\") (code 00000000-0000-0000-0000-000000000000) (pending true) (send USD 1|0 (source (from @A :amount USD 1|0)) (distribute (to @B :amount USD 1|0))))",
            want: func() libTransaction.Transaction {
                return libTransaction.Transaction{
                    ChartOfAccountsGroupName: "FUNDING",
                    Description:              "desc",
                    Code:                     "00000000-0000-0000-0000-000000000000",
                    Pending:                  true,
                    Send: libTransaction.Send{
                        Asset: "USD",
                        Value: decimal.RequireFromString("1"),
                        Source: libTransaction.Source{From: []libTransaction.FromTo{{
                            AccountAlias: "@A",
                            Amount: &libTransaction.Amount{Asset: "USD", Value: decimal.RequireFromString("1")},
                            IsFrom: true,
                        }}},
                        Distribute: libTransaction.Distribute{To: []libTransaction.FromTo{{
                            AccountAlias: "@B",
                            Amount: &libTransaction.Amount{Asset: "USD", Value: decimal.RequireFromString("1")},
                            IsFrom: false,
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
            tx, ok := got.(libTransaction.Transaction)
            if !ok { t.Fatalf("unexpected parse type: %T", got) }
            if !reflect.DeepEqual(simplify(tx), simplify(tc.want)) {
                t.Fatalf("mismatch\nwant: %#v\n got: %#v", tc.want, tx)
            }
        })
    }
}

// simplify clears zero-value metadata fields that may be omitted by the parser implementation.
func simplify(t libTransaction.Transaction) libTransaction.Transaction {
    // Normalize metadata empties
    if t.Metadata != nil && len(t.Metadata) == 0 { t.Metadata = nil }
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
