// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"testing"

	cn "github.com/LerianStudio/midaz/v3/pkg/constant"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTransactionInput_BuildTransaction(t *testing.T) {
	t.Parallel()

	transactionDate := &TransactionDate{}

	tests := []struct {
		name     string
		input    CreateTransactionInput
		validate func(t *testing.T, result *Transaction)
	}{
		{
			name: "minimal input without send",
			input: CreateTransactionInput{
				Description: "Minimal transaction",
			},
			validate: func(t *testing.T, result *Transaction) {
				assert.Equal(t, "Minimal transaction", result.Description)
				assert.Empty(t, result.ChartOfAccountsGroupName)
				assert.Empty(t, result.Code)
				assert.False(t, result.Pending)
				assert.Nil(t, result.Metadata)
			},
		},
		{
			name: "input with all fields except send",
			input: CreateTransactionInput{
				ChartOfAccountsGroupName: "FUNDING",
				Description:              "Full transaction",
				Code:                     "TX-001",
				Pending:                  true,
				Metadata:                 map[string]any{"key": "value"},
				Route:                    "route-123",
				TransactionDate:          transactionDate,
			},
			validate: func(t *testing.T, result *Transaction) {
				assert.Equal(t, "FUNDING", result.ChartOfAccountsGroupName)
				assert.Equal(t, "Full transaction", result.Description)
				assert.Equal(t, "TX-001", result.Code)
				assert.True(t, result.Pending)
				assert.Equal(t, map[string]any{"key": "value"}, result.Metadata)
				assert.Equal(t, "route-123", result.Route)
				assert.Equal(t, transactionDate, result.TransactionDate)
			},
		},
		{
			name: "input with send and from entries",
			input: CreateTransactionInput{
				Description: "Transaction with send",
				Send: Send{
					Asset: "USD",
					Value: decimal.NewFromInt(1000),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@sender1",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(600),
								},
								IsFrom: false, // Should be set to true by BuildTransaction
							},
							{
								AccountAlias: "@sender2",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(400),
								},
								IsFrom: false, // Should be set to true by BuildTransaction
							},
						},
					},
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@receiver",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(1000),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *Transaction) {
				assert.Equal(t, "Transaction with send", result.Description)
				assert.Equal(t, "USD", result.Send.Asset)
				assert.True(t, result.Send.Value.Equal(decimal.NewFromInt(1000)))

				// Verify IsFrom is set to true for all From entries
				require.Len(t, result.Send.Source.From, 2)
				for _, from := range result.Send.Source.From {
					assert.True(t, from.IsFrom, "IsFrom should be true for From entries")
				}

				require.Len(t, result.Send.Distribute.To, 1)
				assert.Equal(t, "@receiver", result.Send.Distribute.To[0].AccountAlias)
			},
		},
		{
			name: "input with zero-value send",
			input: CreateTransactionInput{
				Description: "No send",
			},
			validate: func(t *testing.T, result *Transaction) {
				assert.Equal(t, "No send", result.Description)
				// Send should be empty/zero value
				assert.True(t, result.Send.Value.IsZero())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.input.BuildTransaction()

			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

func TestCreateTransactionInflowInput_BuildInflowEntry(t *testing.T) {
	t.Parallel()

	transactionDate := &TransactionDate{}

	tests := []struct {
		name     string
		input    CreateTransactionInflowInput
		validate func(t *testing.T, result *Transaction)
	}{
		{
			name: "minimal inflow",
			input: CreateTransactionInflowInput{
				Description: "Minimal inflow",
				Send: SendInflow{
					Asset: "USD",
					Value: decimal.NewFromInt(500),
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@receiver",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(500),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *Transaction) {
				assert.Equal(t, "Minimal inflow", result.Description)
				assert.Equal(t, "USD", result.Send.Asset)
				assert.True(t, result.Send.Value.Equal(decimal.NewFromInt(500)))

				// Verify external account is created as source
				require.Len(t, result.Send.Source.From, 1)
				from := result.Send.Source.From[0]
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"USD", from.AccountAlias)
				assert.True(t, from.IsFrom)
				require.NotNil(t, from.Amount)
				assert.Equal(t, "USD", from.Amount.Asset)
				assert.True(t, from.Amount.Value.Equal(decimal.NewFromInt(500)))

				// Verify distribute is passed through
				require.Len(t, result.Send.Distribute.To, 1)
				assert.Equal(t, "@receiver", result.Send.Distribute.To[0].AccountAlias)
			},
		},
		{
			name: "inflow with all fields",
			input: CreateTransactionInflowInput{
				ChartOfAccountsGroupName: "FUNDING",
				Description:              "Full inflow",
				Code:                     "INF-001",
				Metadata:                 map[string]any{"source": "external"},
				Route:                    "inflow-route",
				TransactionDate:          transactionDate,
				Send: SendInflow{
					Asset: "BRL",
					Value: decimal.NewFromInt(1000),
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias:    "@account1",
								Description:     "Credit to account1",
								ChartOfAccounts: "4001",
								Amount: &Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(600),
								},
							},
							{
								AccountAlias:    "@account2",
								Description:     "Credit to account2",
								ChartOfAccounts: "4002",
								Amount: &Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(400),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *Transaction) {
				assert.Equal(t, "FUNDING", result.ChartOfAccountsGroupName)
				assert.Equal(t, "Full inflow", result.Description)
				assert.Equal(t, "INF-001", result.Code)
				assert.Equal(t, map[string]any{"source": "external"}, result.Metadata)
				assert.Equal(t, "inflow-route", result.Route)
				assert.Equal(t, transactionDate, result.TransactionDate)

				// Verify external account prefix
				require.Len(t, result.Send.Source.From, 1)
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"BRL", result.Send.Source.From[0].AccountAlias)

				// Verify multiple To entries
				require.Len(t, result.Send.Distribute.To, 2)
			},
		},
		{
			name: "inflow with different asset",
			input: CreateTransactionInflowInput{
				Description: "EUR inflow",
				Send: SendInflow{
					Asset: "EUR",
					Value: decimal.NewFromInt(250),
					Distribute: Distribute{
						To: []FromTo{
							{
								AccountAlias: "@euro_account",
								Amount: &Amount{
									Asset: "EUR",
									Value: decimal.NewFromInt(250),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *Transaction) {
				// Verify external account uses correct asset
				require.Len(t, result.Send.Source.From, 1)
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"EUR", result.Send.Source.From[0].AccountAlias)
				assert.Equal(t, "EUR", result.Send.Source.From[0].Amount.Asset)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.input.BuildInflowEntry()

			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

func TestCreateTransactionOutflowInput_BuildOutflowEntry(t *testing.T) {
	t.Parallel()

	transactionDate := &TransactionDate{}

	tests := []struct {
		name     string
		input    CreateTransactionOutflowInput
		validate func(t *testing.T, result *Transaction)
	}{
		{
			name: "minimal outflow",
			input: CreateTransactionOutflowInput{
				Description: "Minimal outflow",
				Send: SendOutflow{
					Asset: "USD",
					Value: decimal.NewFromInt(500),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@sender",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(500),
								},
								IsFrom: false, // Should be set to true by BuildOutflowEntry
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *Transaction) {
				assert.Equal(t, "Minimal outflow", result.Description)
				assert.Equal(t, "USD", result.Send.Asset)
				assert.True(t, result.Send.Value.Equal(decimal.NewFromInt(500)))

				// Verify external account is created as destination
				require.Len(t, result.Send.Distribute.To, 1)
				to := result.Send.Distribute.To[0]
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"USD", to.AccountAlias)
				assert.False(t, to.IsFrom)
				require.NotNil(t, to.Amount)
				assert.Equal(t, "USD", to.Amount.Asset)

				// Verify source From entries have IsFrom=true
				require.Len(t, result.Send.Source.From, 1)
				assert.True(t, result.Send.Source.From[0].IsFrom, "From entries should have IsFrom=true")
			},
		},
		{
			name: "outflow with all fields",
			input: CreateTransactionOutflowInput{
				ChartOfAccountsGroupName: "WITHDRAWAL",
				Description:              "Full outflow",
				Code:                     "OUT-001",
				Pending:                  true,
				Metadata:                 map[string]any{"destination": "external"},
				Route:                    "outflow-route",
				TransactionDate:          transactionDate,
				Send: SendOutflow{
					Asset: "BRL",
					Value: decimal.NewFromInt(1000),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias:    "@account1",
								Description:     "Debit from account1",
								ChartOfAccounts: "5001",
								Amount: &Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(600),
								},
							},
							{
								AccountAlias:    "@account2",
								Description:     "Debit from account2",
								ChartOfAccounts: "5002",
								Amount: &Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(400),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *Transaction) {
				assert.Equal(t, "WITHDRAWAL", result.ChartOfAccountsGroupName)
				assert.Equal(t, "Full outflow", result.Description)
				assert.Equal(t, "OUT-001", result.Code)
				assert.True(t, result.Pending, "Pending flag should be preserved")
				assert.Equal(t, map[string]any{"destination": "external"}, result.Metadata)
				assert.Equal(t, "outflow-route", result.Route)
				assert.Equal(t, transactionDate, result.TransactionDate)

				// Verify external account prefix in To
				require.Len(t, result.Send.Distribute.To, 1)
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"BRL", result.Send.Distribute.To[0].AccountAlias)

				// Verify multiple From entries have IsFrom=true
				require.Len(t, result.Send.Source.From, 2)
				for _, from := range result.Send.Source.From {
					assert.True(t, from.IsFrom, "All From entries should have IsFrom=true")
				}
			},
		},
		{
			name: "outflow with different asset",
			input: CreateTransactionOutflowInput{
				Description: "EUR outflow",
				Send: SendOutflow{
					Asset: "EUR",
					Value: decimal.NewFromInt(250),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@euro_account",
								Amount: &Amount{
									Asset: "EUR",
									Value: decimal.NewFromInt(250),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *Transaction) {
				// Verify external account uses correct asset
				require.Len(t, result.Send.Distribute.To, 1)
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"EUR", result.Send.Distribute.To[0].AccountAlias)
				assert.Equal(t, "EUR", result.Send.Distribute.To[0].Amount.Asset)
			},
		},
		{
			name: "outflow not pending",
			input: CreateTransactionOutflowInput{
				Description: "Non-pending outflow",
				Pending:     false,
				Send: SendOutflow{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: Source{
						From: []FromTo{
							{
								AccountAlias: "@account",
								Amount: &Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(100),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *Transaction) {
				assert.False(t, result.Pending, "Pending flag should be false")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.input.BuildOutflowEntry()

			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}
