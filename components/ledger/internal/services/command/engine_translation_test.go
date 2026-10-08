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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestTranslateEngineTransactionPreservesOrderedLegIdentity(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	one := decimal.NewFromInt(1)
	two := decimal.NewFromInt(2)
	three := decimal.NewFromInt(3)

	input := EngineTranslationInput{
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

	transaction, projection, err := TranslateEngineTransaction(input)
	require.NoError(t, err)
	require.Len(t, transaction.Postings, 3)
	require.Len(t, transaction.BalanceRequirements, 3)
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
	assert.Equal(t, []accounting.PostingType{accounting.PostingDebit, accounting.PostingDebit, accounting.PostingCredit}, []accounting.PostingType{
		transaction.Postings[0].Type,
		transaction.Postings[1].Type,
		transaction.Postings[2].Type,
	})
	assert.Equal(t, []accounting.BalancePermission{accounting.BalancePermissionSend, accounting.BalancePermissionSend, accounting.BalancePermissionReceive}, []accounting.BalancePermission{
		transaction.BalanceRequirements[0].Permission,
		transaction.BalanceRequirements[1].Permission,
		transaction.BalanceRequirements[2].Permission,
	})
	assert.Equal(t, []string{"from:0", "from:1", "to:0"}, []string{
		projection[0].OriginRef,
		projection[1].OriginRef,
		projection[2].OriginRef,
	})
	assert.Equal(t, "first", projection[0].Description)
	assert.Equal(t, map[string]any{"leg": "first"}, projection[0].Metadata)
}

func TestTranslateEngineTransactionPreservesOperationTypeOverrideInProjection(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	amount := mtransaction.Amount{
		Asset: "USD", Value: decimal.NewFromInt(10), TransactionType: constant.CREATED,
	}
	input := EngineTranslationInput{
		TransactionID:     uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		Action:            constant.ActionDirect,
		TransactionStatus: constant.CREATED,
		TransactionInput: mtransaction.Transaction{
			OperationTypeOverride: constant.BLOCK,
			Send: mtransaction.Send{
				Asset:      "USD",
				Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "0#@source#default", BalanceKey: "default", IsFrom: true}}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "0#@destination#default", BalanceKey: "default"}}},
			},
		},
		Validate: &mtransaction.Responses{
			From: map[string]mtransaction.Amount{"0#@source#default": amount},
			To:   map[string]mtransaction.Amount{"0#@destination#default": amount},
		},
		Balances: []*mmodel.Balance{
			translationBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@source", "default"),
			translationBalance(organizationID, ledgerID, "55555555-5555-4555-8555-555555555555", "@destination", "default"),
		},
	}

	transaction, projection, err := TranslateEngineTransaction(input)
	require.NoError(t, err)
	require.Len(t, transaction.Postings, 2)
	require.Len(t, projection, 2)

	assert.Equal(t, accounting.PostingDebit, transaction.Postings[0].Type)
	assert.Equal(t, accounting.PostingCredit, transaction.Postings[1].Type)
	assert.Equal(t, constant.BLOCK, projection[0].RowType)
	assert.Equal(t, constant.BLOCK, projection[1].RowType)
	assert.Equal(t, constant.DirectionDebit, projection[0].Direction)
	assert.Equal(t, constant.DirectionCredit, projection[1].Direction)
}

