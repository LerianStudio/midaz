// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"context"
	"testing"

	constant "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/shopspring/decimal"
)

// BenchmarkOperateBalances benchmarks the core balance operation function.
// This function is called for every account involved in a transaction.
func BenchmarkOperateBalances(b *testing.B) {
	scenarios := []struct {
		name   string
		amount Amount
	}{
		{
			name: "Debit_Created",
			amount: Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromInt(1000),
				Operation:       constant.DEBIT,
				TransactionType: constant.CREATED,
			},
		},
		{
			name: "Credit_Created",
			amount: Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromInt(1000),
				Operation:       constant.CREDIT,
				TransactionType: constant.CREATED,
			},
		},
		{
			name: "OnHold_Pending",
			amount: Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromInt(500),
				Operation:       constant.ONHOLD,
				TransactionType: constant.PENDING,
			},
		},
		{
			name: "Release_Canceled",
			amount: Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromInt(500),
				Operation:       constant.RELEASE,
				TransactionType: constant.CANCELED,
			},
		},
		{
			name: "Debit_Approved",
			amount: Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromInt(500),
				Operation:       constant.DEBIT,
				TransactionType: constant.APPROVED,
			},
		},
		{
			name: "Credit_Approved",
			amount: Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromInt(500),
				Operation:       constant.CREDIT,
				TransactionType: constant.APPROVED,
			},
		},
		{
			name: "LargeValue",
			amount: Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromFloat(999999999999.99),
				Operation:       constant.DEBIT,
				TransactionType: constant.CREATED,
			},
		},
		{
			name: "SmallValue",
			amount: Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromFloat(0.01),
				Operation:       constant.CREDIT,
				TransactionType: constant.CREATED,
			},
		},
	}

	balance := Balance{
		Available: decimal.NewFromInt(10000),
		OnHold:    decimal.NewFromInt(500),
		Version:   1,
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _ = OperateBalances(sc.amount, balance)
			}
		})
	}
}

// BenchmarkValidateFromBalances measures the source-side balance gate across three arms:
//
//   - Allowed:      not blocked, sending allowed -> returns nil (no error constructed).
//   - StatusDenied: not blocked, sending NOT allowed -> pre-existing 0024 rejection.
//   - Blocked:      account blocked -> new 0502 rejection.
//
// The account-block gate itself is a single bool branch placed before the allow-flag
// check. When it fires it constructs a business error via pkg.ValidateBusinessError
// exactly as the pre-existing 0024 path does, so "Blocked" and "StatusDenied" are
// expected to be within noise of each other. Their shared cost is the error-catalog
// construction that already existed before this feature — not something the block gate
// introduced. The gap to "Allowed" is the nil-vs-error path, identical for both
// rejection reasons. This is the evidence that the block gate adds no measurable cost
// beyond the rejection machinery already present on the restricted-status path.
func BenchmarkValidateFromBalances(b *testing.B) {
	from := map[string]Amount{
		"0#@account1#default": {Value: decimal.NewFromInt(50)},
	}

	scenarios := []struct {
		name    string
		balance *Balance
	}{
		{
			name: "Allowed",
			balance: &Balance{
				ID:           "123",
				Alias:        "@account1",
				Key:          "default",
				AssetCode:    "USD",
				Available:    decimal.NewFromInt(100),
				AllowSending: true,
				AccountType:  "internal",
			},
		},
		{
			name: "StatusDenied",
			balance: &Balance{
				ID:           "123",
				Alias:        "@account1",
				Key:          "default",
				AssetCode:    "USD",
				Available:    decimal.NewFromInt(100),
				AllowSending: false,
				AccountType:  "internal",
			},
		},
		{
			name: "Blocked",
			balance: &Balance{
				ID:             "123",
				Alias:          "@account1",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(100),
				AllowSending:   true,
				AccountBlocked: true,
				AccountType:    "internal",
			},
		},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = validateFromBalances(sc.balance, from, "USD", false)
			}
		})
	}
}

// BenchmarkCalculateTotal benchmarks the share/amount distribution calculation.
// Performance varies with the number of FromTo entries.
func BenchmarkCalculateTotal(b *testing.B) {
	makeFromTos := func(count int) []FromTo {
		fromTos := make([]FromTo, count)
		for i := 0; i < count; i++ {
			fromTos[i] = FromTo{
				AccountAlias: "@account" + string(rune('A'+i%26)),
				BalanceKey:   "default",
				Share: &Share{
					Percentage:             int64(100 / count),
					PercentageOfPercentage: 100,
				},
				IsFrom: true,
			}
		}
		// Last one takes the remainder
		fromTos[count-1].Share = nil
		fromTos[count-1].Remaining = "remaining"
		return fromTos
	}

	transaction := Transaction{
		Send: Send{
			Asset: "BRL",
			Value: decimal.NewFromInt(10000),
		},
	}

	sizes := []int{1, 2, 5, 10, 20}

	for _, size := range sizes {
		b.Run("FromTos_"+string(rune('0'+size/10))+string(rune('0'+size%10)), func(b *testing.B) {
			fromTos := makeFromTos(size)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				t := make(chan decimal.Decimal, 1)
				ft := make(chan map[string]Amount, 1)
				sd := make(chan []string, 1)
				or := make(chan map[string]string, 1)

				go CalculateTotal(fromTos, transaction, constant.CREATED, t, ft, sd, or)

				<-t
				<-ft
				<-sd
				<-or
			}
		})
	}
}

