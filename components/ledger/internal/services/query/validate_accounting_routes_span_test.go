// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const validateAccountingRulesSpanName = "usecase.validate_accounting_rules"

// removedAccountingStepSpans are the inner steps of ValidateAccountingRules that no longer
// open a span of their own.
var removedAccountingStepSpans = []string{
	"usecase.validate_account_rules",
	"usecase.validate_overdraft_routes",
}

// directOnlyRouteFixture wires a use case whose ledger validates routes and whose
// transaction route defines a single `direct` source entry with no overdraft rubric.
func directOnlyRouteFixture(t *testing.T) (*UseCase, uuid.UUID, uuid.UUID, string, uuid.UUID) {
	t.Helper()

	ctrl := gomock.NewController(t)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	srcRouteID := uuid.New().String()
	transactionRouteID := uuid.New()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]any{"accounting": map[string]any{"validateRoutes": true}}, nil)

	cache := mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source: map[string]mmodel.OperationRouteCache{
					srcRouteID: {
						OperationType: "source",
						AccountingEntries: &mmodel.AccountingEntries{
							Direct: &mmodel.AccountingEntry{
								Debit: &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
							},
						},
					},
				},
				Destination:   make(map[string]mmodel.OperationRouteCache),
				Bidirectional: make(map[string]mmodel.OperationRouteCache),
			},
		},
	}

	cacheBytes, err := cache.ToMsgpack()
	require.NoError(t, err)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), utils.AccountingRoutesInternalKey(organizationID, transactionRouteID)).
		Return(cacheBytes, nil)

	uc := &UseCase{TransactionRedisRepo: mockRedisRepo, LedgerRepo: mockLedgerRepo}

	return uc, organizationID, ledgerID, srcRouteID, transactionRouteID
}

// The accepted path leaves no trace of the inner steps: no child span and no event.
func TestValidateAccountingRulesSpans_AcceptedPathOpensNoStepSpan(t *testing.T) {
	uc, organizationID, ledgerID, srcRouteID, transactionRouteID := directOnlyRouteFixture(t)

	primaryAlias := "0#@sender#default"
	validate := &mtransaction.Responses{
		TransactionRouteID:  strPtr(transactionRouteID.String()),
		OperationRoutesFrom: map[string]string{primaryAlias: srcRouteID},
		From:                map[string]mtransaction.Amount{primaryAlias: {Operation: "DEBIT", Direction: "debit"}},
	}
	operations := []mmodel.BalanceOperation{{
		Alias:   primaryAlias,
		Balance: &mmodel.Balance{AccountType: "deposit"},
		Amount:  mtransaction.Amount{Operation: "DEBIT", Direction: "debit"},
	}}

	ctx, recorder := spantest.RecordingContext()

	_, err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, constant.ActionDirect)
	require.NoError(t, err)

	spantest.AssertNoSpan(t, recorder, removedAccountingStepSpans...)

	span := spantest.FindSpan(t, recorder, validateAccountingRulesSpanName)
	assert.Empty(t, span.Events(), "an accepted validation records no step event")
	assert.Equal(t, codes.Unset, span.Status().Code)
}

// Scenario: Regras de conta e overdraft com erro viram evento em validate_accounting_rules
// (overdraft step).
func TestValidateAccountingRulesSpans_OverdraftRouteFailureIsAnEventOnTheUseCaseSpan(t *testing.T) {
	uc, organizationID, ledgerID, srcRouteID, transactionRouteID := directOnlyRouteFixture(t)

	primaryAlias := "0#@sender#default"
	companionAlias := "0#@sender#overdraft"
	validate := &mtransaction.Responses{
		TransactionRouteID: strPtr(transactionRouteID.String()),
		OperationRoutesFrom: map[string]string{
			primaryAlias:   srcRouteID,
			companionAlias: srcRouteID,
		},
		From: map[string]mtransaction.Amount{
			primaryAlias:   {Operation: "DEBIT", Direction: "debit"},
			companionAlias: {Operation: "DEBIT", Direction: "debit"},
		},
	}
	operations := []mmodel.BalanceOperation{
		{
			Alias:   primaryAlias,
			Balance: &mmodel.Balance{AccountType: "deposit"},
			Amount:  mtransaction.Amount{Operation: "DEBIT", Direction: "debit"},
		},
		{
			Alias:   companionAlias,
			Balance: &mmodel.Balance{Key: constant.OverdraftBalanceKey, AccountType: "deposit"},
			Amount:  mtransaction.Amount{Operation: "DEBIT", Direction: "debit"},
		},
	}

	ctx, recorder := spantest.RecordingContext()

	_, err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, constant.ActionDirect)
	require.Error(t, err)
	assertOverdraftRouteNotConfigured(t, err)

	spantest.AssertNoSpan(t, recorder, removedAccountingStepSpans...)

	span := spantest.FindSpan(t, recorder, validateAccountingRulesSpanName)
	event, ok := spantest.FindEvent(span, "usecase.validate_overdraft_routes")
	require.True(t, ok, "the overdraft step failure must be an event named after the step on %s", validateAccountingRulesSpanName)
	assert.Contains(t, spantest.EventText(event), constant.ErrOverdraftRouteNotConfigured.Error(),
		"the failure detail rides the event's error attribute")
	assert.Equal(t, codes.Unset, span.Status().Code, "a business rejection keeps the span status unset")
}

// Scenario: Regras de conta e overdraft com erro viram evento em validate_accounting_rules
// (account-rules step). The step records on whatever span the context carries, which in
// production is the usecase.validate_accounting_rules span ValidateAccountingRules opens.
func TestValidateAccountingRulesSpans_AccountRuleFailureIsAnEventOnTheCallerSpan(t *testing.T) {
	ctx, recorder := spantest.RecordingContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	ctx, parent := tracer.Start(ctx, validateAccountingRulesSpanName)

	routeID := uuid.New().String()
	validate := &mtransaction.Responses{
		From:                map[string]mtransaction.Amount{"op-alias": {}},
		To:                  map[string]mtransaction.Amount{},
		OperationRoutesFrom: map[string]string{"op-alias": routeID},
		OperationRoutesTo:   map[string]string{},
	}
	operations := []mmodel.BalanceOperation{{
		Alias:   "op-alias",
		Amount:  mtransaction.Amount{Direction: "debit"},
		Balance: &mmodel.Balance{AccountType: "asset"},
	}}

	// No route registered for routeID: the step answers ErrAccountingRouteNotFound.
	err := validateAccountRules(ctx, map[string]mmodel.OperationRouteCache{}, map[string]mmodel.OperationRouteCache{}, map[string]mmodel.OperationRouteCache{}, validate, operations)
	parent.End()

	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrAccountingRouteNotFound.Error())

	spantest.AssertNoSpan(t, recorder, removedAccountingStepSpans...)

	span := spantest.FindSpan(t, recorder, validateAccountingRulesSpanName)
	event, ok := spantest.FindEvent(span, "usecase.validate_account_rules")
	require.True(t, ok, "the account-rules step failure must be an event named after the step on the caller span")
	assert.Contains(t, spantest.EventText(event), constant.ErrAccountingRouteNotFound.Error(),
		"the failure detail rides the event's error attribute")
	assert.Equal(t, codes.Unset, span.Status().Code, "a business rejection keeps the span status unset")
}
