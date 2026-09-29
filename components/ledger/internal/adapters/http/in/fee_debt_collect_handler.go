// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// FeeDebtCollectInput names the debtor balance a standalone collection settles from.
type FeeDebtCollectInput struct {
	AccountAlias string           `json:"accountAlias" validate:"required,max=100" doc:"Alias of the debtor account." example:"@customer" maxLength:"100"`
	BalanceKey   string           `json:"balanceKey,omitempty" validate:"omitempty,max=100" doc:"Key of the debtor balance; absent means the default balance." example:"default" maxLength:"100"`
	MaxAmount    *decimal.Decimal `json:"maxAmount,omitempty" doc:"Most the collection may settle, greater than zero; absent means everything the balance owes." example:"150.00"`
}

// FeeDebtCollectOutput is what a collection settled and, when it settled anything, the
// transaction that records it.
type FeeDebtCollectOutput struct {
	Collected     decimal.Decimal `json:"collected" doc:"Amount settled, oldest debt first; 0 when the balance had no open debt, no available funds, or cannot pay a debt's fee account." example:"150.00"`
	TransactionID *string         `json:"transactionId,omitempty" doc:"Transaction recording the settlement, absent when nothing was collected." format:"uuid"`
}

// CollectFeeDebtRequest is the Huma request envelope. Tenancy comes from the path and
// the validated JWT, never from the body.
type CollectFeeDebtRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the collection; a retry returns the first answer, another request under the same key answers 409 (0084). Without it every call is a new collection."`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CollectFeeDebtResponse carries the collection result.
type CollectFeeDebtResponse struct {
	IdempotencyReplayed string `header:"X-Idempotency-Replayed"`
	Body                *FeeDebtCollectOutput
}

// CollectFeeDebtV2 decodes and validates the body, then runs the collection.
func (handler *TransactionHandler) CollectFeeDebtV2(ctx context.Context, in *CollectFeeDebtRequest) (*CollectFeeDebtResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(FeeDebtCollectInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if payload.MaxAmount != nil && !payload.MaxAmount.IsPositive() {
		return nil, pkgHTTP.HumaProblem(pkg.ValidateBadRequestFieldsError(nil,
			map[string]string{"maxAmount": "maxAmount must be greater than zero"}, constant.EntityFeeDebt, nil))
	}

	out, err := handler.collectFeeDebt(ctx, command.CollectFeeDebtInput{
		OrganizationID: orgID, LedgerID: ledgerID, BalanceRef: mtransaction.AliasKey(payload.AccountAlias, payload.BalanceKey),
		MaxAmount: payload.MaxAmount, IdempotencyKey: in.IdempotencyKey, IdempotencyTTL: pkgHTTP.ParseIdempotencyTTL(in.IdempotencyTTL),
	})
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return out, nil
}
