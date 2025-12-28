package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
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

	require.Panics(t, func() {
		_, _ = uc.CreateTransaction(ctx, uuid.Nil, uuid.New(), uuid.Nil, nil)
	}, "should panic when organizationID is nil UUID")
}

func TestCreateTransaction_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	require.Panics(t, func() {
		_, _ = uc.CreateTransaction(ctx, uuid.New(), uuid.Nil, uuid.Nil, nil)
	}, "should panic when ledgerID is nil UUID")
}
