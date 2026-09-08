// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestTranslateBalanceEngineTransactionPreservesOrderedLegIdentity(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	one := decimal.NewFromInt(1)
	two := decimal.NewFromInt(2)
	three := decimal.NewFromInt(3)

	input := BalanceEngineTranslationInput{
		TransactionID:     transactionID,
		Action:            constant.ActionDirect,
		TransactionStatus: constant.CREATED,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
			Asset: "USD",
			Source: mtransaction.Source{From: []mtransaction.FromTo{
				{AccountAlias: "0#@same#default", BalanceKey: "default", IsFrom: true, Description: "first", Metadata: map[string]any{"leg": "first"}},
				{AccountAlias: "1#@same#default", BalanceKey: "default", IsFrom: true, Description: "second", Metadata: map[string]any{"leg": "second"}},
			}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{
				{AccountAlias: "0#@destination#default", BalanceKey: "default", Description: "destination"},
			}},
		}},
		Validate: &mtransaction.Responses{
			From: map[string]mtransaction.Amount{
				"1#@same#default": {Asset: "USD", Value: two, Operation: constant.DEBIT, TransactionType: constant.CREATED},
				"0#@same#default": {Asset: "USD", Value: one, Operation: constant.DEBIT, TransactionType: constant.CREATED},
			},
			To: map[string]mtransaction.Amount{
				"0#@destination#default": {Asset: "USD", Value: three, Operation: constant.CREDIT, TransactionType: constant.CREATED},
			},
		},
		Balances: []*mmodel.Balance{
			translationBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@destination", "default"),
			translationBalance(organizationID, ledgerID, "55555555-5555-4555-8555-555555555555", "@same", "default"),
		},
	}

	transaction, projection, err := TranslateBalanceEngineTransaction(input)
	require.NoError(t, err)
	require.Len(t, transaction.Postings, 3)
	require.Len(t, projection, 3)

	assert.Equal(t, transactionID, transaction.ID)
	assert.Equal(t, []string{"from:0:debit", "from:1:debit", "to:0:credit"}, []string{
		transaction.Postings[0].Ref,
		transaction.Postings[1].Ref,
		transaction.Postings[2].Ref,
	})
	assert.Equal(t, []decimal.Decimal{one, two, three}, []decimal.Decimal{
		transaction.Postings[0].Amount,
		transaction.Postings[1].Amount,
		transaction.Postings[2].Amount,
	})
	assert.Equal(t, []engine.PostingType{engine.PostingDebit, engine.PostingDebit, engine.PostingCredit}, []engine.PostingType{
		transaction.Postings[0].Type,
		transaction.Postings[1].Type,
		transaction.Postings[2].Type,
	})
	assert.Equal(t, []string{"from:0", "from:1", "to:0"}, []string{
		projection[0].OriginRef,
		projection[1].OriginRef,
		projection[2].OriginRef,
	})
	assert.Equal(t, "first", projection[0].Description)
	assert.Equal(t, map[string]any{"leg": "first"}, projection[0].Metadata)
}

