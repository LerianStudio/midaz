// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"reflect"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"

	"fmt"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	balance "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
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

	span.SetAttributes(
		attribute.String("app.request.payload.type", "BalanceRequest"),
		attribute.Bool("app.request.payload.has_account_id", req.GetAccountId() != ""),
		attribute.Bool("app.request.payload.has_alias", req.GetAlias() != ""),
		attribute.Bool("app.request.payload.has_key", req.GetKey() != ""),
	)

	logger.Log(ctx, libLog.LevelInfo, "Initiating create balance request")

	orgID, err := uuid.Parse(req.GetOrganizationId())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid organization_id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid organization_id, Error: %s", err.Error()))

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "organizationId")
	}

	ledgerID, err := uuid.Parse(req.GetLedgerId())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ledger_id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid ledger_id, Error: %s", err.Error()))

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "ledgerId")
	}

	accountID, err := uuid.Parse(req.GetAccountId())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account_id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid account_id, Error: %s", err.Error()))

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
		RequestID:      req.GetRequestId(),
	}

	created, err := b.Command.CreateBalanceSync(ctx, input)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create balance on command", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create balance, Error: %s", err.Error()))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, "Successfully created balance")

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

func (b *BalanceProto) DeleteAllBalancesByAccountID(ctx context.Context, req *balance.DeleteAllBalancesByAccountIDRequest) (*balance.Empty, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	ctx, span := tracer.Start(ctx, "handler.delete_all_balances_by_account_id")

	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.payload.type", "DeleteAllBalancesByAccountIDRequest"),
		attribute.Bool("app.request.payload.has_account_id", req.GetAccountId() != ""),
	)

	logger.Log(ctx, libLog.LevelInfo, "Initiating delete all balances by account id request")

	orgID, err := uuid.Parse(req.GetOrganizationId())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid organization_id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid organization_id, Error: %s", err.Error()))

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "organizationId")
	}

	ledgerID, err := uuid.Parse(req.GetLedgerId())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ledger_id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid ledger_id, Error: %s", err.Error()))

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "ledgerId")
	}

	accountID, err := uuid.Parse(req.GetAccountId())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account_id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid account_id, Error: %s", err.Error()))

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Balance{}).Name(), "accountId")
	}

	err = b.Command.DeleteAllBalancesByAccountID(ctx, orgID, ledgerID, accountID, req.GetRequestId())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete all balances by account id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete all balances by account id, Error: %s", err.Error()))

		return nil, err
	}

	return &balance.Empty{}, nil
}
