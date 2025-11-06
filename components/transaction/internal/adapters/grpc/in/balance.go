package in

import (
	"context"
	"reflect"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	ctx, span := tracer.Start(ctx, "handler.create_balance")

	defer span.End()

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", req)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return nil, err
	}

	logger.Infof("Initiating create balance")

	orgID, err := uuid.Parse(req.GetOrganizationId())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid organization_id", err)

		logger.Errorf("Invalid organization_id, Error: %s", err.Error())

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "organizationId")
	}

	ledgerID, err := uuid.Parse(req.GetLedgerId())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid ledger_id", err)

		logger.Errorf("Invalid ledger_id, Error: %s", err.Error())

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "ledgerId")
	}

	accountID, err := uuid.Parse(req.GetAccountId())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid account_id", err)

		logger.Errorf("Invalid account_id, Error: %s", err.Error())

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "accountId")
	}

	input := mmodel.CreateBalanceInput{
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      accountID,
		Alias:          req.GetAlias(),
		Key:            req.GetKey(),
		AssetCode:      req.GetAssetCode(),
		AccountType:    req.GetAccountType(),
		AllowSending:   req.GetAllowSending(),
		AllowReceiving: req.GetAllowReceiving(),
	}

	logger.Infof("Request to create a Balance alias=%s key=%s asset_code=%s", input.Alias, input.Key, input.AssetCode)

	created, err := b.Command.CreateBalanceSync(ctx, input)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create balance on command", err)

		logger.Errorf("Failed to create balance, Error: %s", err.Error())

		return nil, err
	}

	logger.Infof("Successfully created balance")

	resp := &balance.BalanceResponse{
		Id:             created.ID,
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
