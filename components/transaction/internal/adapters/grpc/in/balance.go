package in

import (
	"context"
	"reflect"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	balance "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

type BalanceProto struct {
	balance.UnimplementedBalanceProtoServer
	Command *command.UseCase
	Query   *query.UseCase
}

func (b *BalanceProto) CreateBalance(ctx context.Context, req *balance.BalanceRequest) (*balance.BalanceResponse, error) {
	orgID, err := uuid.Parse(req.GetOrganizationId())
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "organizationId")
	}

	ledgerID, err := uuid.Parse(req.GetLedgerId())
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "ledgerId")
	}

	accountID, err := uuid.Parse(req.GetAccountId())
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "accountId")
	}

	created, err := b.Command.CreateBalanceSync(
		ctx,
		orgID,
		ledgerID,
		accountID,
		req.GetAlias(),
		req.GetKey(),
		req.GetAssetCode(),
		req.GetAccountType(),
		req.GetAllowSending(),
		req.GetAllowReceiving(),
	)
	if err != nil {
		return nil, err
	}

	resp := &balance.BalanceResponse{
		Alias:          created.Alias,
		Key:            created.Key,
		AssetCode:      created.AssetCode,
		Available:      created.Available.String(),
		OnHold:         created.OnHold.String(),
		AllowSending:   created.AllowSending,
		AllowReceiving: created.AllowReceiving,
	}

	return resp, nil
}