func TestTranslateBalanceEngineTransactionLifecyclePaths(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	tests := []struct {
		name, action, status string
		routed               bool
		overdraftAmount      decimal.Decimal
		postingTypes         []engine.PostingType
		rowTypes             []string
		directions           []string
		paths                []string
		drawPolicies         []engine.DrawPolicy
	}{
		{
			name: "direct", action: constant.ActionDirect, status: constant.CREATED,
			postingTypes: []engine.PostingType{engine.PostingDebit, engine.PostingCredit},
			rowTypes:     []string{constant.DEBIT, constant.CREDIT}, directions: []string{constant.DirectionDebit, constant.DirectionCredit},
			paths: []string{ProjectionStandard, ProjectionStandard}, drawPolicies: []engine.DrawPolicy{engine.DrawAllowed, engine.DrawForbidden},
		},
		{
			name: "revert", action: constant.ActionRevert, status: constant.CREATED, overdraftAmount: decimal.NewFromInt(7),
			postingTypes: []engine.PostingType{engine.PostingDebit, engine.PostingCredit},
			rowTypes:     []string{constant.DEBIT, constant.CREDIT}, directions: []string{constant.DirectionDebit, constant.DirectionCredit},
			paths: []string{ProjectionStandard, ProjectionStandard}, drawPolicies: []engine.DrawPolicy{engine.DrawAllowed, engine.DrawForbidden},
		},
		{
			name: "pending legacy", action: constant.ActionHold, status: constant.PENDING,
			postingTypes: []engine.PostingType{engine.PostingHold}, rowTypes: []string{constant.ONHOLD}, directions: []string{constant.DirectionDebit},
			paths: []string{ProjectionStandard}, drawPolicies: []engine.DrawPolicy{engine.DrawForbidden},
		},
		{
			name: "pending routed", action: constant.ActionHold, status: constant.PENDING, routed: true,
			postingTypes: []engine.PostingType{engine.PostingDebit, engine.PostingReserve}, rowTypes: []string{constant.DEBIT, constant.ONHOLD},
			directions: []string{constant.DirectionDebit, constant.DirectionCredit}, paths: []string{ProjectionValidatedHoldDebit, ProjectionValidatedHoldReserve},
			drawPolicies: []engine.DrawPolicy{engine.DrawForbidden, engine.DrawForbidden},
		},
		{
			name: "commit legacy", action: constant.ActionCommit, status: constant.APPROVED,
			postingTypes: []engine.PostingType{engine.PostingUnreserve, engine.PostingCredit}, rowTypes: []string{constant.DEBIT, constant.CREDIT},
			directions: []string{constant.DirectionDebit, constant.DirectionCredit}, paths: []string{ProjectionStandard, ProjectionStandard},
			drawPolicies: []engine.DrawPolicy{engine.DrawForbidden, engine.DrawForbidden},
		},
		{
			name: "commit routed", action: constant.ActionCommit, status: constant.APPROVED, routed: true,
			postingTypes: []engine.PostingType{engine.PostingUnreserve, engine.PostingCredit}, rowTypes: []string{constant.ONHOLD, constant.CREDIT},
			directions: []string{constant.DirectionDebit, constant.DirectionCredit}, paths: []string{ProjectionStandard, ProjectionStandard},
			drawPolicies: []engine.DrawPolicy{engine.DrawForbidden, engine.DrawForbidden},
		},
		{
			name: "cancel legacy", action: constant.ActionCancel, status: constant.CANCELED, overdraftAmount: decimal.NewFromInt(7),
			postingTypes: []engine.PostingType{engine.PostingRelease}, rowTypes: []string{constant.RELEASE}, directions: []string{constant.DirectionCredit},
			paths: []string{ProjectionStandard}, drawPolicies: []engine.DrawPolicy{engine.DrawForbidden},
		},
		{
			name: "cancel routed", action: constant.ActionCancel, status: constant.CANCELED, routed: true, overdraftAmount: decimal.NewFromInt(7),
			postingTypes: []engine.PostingType{engine.PostingUnreserve, engine.PostingCredit}, rowTypes: []string{constant.RELEASE, constant.CREDIT},
			directions: []string{constant.DirectionDebit, constant.DirectionCredit}, paths: []string{ProjectionValidatedCancelRelease, ProjectionValidatedCancelCredit},
			drawPolicies: []engine.DrawPolicy{engine.DrawForbidden, engine.DrawForbidden},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount := mtransaction.Amount{
				Asset: "USD", Value: decimal.NewFromInt(10), TransactionType: tt.status,
				RouteValidationEnabled: tt.routed, OverdraftAmount: tt.overdraftAmount,
			}
			input := BalanceEngineTranslationInput{
				TransactionID: transactionID, Action: tt.action, TransactionStatus: tt.status,
				TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
					Asset:      "USD",
					Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "0#@source#default", BalanceKey: "default", IsFrom: true}}},
					Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "0#@destination#default", BalanceKey: "default"}}},
				}},
				Validate: &mtransaction.Responses{
					From: map[string]mtransaction.Amount{"0#@source#default": amount},
					To:   map[string]mtransaction.Amount{"0#@destination#default": amount},
				},
				Balances: []*mmodel.Balance{
					translationBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@source", "default"),
					translationBalance(organizationID, ledgerID, "55555555-5555-4555-8555-555555555555", "@destination", "default"),
				},
			}

			transaction, projection, err := TranslateBalanceEngineTransaction(input)
			require.NoError(t, err)
			require.Len(t, transaction.Postings, len(tt.postingTypes))
			require.Len(t, projection, len(tt.postingTypes))

			for index := range tt.postingTypes {
				assert.Equal(t, tt.postingTypes[index], transaction.Postings[index].Type)
				assert.Equal(t, tt.drawPolicies[index], transaction.Postings[index].DrawPolicy)
				assert.Equal(t, tt.rowTypes[index], projection[index].RowType)
				assert.Equal(t, tt.directions[index], projection[index].Direction)
				assert.Equal(t, tt.paths[index], projection[index].CompatibilityPath)
				if tt.postingTypes[index] == engine.PostingCredit || tt.postingTypes[index] == engine.PostingRelease {
					assert.True(t, tt.overdraftAmount.Equal(transaction.Postings[index].OverdraftAmount))
				} else {
					assert.True(t, transaction.Postings[index].OverdraftAmount.IsZero())
				}
			}

			if tt.action == constant.ActionCancel {
				assert.Equal(t, tt.overdraftAmount, transaction.Postings[len(transaction.Postings)-1].OverdraftAmount)
			}
		})
	}
}