func TestTranslateEngineTransactionOverdraftRulesForBlockAndUnblock(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	tests := []struct {
		name               string
		override           string
		wantDrawPolicy     accounting.DrawPolicy
		wantRepayForbidden bool
		wantCreditCap      decimal.Decimal
		wantRowTypes       []string
		wantRoles          []string
	}{
		{
			name:               "block never draws nor repays and builds no companion",
			override:           constant.BLOCK,
			wantDrawPolicy:     accounting.DrawForbidden,
			wantRepayForbidden: true,
			wantCreditCap:      decimal.Zero,
			wantRowTypes:       []string{constant.BLOCK, constant.BLOCK},
			wantRoles:          []string{accounting.RolePrimary, accounting.RolePrimary},
		},
		{
			name:           "unblock keeps overdraft arithmetic and books its companions as overdraft",
			override:       constant.UNBLOCK,
			wantDrawPolicy: accounting.DrawAllowed,
			wantCreditCap:  decimal.NewFromInt(5),
			wantRowTypes:   []string{constant.UNBLOCK, constant.OVERDRAFT, constant.UNBLOCK, constant.OVERDRAFT},
			wantRoles:      []string{accounting.RolePrimary, accounting.RoleOverdraftCompanion, accounting.RolePrimary, accounting.RoleOverdraftCompanion},
		},
		{
			name:           "direct keeps overdraft arithmetic and overdraft companions",
			wantDrawPolicy: accounting.DrawAllowed,
			wantCreditCap:  decimal.NewFromInt(5),
			wantRowTypes:   []string{constant.DEBIT, constant.OVERDRAFT, constant.CREDIT, constant.OVERDRAFT},
			wantRoles:      []string{accounting.RolePrimary, accounting.RoleOverdraftCompanion, accounting.RolePrimary, accounting.RoleOverdraftCompanion},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := translationBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@source", "default")
			sourceDebt := translationBalance(organizationID, ledgerID, "66666666-6666-4666-8666-666666666666", "@source", constant.OverdraftBalanceKey)
			sourceDebt.AccountID = source.AccountID
			destination := translationBalance(organizationID, ledgerID, "55555555-5555-4555-8555-555555555555", "@destination", "default")
			destinationDebt := translationBalance(organizationID, ledgerID, "77777777-7777-4777-8777-777777777777", "@destination", constant.OverdraftBalanceKey)
			destinationDebt.AccountID = destination.AccountID

			debit := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), TransactionType: constant.CREATED}
			credit := debit
			credit.OverdraftAmount = decimal.NewFromInt(5)

			input := EngineTranslationInput{
				TransactionID:     uuid.MustParse("33333333-3333-4333-8333-333333333333"),
				Action:            constant.ActionDirect,
				TransactionStatus: constant.CREATED,
				TransactionInput: mtransaction.Transaction{
					OperationTypeOverride: tt.override,
					Send: mtransaction.Send{
						Asset:      "USD",
						Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "0#@source#default", BalanceKey: "default", IsFrom: true}}},
						Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "0#@destination#default", BalanceKey: "default"}}},
					},
				},
				Validate: &mtransaction.Responses{
					From: map[string]mtransaction.Amount{"0#@source#default": debit},
					To:   map[string]mtransaction.Amount{"0#@destination#default": credit},
				},
				Balances: []*mmodel.Balance{source, sourceDebt, destination, destinationDebt},
			}

			transaction, projection, err := TranslateEngineTransaction(input)
			require.NoError(t, err)
			require.Len(t, transaction.Postings, 2)

			debitPosting, creditPosting := transaction.Postings[0], transaction.Postings[1]
			assert.Equal(t, accounting.PostingDebit, debitPosting.Type)
			assert.Equal(t, tt.wantDrawPolicy, debitPosting.DrawPolicy)
			assert.False(t, debitPosting.RepayForbidden)
			assert.Equal(t, accounting.PostingCredit, creditPosting.Type)
			assert.Equal(t, tt.wantRepayForbidden, creditPosting.RepayForbidden)
			assert.True(t, tt.wantCreditCap.Equal(creditPosting.OverdraftAmount), "credit overdraft cap: got %s", creditPosting.OverdraftAmount)

			rowTypes := make([]string, 0, len(projection))
			roles := make([]string, 0, len(projection))

			for _, spec := range projection {
				rowTypes = append(rowTypes, spec.RowType)
				roles = append(roles, spec.Role)
			}

			assert.Equal(t, tt.wantRowTypes, rowTypes)
			assert.Equal(t, tt.wantRoles, roles)
		})
	}
}

