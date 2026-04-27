// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/mock/gomock"
)

// TestTransactionHandler_VerifyAccountsTransactableFromBalances_Empty covers the
// short-circuit when the balance set carries no resolvable account IDs. The handler
// must return nil without calling into the query layer — otherwise the eligibility
// gate would do unnecessary repo work for every transaction with no account-bound
// balances (e.g. annotation transactions).
func TestTransactionHandler_VerifyAccountsTransactableFromBalances_Empty(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// AccountRepo expects ZERO calls — the handler short-circuits before reaching it.
	mockAccountRepo := account.NewMockRepository(ctrl)

	queryUC := &query.UseCase{AccountRepo: mockAccountRepo}
	handler := &TransactionHandler{Query: queryUC}

	_, span := otel.Tracer("test").Start(context.Background(), "test")
	defer span.End()

	err := handler.verifyAccountsTransactableFromBalances(context.Background(), span, uuid.New(), uuid.New(), nil)
	assert.NoError(t, err, "nil balances must short-circuit without a repo call")

	// Same for explicitly empty.
	err = handler.verifyAccountsTransactableFromBalances(context.Background(), span, uuid.New(), uuid.New(), []*mmodel.Balance{})
	assert.NoError(t, err, "empty balances must short-circuit without a repo call")
}

// TestTransactionHandler_VerifyAccountsTransactableFromBalances_AllActive covers the
// happy path: balances resolve to ACTIVE non-blocked accounts, the query returns nil,
// and the handler propagates that as nil.
func TestTransactionHandler_VerifyAccountsTransactableFromBalances_AllActive(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	balances := []*mmodel.Balance{
		{AccountID: accountID.String()},
	}

	falseVal := false
	activeAccount := &mmodel.Account{
		ID:             accountID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status:         mmodel.Status{Code: constant.AccountStatusActive},
		Blocked:        &falseVal,
	}

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		ListAccountsByIDs(gomock.Any(), organizationID, ledgerID, []uuid.UUID{accountID}).
		Return([]*mmodel.Account{activeAccount}, nil).
		Times(1)

	queryUC := &query.UseCase{AccountRepo: mockAccountRepo}
	handler := &TransactionHandler{Query: queryUC}

	_, span := otel.Tracer("test").Start(context.Background(), "test")
	defer span.End()

	err := handler.verifyAccountsTransactableFromBalances(context.Background(), span, organizationID, ledgerID, balances)
	assert.NoError(t, err, "ACTIVE non-blocked accounts must pass the gate")
}

// TestTransactionHandler_VerifyAccountsTransactableFromBalances_PendingRejected covers
// the rejection path: balances resolve to a PENDING_CRM_LINK account, the query
// returns the eligibility business error, and the handler propagates it unchanged.
// This is the gate that prevents transactions hitting accounts mid-CRM-saga.
func TestTransactionHandler_VerifyAccountsTransactableFromBalances_PendingRejected(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	balances := []*mmodel.Balance{
		{AccountID: accountID.String()},
	}

	trueVal := true
	pendingAccount := &mmodel.Account{
		ID:             accountID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status:         mmodel.Status{Code: constant.AccountStatusPendingCRMLink},
		Blocked:        &trueVal,
	}

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		ListAccountsByIDs(gomock.Any(), organizationID, ledgerID, []uuid.UUID{accountID}).
		Return([]*mmodel.Account{pendingAccount}, nil).
		Times(1)

	queryUC := &query.UseCase{AccountRepo: mockAccountRepo}
	handler := &TransactionHandler{Query: queryUC}

	_, span := otel.Tracer("test").Start(context.Background(), "test")
	defer span.End()

	err := handler.verifyAccountsTransactableFromBalances(context.Background(), span, organizationID, ledgerID, balances)
	require.Error(t, err)

	// The query layer maps the rejection to ErrAccountStatusTransactionRestriction.
	var validation pkg.ValidationError
	require.ErrorAs(t, err, &validation, "expected ValidationError, got %T", err)
	assert.Equal(t, constant.ErrAccountStatusTransactionRestriction.Error(), validation.Code,
		"the eligibility error must surface with its stable code")
}