func TestTranslateBalanceEngineTransactionFreezesRoutedCompanionContext(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	routeID := "66666666-6666-4666-8666-666666666666"
	limit := "25"
	deletedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	metadata := map[string]any{"purpose": "primary"}
	primary := translationBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@source", "")
	primary.Metadata = map[string]any{"balance": "frozen"}
	primary.Settings = &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeTransactional, AllowOverdraft: true, OverdraftLimitEnabled: true, OverdraftLimit: &limit}
	primary.DeletedAt = &deletedAt
	companion := translationBalance(organizationID, ledgerID, "55555555-5555-4555-8555-555555555555", "@source", constant.OverdraftBalanceKey)
	companion.AccountID = primary.AccountID
	companion.Direction = constant.DirectionDebit
	companion.Settings = nil

	entries := &mmodel.AccountingEntries{
		Direct:    &mmodel.AccountingEntry{Debit: &mmodel.AccountingRubric{Code: "DIRECT-D", Description: "Direct debit"}},
		Overdraft: &mmodel.AccountingEntry{Debit: &mmodel.AccountingRubric{Code: "OD-D", Description: "Overdraft draw"}},
	}
	route := mmodel.OperationRouteCache{AccountingEntries: entries}
	input := BalanceEngineTranslationInput{
		TransactionID: transactionID, Action: constant.ActionDirect, TransactionStatus: constant.CREATED,
		RouteValidationEnabled: true,
		TransactionInput: mtransaction.Transaction{Description: "fallback", Send: mtransaction.Send{
			Asset: "USD", Source: mtransaction.Source{From: []mtransaction.FromTo{{
				AccountAlias: "0#@source#default", IsFrom: true, Description: "source", ChartOfAccounts: "1000", Metadata: metadata, RouteID: &routeID,
			}}},
		}},
		Validate: &mtransaction.Responses{
			From: map[string]mtransaction.Amount{"0#@source#default": {
				Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.DEBIT, TransactionType: constant.CREATED,
			}},
			OperationRoutesFrom: map[string]string{"0#@source#default": routeID},
		},
		Balances: []*mmodel.Balance{companion, primary},
		RouteCache: &mmodel.TransactionRouteCache{Actions: map[string]mmodel.ActionRouteCache{
			constant.ActionDirect:    {Source: map[string]mmodel.OperationRouteCache{routeID: route}},
			constant.ActionOverdraft: {Source: map[string]mmodel.OperationRouteCache{routeID: route}},
		}},
	}

	transaction, projection, err := TranslateBalanceEngineTransaction(input)
	require.NoError(t, err)
	require.Len(t, transaction.Postings, 1)
	require.Len(t, projection, 2)
	assert.Equal(t, engine.DrawAllowed, transaction.Postings[0].DrawPolicy, "ledger route mode, not the per-leg split flag, governs draw authorization")
	assert.Equal(t, "default", projection[0].Balance.Key)
	assert.Empty(t, primary.Key, "freezing a default key must not mutate the pool row")
	assert.Equal(t, "DIRECT-D", projection[0].RouteCode)
	assert.Equal(t, engine.RoleOverdraftCompanion, projection[1].Role)
	assert.Nil(t, projection[1].Balance.Settings)
	assert.Equal(t, routeID, *projection[1].RouteID)
	assert.Equal(t, "OD-D", projection[1].RouteCode)
	assert.Empty(t, projection[1].ChartOfAccounts)
	assert.Empty(t, projection[1].Metadata)

	metadata["purpose"] = "changed"
	primary.Metadata["balance"] = "changed"
	limit = "999"
	deletedAt = deletedAt.Add(time.Hour)
	assert.Equal(t, map[string]any{"purpose": "primary"}, projection[0].Metadata)
	assert.Equal(t, map[string]any{"balance": "frozen"}, projection[0].Balance.Metadata)
	assert.Equal(t, "25", *projection[0].Balance.Settings.OverdraftLimit)
	assert.Equal(t, time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC), *projection[0].Balance.DeletedAt)

	input.RouteCache = nil
	denied, _, err := TranslateBalanceEngineTransaction(input)
	require.NoError(t, err)
	assert.Equal(t, engine.DrawRouteDenied, denied.Postings[0].DrawPolicy)
}