func TestTranslateEngineTransactionBlockUnblockRubric(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	sourceRouteID := uuid.MustParse("88888888-8888-4888-8888-888888888888")
	destinationRouteID := uuid.MustParse("99999999-9999-4999-8999-999999999999")

	debitRubric := func(code string) *mmodel.AccountingEntry {
		return &mmodel.AccountingEntry{Debit: &mmodel.AccountingRubric{Code: code, Description: code + " description"}}
	}
	creditRubric := func(code string) *mmodel.AccountingEntry {
		return &mmodel.AccountingEntry{Credit: &mmodel.AccountingRubric{Code: code, Description: code + " description"}}
	}
	bothRubrics := func(debit, credit string) *mmodel.AccountingEntry {
		return &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: debit, Description: debit + " description"},
			Credit: &mmodel.AccountingRubric{Code: credit, Description: credit + " description"},
		}
	}

	dedicatedSource := &mmodel.AccountingEntries{Direct: debitRubric("D-SRC"), Block: debitRubric("B-SRC"), Unblock: debitRubric("U-SRC")}
	dedicatedDestination := &mmodel.AccountingEntries{Direct: creditRubric("D-DST"), Block: creditRubric("B-DST"), Unblock: creditRubric("U-DST")}
	directOnlySource := &mmodel.AccountingEntries{Direct: debitRubric("D-SRC")}
	directOnlyDestination := &mmodel.AccountingEntries{Direct: creditRubric("D-DST")}

	tests := []struct {
		name                   string
		override               string
		sourceEntries          *mmodel.AccountingEntries
		destinationEntries     *mmodel.AccountingEntries
		destinationInOverdraft bool
		wantRowTypes           []string
		wantRouteCodes         []string
	}{
		{
			name:     "block books the block rubric when the route configures it",
			override: constant.BLOCK, sourceEntries: dedicatedSource, destinationEntries: dedicatedDestination,
			wantRowTypes:   []string{constant.BLOCK, constant.BLOCK},
			wantRouteCodes: []string{"B-SRC", "B-DST"},
		},
		{
			name:     "unblock books the unblock rubric when the route configures it",
			override: constant.UNBLOCK, sourceEntries: dedicatedSource, destinationEntries: dedicatedDestination,
			wantRowTypes:   []string{constant.UNBLOCK, constant.UNBLOCK},
			wantRouteCodes: []string{"U-SRC", "U-DST"},
		},
		{
			name:     "block falls back to the direct rubric without a block entry",
			override: constant.BLOCK, sourceEntries: directOnlySource, destinationEntries: directOnlyDestination,
			wantRowTypes:   []string{constant.BLOCK, constant.BLOCK},
			wantRouteCodes: []string{"D-SRC", "D-DST"},
		},
		{
			name:     "unblock falls back to the direct rubric without an unblock entry",
			override: constant.UNBLOCK, sourceEntries: directOnlySource, destinationEntries: directOnlyDestination,
			wantRowTypes:   []string{constant.UNBLOCK, constant.UNBLOCK},
			wantRouteCodes: []string{"D-SRC", "D-DST"},
		},
		{
			name:          "direct keeps the direct rubric even when block and unblock entries exist",
			sourceEntries: dedicatedSource, destinationEntries: dedicatedDestination,
			wantRowTypes:   []string{constant.DEBIT, constant.CREDIT},
			wantRouteCodes: []string{"D-SRC", "D-DST"},
		},
		{
			name:     "unblock repaying overdraft books unblock on the primary and overdraft on the companion",
			override: constant.UNBLOCK, sourceEntries: dedicatedSource,
			destinationEntries: &mmodel.AccountingEntries{
				Direct: creditRubric("D-DST"), Unblock: creditRubric("U-DST"), Overdraft: bothRubrics("O-DST-D", "O-DST-C"),
			},
			destinationInOverdraft: true,
			wantRowTypes:           []string{constant.UNBLOCK, constant.UNBLOCK, constant.OVERDRAFT},
			wantRouteCodes:         []string{"U-SRC", "U-DST", "O-DST-C"},
		},
		{
			name:     "a cross-ledger bridge route keeps its bridge rubric under a block",
			override: constant.BLOCK,
			sourceEntries: &mmodel.AccountingEntries{
				Direct: debitRubric("D-SRC"), Block: debitRubric("B-SRC"), CrossLedger: debitRubric("X-SRC"),
			},
			destinationEntries: dedicatedDestination,
			wantRowTypes:       []string{constant.BLOCK, constant.BLOCK},
			wantRouteCodes:     []string{"X-SRC", "B-DST"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := mmodel.TransactionRoute{OperationRoutes: []mmodel.OperationRoute{
				{ID: sourceRouteID, OperationType: "source", AccountingEntries: tt.sourceEntries},
				{ID: destinationRouteID, OperationType: "destination", AccountingEntries: tt.destinationEntries},
			}}
			routeCache := route.ToCache()

			source := translationBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@source", "default")
			destination := translationBalance(organizationID, ledgerID, "55555555-5555-4555-8555-555555555555", "@destination", "default")
			balances := []*mmodel.Balance{source, destination}

			debit := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), TransactionType: constant.CREATED}
			credit := debit

			if tt.destinationInOverdraft {
				destinationDebt := translationBalance(organizationID, ledgerID, "77777777-7777-4777-8777-777777777777", "@destination", constant.OverdraftBalanceKey)
				destinationDebt.AccountID = destination.AccountID
				balances = append(balances, destinationDebt)
				credit.OverdraftAmount = decimal.NewFromInt(5)
			}

			input := EngineTranslationInput{
				TransactionID:     uuid.MustParse("33333333-3333-4333-8333-333333333333"),
				Action:            constant.ActionDirect,
				TransactionStatus: constant.CREATED,
				TransactionInput: mtransaction.Transaction{
					OperationTypeOverride: tt.override,
					Send: mtransaction.Send{
						Asset:      "USD",
						Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "0#@source#default", BalanceKey: "default", IsFrom: true}}},
						Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "0#@destination#default", BalanceKey: "default"}}},
					},
				},
				Validate: &mtransaction.Responses{
					From:                map[string]mtransaction.Amount{"0#@source#default": debit},
					To:                  map[string]mtransaction.Amount{"0#@destination#default": credit},
					OperationRoutesFrom: map[string]string{"0#@source#default": sourceRouteID.String()},
					OperationRoutesTo:   map[string]string{"0#@destination#default": destinationRouteID.String()},
				},
				Balances:   balances,
				RouteCache: &routeCache,
			}

			_, projection, err := TranslateEngineTransaction(input)
			require.NoError(t, err)

			rowTypes := make([]string, 0, len(projection))
			routeCodes := make([]string, 0, len(projection))

			for _, spec := range projection {
				rowTypes = append(rowTypes, spec.RowType)
				routeCodes = append(routeCodes, spec.RouteCode)
				assert.Equal(t, spec.RouteCode+" description", spec.RouteDescription, "rubric description must come from the same entry as its code")
			}

			assert.Equal(t, tt.wantRowTypes, rowTypes)
			assert.Equal(t, tt.wantRouteCodes, routeCodes)
		})
	}
}