// BenchmarkValidateSendSourceAndDistribute benchmarks the main validation orchestrator.
// This is the hot path for every transaction creation.
func BenchmarkValidateSendSourceAndDistribute(b *testing.B) {
	scenarios := []struct {
		name        string
		transaction Transaction
	}{
		{
			name: "Simple_1to1",
			transaction: Transaction{
				Send: Send{
					Asset: "BRL",
					Value: decimal.NewFromInt(1000),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@source",
								BalanceKey:   "default",
								Amount: &Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(1000),
								},
								IsFrom: true,
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@dest",
								BalanceKey:   "default",
								Amount: &Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(1000),
								},
								IsFrom: false,
							},
						},
					},
				},
			},
		},
		{
			name: "Split_1to3_Shares",
			transaction: Transaction{
				Send: Send{
					Asset: "BRL",
					Value: decimal.NewFromInt(10000),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@source",
								BalanceKey:   "default",
								Amount: &Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(10000),
								},
								IsFrom: true,
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@dest1",
								BalanceKey:   "default",
								Share: &Share{
									Percentage:             50,
									PercentageOfPercentage: 100,
								},
								IsFrom: false,
							},
							{
								AccountAlias: "@dest2",
								BalanceKey:   "default",
								Share: &Share{
									Percentage:             30,
									PercentageOfPercentage: 100,
								},
								IsFrom: false,
							},
							{
								AccountAlias: "@dest3",
								BalanceKey:   "default",
								Remaining:    "remaining",
								IsFrom:       false,
							},
						},
					},
				},
			},
		},
		{
			name: "Complex_3to5_Mixed",
			transaction: Transaction{
				Send: Send{
					Asset: "BRL",
					Value: decimal.NewFromInt(100000),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@sourceA",
								BalanceKey:   "default",
								Share: &Share{
									Percentage:             40,
									PercentageOfPercentage: 100,
								},
								IsFrom: true,
							},
							{
								AccountAlias: "@sourceB",
								BalanceKey:   "default",
								Share: &Share{
									Percentage:             35,
									PercentageOfPercentage: 100,
								},
								IsFrom: true,
							},
							{
								AccountAlias: "@sourceC",
								BalanceKey:   "default",
								Remaining:    "remaining",
								IsFrom:       true,
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@dest1",
								BalanceKey:   "default",
								Share: &Share{
									Percentage:             30,
									PercentageOfPercentage: 100,
								},
								IsFrom: false,
							},
							{
								AccountAlias: "@dest2",
								BalanceKey:   "default",
								Share: &Share{
									Percentage:             25,
									PercentageOfPercentage: 100,
								},
								IsFrom: false,
							},
							{
								AccountAlias: "@dest3",
								BalanceKey:   "default",
								Share: &Share{
									Percentage:             20,
									PercentageOfPercentage: 100,
								},
								IsFrom: false,
							},
							{
								AccountAlias: "@dest4",
								BalanceKey:   "default",
								Share: &Share{
									Percentage:             15,
									PercentageOfPercentage: 100,
								},
								IsFrom: false,
							},
							{
								AccountAlias: "@dest5",
								BalanceKey:   "default",
								Remaining:    "remaining",
								IsFrom:       false,
							},
						},
					},
				},
			},
		},
	}

	ctx := context.Background()

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _ = ValidateSendSourceAndDistribute(ctx, sc.transaction, constant.CREATED)
			}
		})
	}
}

// BenchmarkDetermineOperation benchmarks the operation type determination.
// This is a simple but frequently called function.
func BenchmarkDetermineOperation(b *testing.B) {
	scenarios := []struct {
		name            string
		isPending       bool
		isFrom          bool
		transactionType string
	}{
		{"Pending_From_Pending", true, true, constant.PENDING},
		{"Pending_To_Pending", true, false, constant.PENDING},
		{"Pending_From_Canceled", true, true, constant.CANCELED},
		{"Pending_From_Approved", true, true, constant.APPROVED},
		{"Pending_To_Approved", true, false, constant.APPROVED},
		{"NotPending_From", false, true, constant.CREATED},
		{"NotPending_To", false, false, constant.CREATED},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _ = DetermineOperation(sc.isPending, sc.isFrom, sc.transactionType)
			}
		})
	}
}
