// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"testing"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
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
				_ = DetermineOperation(sc.isPending, sc.isFrom, sc.transactionType)
			}
		})
	}
}