func TestTranslateBalanceEngineTransactionRejectsNonExecutableAndInvalidAmounts(t *testing.T) {
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	input := BalanceEngineTranslationInput{TransactionID: transactionID, TransactionStatus: constant.NOTED}
	_, _, err := TranslateBalanceEngineTransaction(input)
	assert.ErrorIs(t, err, ErrBalanceEngineTransactionNotExecutable)

	input = BalanceEngineTranslationInput{
		TransactionID: transactionID, Action: constant.ActionDirect, TransactionStatus: constant.CREATED,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{Source: mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "0#@source#default"}}}}},
		Validate:         &mtransaction.Responses{From: map[string]mtransaction.Amount{"0#@source#default": {Value: decimal.NewFromInt(-1)}}},
		Balances:         []*mmodel.Balance{{Alias: "@source", Key: "default"}},
	}
	_, _, err = TranslateBalanceEngineTransaction(input)
	assert.True(t, errors.Is(err, ErrInvalidBalanceEngineTranslation))

	input.Validate.From["0#@source#default"] = mtransaction.Amount{
		Value: decimal.NewFromInt(1), OverdraftAmount: decimal.NewFromInt(-1),
	}
	_, _, err = TranslateBalanceEngineTransaction(input)
	assert.ErrorIs(t, err, ErrInvalidBalanceEngineTranslation)
}

func translationBalance(organizationID, ledgerID uuid.UUID, id, alias, key string) *mmodel.Balance {
	return &mmodel.Balance{
		ID: id, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		AccountID: uuid.NewSHA1(organizationID, []byte(id)).String(), Alias: alias, Key: key,
		AssetCode: "USD", Available: decimal.NewFromInt(100), Version: 1,
		AccountType: "deposit", AllowSending: true, AllowReceiving: true,
	}
}
