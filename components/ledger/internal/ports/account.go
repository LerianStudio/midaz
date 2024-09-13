package ports

import (
	"context"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	proto "github.com/LerianStudio/midaz/components/ledger/proto/account"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// AccountHandler struct contains an account use case for managing account related operations.
type AccountHandler struct {
	proto.UnimplementedAccountHandlerServer
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateAccount is a method that creates account information.
func (handler *AccountHandler) CreateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	portfolioID := c.Params("portfolio_id")

	logger.Infof("Initiating create of Account with Portfolio ID: %s", portfolioID)

	payload := i.(*a.CreateAccountInput)
	logger.Infof("Request to create a Account with details: %#v", payload)

	account, err := handler.Command.CreateAccount(ctx, organizationID, ledgerID, portfolioID, payload)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created Account")

	return commonHTTP.Created(c, account)
}

// GetAllAccounts is a method that retrieves all Accounts.
func (handler *AccountHandler) GetAllAccounts(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	portfolioID := c.Params("portfolio_id")

	logger.Infof("Get Accounts with Portfolio ID: %s", portfolioID)

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Accounts by metadata")

		accounts, err := handler.Query.GetAllMetadataAccounts(ctx, organizationID, ledgerID, portfolioID, *headerParams)
		if err != nil {
			logger.Errorf("Failed to retrieve all Accounts, Error: %s", err.Error())
			return commonHTTP.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Accounts by metadata")

		pagination.SetItems(accounts)

		return commonHTTP.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Accounts ")

	headerParams.Metadata = &bson.M{}

	accounts, err := handler.Query.GetAllAccount(ctx, organizationID, ledgerID, portfolioID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Accounts, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Accounts")

	pagination.SetItems(accounts)

	return commonHTTP.OK(c, pagination)
}

// GetAccountByID is a method that retrieves Account information by a given id.
func (handler *AccountHandler) GetAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	portfolioID := c.Params("portfolio_id")
	id := c.Params("id")

	logger := mlog.NewLoggerFromContext(ctx)

	logger.Infof("Initiating retrieval of Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, portfolioID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Account with Portfolio ID: %s and Account ID: %s, Error: %s", portfolioID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	return commonHTTP.OK(c, account)
}

// UpdateAccount is a method that updates Account information.
func (handler *AccountHandler) UpdateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	portfolioID := c.Params("portfolio_id")
	id := c.Params("id")

	logger.Infof("Initiating update of Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	payload := i.(*a.UpdateAccountInput)
	logger.Infof("Request to update an Account with details: %#v", payload)

	account, err := handler.Command.UpdateAccountByID(ctx, organizationID, ledgerID, portfolioID, id, payload)
	if err != nil {
		logger.Errorf("Failed to update Account with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	return commonHTTP.OK(c, account)
}

// DeleteAccountByID is a method that removes Account information by a given ids.
func (handler *AccountHandler) DeleteAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	portfolioID := c.Params("portfolio_id")
	id := c.Params("id")

	logger.Infof("Initiating removal of Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	if err := handler.Command.DeleteAccountByID(ctx, organizationID, ledgerID, portfolioID, id); err != nil {
		logger.Errorf("Failed to remove Account with Portfolio ID: %s and Account ID: %s, Error: %s", portfolioID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	return commonHTTP.NoContent(c)
}

// GetByIds is a method that retrieves Account information by a given ids.
func (handler *AccountHandler) GetByIds(ctx context.Context, ids *proto.ManyAccountsID) (*proto.ManyAccountsResponse, error) {
	logger := mlog.NewLoggerFromContext(ctx)

	uuids := make([]uuid.UUID, len(ids.Ids))
	for i, id := range ids.Ids {
		uuids[i] = uuid.MustParse(id.Id)
	}

	acc, err := handler.Query.ListAccountsByIDs(ctx, uuids)
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

func (handler *AccountHandler) GetByAlias(ctx context.Context, alias *proto.ManyAccountsAlias) (*proto.ManyAccountsResponse, error) {
	return nil, nil
}

func (handler *AccountHandler) Update(ctx context.Context, update *proto.UpdateRequest) (*proto.Account, error) {
	return nil, nil
}

func (handler *AccountHandler) GetByFilters(ctx context.Context, filter *proto.GetByFiltersRequest) (*proto.ManyAccountsResponse, error) {
	return nil, nil
}
