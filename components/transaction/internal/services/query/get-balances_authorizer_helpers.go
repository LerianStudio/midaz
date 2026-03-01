// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// buildAuthorizerOperations converts mmodel.BalanceOperation entries into the
// protobuf BalanceOperation messages expected by the authorizer gRPC service.
// Each amount is scaled to an int64 at the given scale so the authorizer can
// perform integer arithmetic without floating-point precision issues.
func buildAuthorizerOperations(balanceOperations []mmodel.BalanceOperation, scale int32) ([]*authorizerv1.BalanceOperation, error) {
	operations := make([]*authorizerv1.BalanceOperation, 0, len(balanceOperations))

	for _, op := range balanceOperations {
		scaledAmount, err := pkgTransaction.ScaleToInt(op.Amount.Value, scale)
		if err != nil {
			return nil, err
		}

		accountAlias := ""
		balanceKey := ""
		assetCode := op.Amount.Asset

		if op.Balance != nil {
			accountAlias = op.Balance.Alias
			balanceKey = op.Balance.Key
			assetCode = op.Balance.AssetCode
		}

		operations = append(operations, &authorizerv1.BalanceOperation{
			OperationAlias: op.Alias,
			AccountAlias:   accountAlias,
			BalanceKey:     balanceKey,
			AssetCode:      assetCode,
			Operation:      op.Amount.Operation,
			Amount:         scaledAmount,
			Scale:          scale,
			IsExternal:     shard.IsExternal(accountAlias),
		})
	}

	return operations, nil
}

// convertAuthorizerSnapshots converts the authorizer gRPC BalanceSnapshot
// responses back into mmodel.Balance instances. The available and on-hold
// values arrive as pre-scaled integers (e.g. 9000 at scale 2 = 90.00) and
// are converted back to decimal.Decimal using IntToDecimal. Metadata such as
// organization/ledger IDs and timestamps are carried over from the original
// mapBalances entries that were sent into the authorize request.
func convertAuthorizerSnapshots(
	snapshots []*authorizerv1.BalanceSnapshot,
	organizationID, ledgerID uuid.UUID,
	mapBalances map[string]*mmodel.Balance,
) []*mmodel.Balance {
	balances := make([]*mmodel.Balance, 0, len(snapshots))

	for _, snap := range snapshots {
		if snap == nil {
			continue
		}

		available := pkgTransaction.IntToDecimal(snap.GetAvailable(), snap.GetScale())
		onHold := pkgTransaction.IntToDecimal(snap.GetOnHold(), snap.GetScale())

		balance := &mmodel.Balance{
			ID:             snap.GetBalanceId(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      snap.GetAccountId(),
			Alias:          snap.GetOperationAlias(),
			Key:            snap.GetBalanceKey(),
			AssetCode:      snap.GetAssetCode(),
			Available:      available,
			OnHold:         onHold,
			Version:        int64(snap.GetVersion()), //nolint:gosec // protobuf uint32 is always safe to convert to int64
			AccountType:    snap.GetAccountType(),
			AllowSending:   snap.GetAllowSending(),
			AllowReceiving: snap.GetAllowReceiving(),
		}

		// Carry over timestamps and metadata from the original balance if present.
		if orig, ok := mapBalances[snap.GetOperationAlias()]; ok {
			balance.CreatedAt = orig.CreatedAt
			balance.UpdatedAt = orig.UpdatedAt
			balance.DeletedAt = orig.DeletedAt
			balance.Metadata = orig.Metadata
		}

		balances = append(balances, balance)
	}

	return balances
}