func TestTranslateEngineTransactionLifecyclePaths(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	tests := []struct {
		name, action, status string
		routeValidation      bool
		overdraftAmount      decimal.Decimal
		postingTypes         []accounting.PostingType
		rowTypes             []string
		directions           []string
		paths                []string
		drawPolicies         []accounting.DrawPolicy
	}{
		{
			name: "direct", action: constant.ActionDirect, status: constant.CREATED,
			postingTypes: []accounting.PostingType{accounting.PostingDebit, accounting.PostingCredit},
			rowTypes:     []string{constant.DEBIT, constant.CREDIT}, directions: []string{constant.DirectionDebit, constant.DirectionCredit},
			paths: []string{OperationRecordStandard, OperationRecordStandard}, drawPolicies: []accounting.DrawPolicy{accounting.DrawAllowed, accounting.DrawForbidden},
		},
		{
			name: "revert", action: constant.ActionRevert, status: constant.CREATED, overdraftAmount: decimal.NewFromInt(7),
			postingTypes: []accounting.PostingType{accounting.PostingDebit, accounting.PostingCredit},
			rowTypes:     []string{constant.DEBIT, constant.CREDIT}, directions: []string{constant.DirectionDebit, constant.DirectionCredit},
			paths: []string{OperationRecordStandard, OperationRecordStandard}, drawPolicies: []accounting.DrawPolicy{accounting.DrawAllowed, accounting.DrawForbidden},
		},
		{
			name: "pending without route validation", action: constant.ActionHold, status: constant.PENDING,
			postingTypes: []accounting.PostingType{accounting.PostingHold}, rowTypes: []string{constant.ONHOLD}, directions: []string{constant.DirectionDebit},
			paths: []string{OperationRecordStandard}, drawPolicies: []accounting.DrawPolicy{accounting.DrawForbidden},
		},
		{
			name: "pending with route validation", action: constant.ActionHold, status: constant.PENDING, routeValidation: true,
			postingTypes: []accounting.PostingType{accounting.PostingDebit, accounting.PostingReserve}, rowTypes: []string{constant.DEBIT, constant.ONHOLD},
			directions: []string{constant.DirectionDebit, constant.DirectionCredit}, paths: []string{OperationRecordValidatedHoldDebit, OperationRecordValidatedHoldReserve},
			drawPolicies: []accounting.DrawPolicy{accounting.DrawForbidden, accounting.DrawForbidden},
		},
		{
			name: "commit without route validation", action: constant.ActionCommit, status: constant.APPROVED,
			postingTypes: []accounting.PostingType{accounting.PostingUnreserve, accounting.PostingCredit}, rowTypes: []string{constant.DEBIT, constant.CREDIT},
			directions: []string{constant.DirectionDebit, constant.DirectionCredit}, paths: []string{OperationRecordStandard, OperationRecordStandard},
			drawPolicies: []accounting.DrawPolicy{accounting.DrawForbidden, accounting.DrawForbidden},
		},
		{
			name: "commit with route validation", action: constant.ActionCommit, status: constant.APPROVED, routeValidation: true,
			postingTypes: []accounting.PostingType{accounting.PostingUnreserve, accounting.PostingCredit}, rowTypes: []string{constant.ONHOLD, constant.CREDIT},
			directions: []string{constant.DirectionDebit, constant.DirectionCredit}, paths: []string{OperationRecordStandard, OperationRecordStandard},
			drawPolicies: []accounting.DrawPolicy{accounting.DrawForbidden, accounting.DrawForbidden},
		},
		{
			name: "cancel without route validation", action: constant.ActionCancel, status: constant.CANCELED, overdraftAmount: decimal.NewFromInt(7),
			postingTypes: []accounting.PostingType{accounting.PostingRelease}, rowTypes: []string{constant.RELEASE}, directions: []string{constant.DirectionCredit},
			paths: []string{OperationRecordStandard}, drawPolicies: []accounting.DrawPolicy{accounting.DrawForbidden},
		},
		{
			name: "cancel with route validation", action: constant.ActionCancel, status: constant.CANCELED, routeValidation: true, overdraftAmount: decimal.NewFromInt(7),
			postingTypes: []accounting.PostingType{accounting.PostingUnreserve, accounting.PostingCredit}, rowTypes: []string{constant.RELEASE, constant.CREDIT},
			directions: []string{constant.DirectionDebit, constant.DirectionCredit}, paths: []string{OperationRecordValidatedCancelRelease, OperationRecordValidatedCancelCredit},
			drawPolicies: []accounting.DrawPolicy{accounting.DrawForbidden, accounting.DrawForbidden},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount := mtransaction.Amount{
				Asset: "USD", Value: decimal.NewFromInt(10), TransactionType: tt.status,
				RouteValidationEnabled: tt.routeValidation, OverdraftAmount: tt.overdraftAmount,
			}
			input := EngineTranslationInput{
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

			transaction, projection, err := TranslateEngineTransaction(input)
			require.NoError(t, err)
			assert.Equal(t, tt.action != constant.ActionCancel, transaction.RejectBlockedBalances)
			require.Len(t, transaction.Postings, len(tt.postingTypes))
			require.Len(t, projection, len(tt.postingTypes))
			if tt.action == constant.ActionCommit || tt.action == constant.ActionCancel {
				assert.Empty(t, transaction.BalanceRequirements)
			} else {
				require.Len(t, transaction.BalanceRequirements, 2)
				assert.Equal(t, accounting.BalancePermissionSend, transaction.BalanceRequirements[0].Permission)
				assert.Equal(t, accounting.BalancePermissionReceive, transaction.BalanceRequirements[1].Permission)
				assert.Equal(t, tt.action == constant.ActionHold, transaction.BalanceRequirements[0].ForbidExternal)
				assert.False(t, transaction.BalanceRequirements[1].ForbidExternal)
			}

			for index := range tt.postingTypes {
				assert.Equal(t, tt.postingTypes[index], transaction.Postings[index].Type)
				assert.Equal(t, tt.drawPolicies[index], transaction.Postings[index].DrawPolicy)
				assert.Equal(t, tt.rowTypes[index], projection[index].RowType)
				assert.Equal(t, tt.directions[index], projection[index].Direction)
				assert.Equal(t, tt.paths[index], projection[index].CompatibilityPath)
				if tt.postingTypes[index] == accounting.PostingCredit || tt.postingTypes[index] == accounting.PostingRelease {
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

func TestBuildPostingPlanPreservesLifecycleMatrix(t *testing.T) {
	credit := func(overdraftCap decimal.Decimal) postingPlanItem {
		return postingPlanItem{
			postingType:             accounting.PostingCredit,
			operationRowType:        constant.CREDIT,
			operationDirection:      constant.DirectionCredit,
			operationProjectionMode: OperationRecordStandard,
			historicalOverdraftCap:  overdraftCap,
			mayAffectOverdraft:      true,
		}
	}
	debit := postingPlanItem{
		postingType:             accounting.PostingDebit,
		operationRowType:        constant.DEBIT,
		operationDirection:      constant.DirectionDebit,
		operationProjectionMode: OperationRecordStandard,
		allowsOverdraftDraw:     true,
		mayAffectOverdraft:      true,
	}

	overdraftAmount := decimal.NewFromInt(3)
	tests := []struct {
		name            string
		action          string
		status          string
		routeValidation bool
		side            string
		want            postingPlan
	}{
		{name: "direct source", action: constant.ActionDirect, status: constant.CREATED, side: OperationSpecSideFrom, want: postingPlan{items: []postingPlanItem{debit}}},
		{name: "direct destination", action: constant.ActionDirect, status: constant.CREATED, side: OperationSpecSideTo, want: postingPlan{items: []postingPlanItem{credit(overdraftAmount)}}},
		{name: "revert source", action: constant.ActionRevert, status: constant.CREATED, side: OperationSpecSideFrom, want: postingPlan{items: []postingPlanItem{debit}}},
		{name: "revert destination", action: constant.ActionRevert, status: constant.CREATED, side: OperationSpecSideTo, want: postingPlan{items: []postingPlanItem{credit(overdraftAmount)}}},
		{name: "pending without route validation", action: constant.ActionHold, status: constant.PENDING, side: OperationSpecSideFrom, want: postingPlan{items: []postingPlanItem{{
			postingType: accounting.PostingHold, operationRowType: constant.ONHOLD, operationDirection: constant.DirectionDebit, operationProjectionMode: OperationRecordStandard,
		}}}},
		{name: "pending with route validation", action: constant.ActionHold, status: constant.PENDING, routeValidation: true, side: OperationSpecSideFrom, want: postingPlan{items: []postingPlanItem{
			{postingType: accounting.PostingDebit, operationRowType: constant.DEBIT, operationDirection: constant.DirectionDebit, operationProjectionMode: OperationRecordValidatedHoldDebit},
			{postingType: accounting.PostingReserve, operationRowType: constant.ONHOLD, operationDirection: constant.DirectionCredit, operationProjectionMode: OperationRecordValidatedHoldReserve},
		}}},
		{name: "pending destination", action: constant.ActionHold, status: constant.PENDING, routeValidation: true, side: OperationSpecSideTo, want: postingPlan{}},
		{name: "commit without route validation", action: constant.ActionCommit, status: constant.APPROVED, side: OperationSpecSideFrom, want: postingPlan{items: []postingPlanItem{{
			postingType: accounting.PostingUnreserve, operationRowType: constant.DEBIT, operationDirection: constant.DirectionDebit, operationProjectionMode: OperationRecordStandard,
		}}}},
		{name: "commit with route validation", action: constant.ActionCommit, status: constant.APPROVED, routeValidation: true, side: OperationSpecSideFrom, want: postingPlan{items: []postingPlanItem{{
			postingType: accounting.PostingUnreserve, operationRowType: constant.ONHOLD, operationDirection: constant.DirectionDebit, operationProjectionMode: OperationRecordStandard,
		}}}},
		{name: "commit destination", action: constant.ActionCommit, status: constant.APPROVED, side: OperationSpecSideTo, want: postingPlan{items: []postingPlanItem{credit(overdraftAmount)}}},
		{name: "cancel without route validation", action: constant.ActionCancel, status: constant.CANCELED, side: OperationSpecSideFrom, want: postingPlan{items: []postingPlanItem{{
			postingType: accounting.PostingRelease, operationRowType: constant.RELEASE, operationDirection: constant.DirectionCredit, operationProjectionMode: OperationRecordStandard,
			historicalOverdraftCap: overdraftAmount, mayAffectOverdraft: true,
		}}}},
		{name: "cancel with route validation", action: constant.ActionCancel, status: constant.CANCELED, routeValidation: true, side: OperationSpecSideFrom, want: postingPlan{items: []postingPlanItem{
			{postingType: accounting.PostingUnreserve, operationRowType: constant.RELEASE, operationDirection: constant.DirectionDebit, operationProjectionMode: OperationRecordValidatedCancelRelease},
			{postingType: accounting.PostingCredit, operationRowType: constant.CREDIT, operationDirection: constant.DirectionCredit, operationProjectionMode: OperationRecordValidatedCancelCredit, historicalOverdraftCap: overdraftAmount, mayAffectOverdraft: true},
		}}},
		{name: "cancel destination", action: constant.ActionCancel, status: constant.CANCELED, routeValidation: true, side: OperationSpecSideTo, want: postingPlan{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildPostingPlan(tt.action, tt.status, tt.side, tt.routeValidation, overdraftAmount)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildPostingPlanRejectsInvalidLifecyclePairs(t *testing.T) {
	tests := []struct {
		name   string
		action string
		status string
	}{
		{name: "direct pending", action: constant.ActionDirect, status: constant.PENDING},
		{name: "revert approved", action: constant.ActionRevert, status: constant.APPROVED},
		{name: "hold created", action: constant.ActionHold, status: constant.CREATED},
		{name: "commit pending", action: constant.ActionCommit, status: constant.PENDING},
		{name: "cancel approved", action: constant.ActionCancel, status: constant.APPROVED},
		{name: "unsupported action", action: "unsupported", status: constant.CREATED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildPostingPlan(tt.action, tt.status, OperationSpecSideFrom, false, decimal.Zero)
			assert.ErrorIs(t, err, ErrInvalidEngineTranslation)
		})
	}
}

func TestTranslateEngineTransactionPreservesRouteAndCompanionContext(t *testing.T) {
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
	input := EngineTranslationInput{
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

	transaction, projection, err := TranslateEngineTransaction(input)
	require.NoError(t, err)
	require.Len(t, transaction.Postings, 1)
	require.Len(t, projection, 2)
	assert.Equal(t, accounting.DrawAllowed, transaction.Postings[0].DrawPolicy, "ledger route mode, not the per-leg split flag, governs draw authorization")
	assert.Equal(t, "default", projection[0].Balance.Key)
	assert.Empty(t, primary.Key, "freezing a default key must not mutate the pool row")
	assert.Equal(t, "DIRECT-D", projection[0].RouteCode)
	assert.Equal(t, accounting.RoleOverdraftCompanion, projection[1].Role)
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
	denied, _, err := TranslateEngineTransaction(input)
	require.NoError(t, err)
	assert.Equal(t, accounting.DrawRouteDenied, denied.Postings[0].DrawPolicy)
}

func TestTranslateEngineTransactionRejectsNonExecutableAndInvalidAmounts(t *testing.T) {
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	input := EngineTranslationInput{TransactionID: transactionID, TransactionStatus: constant.NOTED}
	_, _, err := TranslateEngineTransaction(input)
	assert.ErrorIs(t, err, ErrEngineTransactionNotExecutable)

	input = EngineTranslationInput{
		TransactionID: transactionID, Action: constant.ActionDirect, TransactionStatus: constant.CREATED,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{Source: mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "0#@source#default"}}}}},
		Validate:         &mtransaction.Responses{From: map[string]mtransaction.Amount{"0#@source#default": {Value: decimal.NewFromInt(-1)}}},
		Balances:         []*mmodel.Balance{{Alias: "@source", Key: "default"}},
	}
	_, _, err = TranslateEngineTransaction(input)
	assert.True(t, errors.Is(err, ErrInvalidEngineTranslation))

	input.Validate.From["0#@source#default"] = mtransaction.Amount{
		Value: decimal.NewFromInt(1), OverdraftAmount: decimal.NewFromInt(-1),
	}
	_, _, err = TranslateEngineTransaction(input)
	assert.ErrorIs(t, err, ErrInvalidEngineTranslation)
}

func translationBalance(organizationID, ledgerID uuid.UUID, id, alias, key string) *mmodel.Balance {
	return &mmodel.Balance{
		ID: id, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		AccountID: uuid.NewSHA1(organizationID, []byte(id)).String(), Alias: alias, Key: key,
		AssetCode: "USD", Available: decimal.NewFromInt(100), Version: 1,
		AccountType: "deposit", AllowSending: true, AllowReceiving: true,
	}
}

func TestOperationOriginRefRoundTrip(t *testing.T) {
	t.Parallel()

	for _, side := range []string{OperationSpecSideFrom, OperationSpecSideTo} {
		for _, index := range []int{0, 1, 10} {
			gotSide, gotIndex, ok := parseOperationOriginRef(operationOriginRef(side, index))
			require.True(t, ok)
			assert.Equal(t, side, gotSide)
			assert.Equal(t, index, gotIndex)
		}
	}

	for _, ref := range []string{"", "from", "from:", "from:-1", "from:01", "from:+1", "from:1:debit", "source:0", ":0"} {
		_, _, ok := parseOperationOriginRef(ref)
		assert.False(t, ok, "reference %q must not resolve a leg", ref)
	}
}

func TestOperationPostingRefStaysBoundToItsOrigin(t *testing.T) {
	t.Parallel()

	postingRef := operationPostingRef(operationOriginRef(OperationSpecSideFrom, 10), accounting.PostingDebit)

	assert.Equal(t, "from:10:debit", postingRef)
	assert.True(t, postingRefFromOrigin(postingRef, "from:10"))
	assert.False(t, postingRefFromOrigin(postingRef, "from:1"), "a shorter position must not claim the posting")
	assert.False(t, postingRefFromOrigin(postingRef, "to:10"))
	assert.False(t, postingRefFromOrigin(postingRef, ""))
}
