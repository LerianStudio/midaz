package grpc

import (
	"context"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/common"
	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
)

// AccountProto struct contains an account use case for managing account related operations.
type AccountProto struct {
	Command *command.UseCase
	Query   *query.UseCase
	proto.UnimplementedAccountProtoServer
}

// GetAccountsByIds is a method that retrieves Account information by a given ids.
func (ap *AccountProto) GetAccountsByIds(ctx context.Context, ids *proto.AccountsID) (*proto.AccountsResponse, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.GetAccountsByIds")
	defer span.End()

	organizationUUID, err := uuid.Parse(ids.GetOrganizationId())
	if err != nil {
		return nil, common.ValidateBusinessError(cn.ErrInvalidPathParameter, reflect.TypeOf(a.Account{}).Name(), organizationUUID)
	}

	ledgerUUID, err := uuid.Parse(ids.GetLedgerId())
	if err != nil {
		return nil, common.ValidateBusinessError(cn.ErrInvalidPathParameter, reflect.TypeOf(a.Account{}).Name(), ledgerUUID)
	}

	var invalidUUIDs []string

	uuids := make([]uuid.UUID, len(ids.GetIds()))

	for _, id := range ids.GetIds() {
		parsedUUID, err := uuid.Parse(id)

		if err != nil {
			invalidUUIDs = append(invalidUUIDs, id)
			continue
		} else {
			uuids = append(uuids, parsedUUID)
		}
	}

	if len(invalidUUIDs) > 0 {
		return nil, common.ValidateBusinessError(cn.ErrInvalidPathParameter, reflect.TypeOf(a.Account{}).Name(), strings.Join(invalidUUIDs, ", "))
	}

	acc, err := ap.Query.ListAccountsByIDs(ctx, organizationUUID, ledgerUUID, uuids)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Accounts by ids for grpc", err)

		logger.Errorf("Failed to retrieve Accounts by ids for grpc, Error: %s", err.Error())

		return nil, common.ValidateBusinessError(cn.ErrNoAccountsFound, reflect.TypeOf(a.Account{}).Name())
	}

	accounts := make([]*proto.Account, 0)
	for _, ac := range acc {
		accounts = append(accounts, ac.ToProto())
	}

	response := proto.AccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

// GetAccountsByAliases is a method that retrieves Account information by a given aliases.
func (ap *AccountProto) GetAccountsByAliases(ctx context.Context, aliases *proto.AccountsAlias) (*proto.AccountsResponse, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.GetAccountsByAliases")
	defer span.End()

	organizationUUID, err := uuid.Parse(aliases.GetOrganizationId())
	if err != nil {
		return nil, common.ValidateBusinessError(cn.ErrInvalidPathParameter, reflect.TypeOf(a.Account{}).Name(), organizationUUID)
	}

	ledgerUUID, err := uuid.Parse(aliases.GetLedgerId())
	if err != nil {
		return nil, common.ValidateBusinessError(cn.ErrInvalidPathParameter, reflect.TypeOf(a.Account{}).Name(), ledgerUUID)
	}

	acc, err := ap.Query.ListAccountsByAlias(ctx, organizationUUID, ledgerUUID, aliases.GetAliases())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Accounts by aliases for grpc", err)

		logger.Errorf("Failed to retrieve Accounts by aliases for grpc, Error: %s", err.Error())

		return nil, common.ValidateBusinessError(cn.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(a.Account{}).Name())
	}

	accounts := make([]*proto.Account, 0)
	for _, ac := range acc {
		accounts = append(accounts, ac.ToProto())
	}

	response := proto.AccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

// UpdateAccounts is a method that update Account balances by a given ids.
func (ap *AccountProto) UpdateAccounts(ctx context.Context, update *proto.AccountsRequest) (*proto.AccountsResponse, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.UpdateAccounts")
	defer span.End()

	accounts := make([]*proto.Account, 0)

	uuids := make([]uuid.UUID, 0)

	for _, account := range update.GetAccounts() {
		if common.IsNilOrEmpty(&account.Id) {
			mopentelemetry.HandleSpanError(&span, "Failed to update Accounts because id is empty", nil)

			logger.Errorf("Failed to update Accounts because id is empty")

			return nil, common.ValidateBusinessError(cn.ErrNoAccountIDsProvided, reflect.TypeOf(a.Account{}).Name())
		}

		balance := a.Balance{
			Available: &account.Balance.Available,
			OnHold:    &account.Balance.OnHold,
			Scale:     &account.Balance.Scale,
		}

		organizationUUID, err := uuid.Parse(account.GetOrganizationId())
		if err != nil {
			return nil, common.ValidateBusinessError(cn.ErrInvalidPathParameter, reflect.TypeOf(a.Account{}).Name(), organizationUUID)
		}

		ledgerUUID, err := uuid.Parse(account.GetLedgerId())
		if err != nil {
			return nil, common.ValidateBusinessError(cn.ErrInvalidPathParameter, reflect.TypeOf(a.Account{}).Name(), ledgerUUID)
		}

		accountUUID, err := uuid.Parse(account.GetId())
		if err != nil {
			return nil, common.ValidateBusinessError(cn.ErrInvalidPathParameter, reflect.TypeOf(a.Account{}).Name(), accountUUID)
		}

		_, err = ap.Command.UpdateAccountByID(ctx, organizationUUID, ledgerUUID, accountUUID, &balance)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to update balance in Account by id", err)

			logger.Errorf("Failed to update balance in Account by id for organizationId %s and ledgerId %s in grpc, Error: %s", account.OrganizationId, account.LedgerId, err.Error())

			return nil, common.ValidateBusinessError(cn.ErrBalanceUpdateFailed, reflect.TypeOf(a.Account{}).Name())
		}

		uuids = append(uuids, uuid.MustParse(account.Id))
	}

	organizationID := update.GetOrganizationId()
	ledgerID := update.GetLedgerId()

	acc, err := ap.Query.ListAccountsByIDs(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuids)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Accounts by ids for grpc", err)

		logger.Errorf("Failed to retrieve Accounts by ids for organizationId %s and ledgerId %s in grpc, Error: %s", organizationID, ledgerID, err.Error())

		return nil, common.ValidateBusinessError(cn.ErrNoAccountsFound, reflect.TypeOf(a.Account{}).Name())
	}

	for _, ac := range acc {
		accounts = append(accounts, ac.ToProto())
	}

	response := proto.AccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}
