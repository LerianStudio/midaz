// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/google/uuid"
)

// AccountAssetsInput contains official records loaded in the authenticated
// tenant and requested organization/ledger. It is not a Tracer limit definition.
type AccountAssetsInput struct {
	Namespace      string
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	Accounts       []*mmodel.Account
	Assets         []*mmodel.Asset
}

// BuildAccountAssets resolves each official account's asset code to the unique
// official UUID in its scope. It never fabricates postings or decides which
// limits should apply. The caller owns authorized database reads and transport;
// this mapper neither fetches records nor attests a caller-supplied namespace.
func BuildAccountAssets(ctx context.Context, input AccountAssetsInput, bounds tracercontract.Limits) ([]tracercontract.AccountAsset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := bounds.Validate(); err != nil {
		return nil, err
	}

	if input.OrganizationID == uuid.Nil || input.LedgerID == uuid.Nil || len(input.Accounts) == 0 ||
		len(input.Accounts) > bounds.MaxAccounts || len(input.Assets) > bounds.MaxAccounts {
		return nil, projectionError("account asset scope or size")
	}

	scope := ContextInput{Namespace: input.Namespace, OrganizationID: input.OrganizationID, LedgerID: input.LedgerID, Assets: input.Assets}

	assets, err := projectAssets(ctx, scope)
	if err != nil {
		return nil, err
	}

	facts := make([]tracercontract.AccountAsset, 0, len(input.Accounts))
	for _, account := range input.Accounts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if account == nil || account.DeletedAt != nil || !sameScope(account.OrganizationID, account.LedgerID, scope) {
			return nil, projectionError("account outside official scope")
		}

		id, err := uuid.Parse(account.ID)
		if err != nil || id == uuid.Nil {
			return nil, projectionError("invalid account UUID")
		}

		asset, exists := assets[account.AssetCode]
		if !exists {
			return nil, projectionError("unresolved account asset")
		}

		facts = append(facts, tracercontract.AccountAsset{AccountID: id, Asset: asset})
	}

	if err := tracercontract.ValidateAccountAssets(ctx, facts, input.Namespace, bounds); err != nil {
		return nil, err
	}

	return facts, nil
}
