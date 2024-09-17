package grpc

import (
	"context"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	proto "github.com/LerianStudio/midaz/components/ledger/proto/account"
	"github.com/google/uuid"
)

// AccountProto struct contains an account use case for managing account related operations.
type AccountProto struct {
	Command *command.UseCase
	Query   *query.UseCase
	proto.UnimplementedAccountProtoServer
}

// GetByIds is a method that retrieves Account information by a given ids.
func (ap *AccountProto) GetByIds(ctx context.Context, ids *proto.ManyAccountsID) (*proto.ManyAccountsResponse, error) {
	logger := mlog.NewLoggerFromContext(ctx)

	uuids := make([]uuid.UUID, len(ids.Ids))
	for i, id := range ids.Ids {
		uuids[i] = uuid.MustParse(id.Id)
	}

	acc, err := ap.Query.ListAccountsByIDs(ctx, uuids)
	if err != nil {
		logger.Errorf("Failed to retrieve Accounts by ids for grpc, Error: %s", err.Error())
		return nil, err
	}

	var accounts []*proto.Account
	for _, a := range acc {
		ac := proto.Account{
			ID:               a.ID,
			Alias:            *a.Alias,
			AvailableBalance: *a.Balance.Available,
			OnHoldBalance:    *a.Balance.OnHold,
			BalanceScale:     *a.Balance.Scale,
			AllowSending:     a.Status.AllowSending,
			AllowReceiving:   a.Status.AllowReceiving,
		}
		accounts = append(accounts, &ac)
	}

	response := proto.ManyAccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

func (ap *AccountProto) GetByAlias(ctx context.Context, alias *proto.ManyAccountsAlias) (*proto.ManyAccountsResponse, error) {
	return nil, nil
}

func (ap *AccountProto) Update(ctx context.Context, update *proto.UpdateRequest) (*proto.Account, error) {
	return nil, nil
}

func (ap *AccountProto) GetByFilters(ctx context.Context, filter *proto.GetByFiltersRequest) (*proto.ManyAccountsResponse, error) {
	return nil, nil
}
