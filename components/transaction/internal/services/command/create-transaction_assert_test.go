package command

import (
	"context"
	"testing"

	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestCreateTransaction_NilTransaction_Panics(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.Nil

	require.Panics(t, func() {
		_, _ = uc.CreateTransaction(ctx, orgID, ledgerID, transactionID, nil)
	}, "should panic when transaction is nil")
}

func TestCreateTransaction_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	// Provide valid transaction to reach the organizationID assertion
	validTransaction := &pkgTransaction.Transaction{
		Send: pkgTransaction.Send{Asset: "USD", Value: decimal.NewFromInt(100)},
	}

	require.Panics(t, func() {
		_, _ = uc.CreateTransaction(ctx, uuid.Nil, uuid.New(), uuid.Nil, validTransaction)
	}, "should panic when organizationID is nil UUID")
}

func TestCreateTransaction_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	// Provide valid transaction and organizationID to reach the ledgerID assertion
	validTransaction := &pkgTransaction.Transaction{
		Send: pkgTransaction.Send{Asset: "USD", Value: decimal.NewFromInt(100)},
	}

	require.Panics(t, func() {
		_, _ = uc.CreateTransaction(ctx, uuid.New(), uuid.Nil, uuid.Nil, validTransaction)
	}, "should panic when ledgerID is nil UUID")
}
