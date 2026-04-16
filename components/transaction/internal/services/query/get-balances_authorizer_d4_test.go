// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// D4 scope item #6: when the authorizer returns codes.FailedPrecondition, the
// transaction service MUST map it to ErrTransactionRequiresManualIntervention
// (non-retryable), not the default ErrGRPCServiceUnavailable (retryable).
// Naive retry on a partial-commit state would double-spend.
func TestTransactionClient_DoesNotRetryOnManualInterventionRejection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	balance := &mmodel.Balance{
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@alice",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.NewFromInt(0),
	}

	balanceOperations := []mmodel.BalanceOperation{
		{
			Alias:   "@alice#default",
			Balance: balance,
			Amount: pkgTransaction.Amount{
				Asset: "USD",
				Value: decimal.NewFromInt(100),
			},
		},
	}

	// Simulate the authorizer returning FailedPrecondition (partial-commit).
	stub := &stubAuthorizer{
		enabled: true,
		err:     grpcstatus.Error(codes.FailedPrecondition, "transaction partial-commit durable; manual intervention required (do not retry)"),
	}

	uc := &UseCase{
		Authorizer: stub,
	}

	_, err := uc.processAuthorizerAtomicOperation(
		ctx,
		organizationID,
		ledgerID,
		transactionID,
		constant.CREATED,
		false,
		balanceOperations,
		map[string]*mmodel.Balance{"@alice#default": balance},
	)
	require.Error(t, err)

	// Must NOT be a ServiceUnavailable (retryable) error.
	var serviceUnavailableErr pkg.ServiceUnavailableError
	assert.NotErrorAs(t, err, &serviceUnavailableErr,
		"FailedPrecondition must NOT map to ServiceUnavailable (retryable)")

	// Must be a non-retryable UnprocessableOperationError with the dedicated
	// ErrTransactionRequiresManualIntervention code.
	var unprocessableErr pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &unprocessableErr)
	assert.Equal(t, constant.ErrTransactionRequiresManualIntervention.Error(), unprocessableErr.Code)

	// Caller made a single Authorize call — no retry even though the
	// authorizer would normally be considered transiently unavailable.
	assert.Equal(t, 1, stub.authorizeCalls, "client must not retry on FailedPrecondition")
}

// D4 scope item #6: any other gRPC error class continues to map to the
// retryable ErrGRPCServiceUnavailable path so legacy callers are not
// surprised by new mappings.
func TestTransactionClient_RetryPreservedForOtherGRPCCodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	balance := &mmodel.Balance{
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@alice",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.NewFromInt(0),
	}

	balanceOperations := []mmodel.BalanceOperation{
		{
			Alias:   "@alice#default",
			Balance: balance,
			Amount: pkgTransaction.Amount{
				Asset: "USD",
				Value: decimal.NewFromInt(100),
			},
		},
	}

	stub := &stubAuthorizer{
		enabled: true,
		err:     grpcstatus.Error(codes.Unavailable, "backend down"),
	}

	uc := &UseCase{
		Authorizer: stub,
	}

	_, err := uc.processAuthorizerAtomicOperation(
		ctx,
		organizationID,
		ledgerID,
		transactionID,
		constant.CREATED,
		false,
		balanceOperations,
		map[string]*mmodel.Balance{"@alice#default": balance},
	)
	require.Error(t, err)

	var serviceUnavailableErr pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &serviceUnavailableErr)
	assert.Equal(t, constant.ErrGRPCServiceUnavailable.Error(), serviceUnavailableErr.Code)
}
