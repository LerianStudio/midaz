package in

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	libCommonsHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	pgOperationRoute "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	pgTransactionRoute "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	libHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func newMockedHandler(t *testing.T) (*TransactionRouteHandler, *gomock.Controller, *pgOperationRoute.MockRepository, *pgTransactionRoute.MockRepository, *mongodb.MockRepository, *redis.MockRedisRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)

	mockOpRouteRepo := pgOperationRoute.NewMockRepository(ctrl)
	mockTrxRouteRepo := pgTransactionRoute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	trh := &TransactionRouteHandler{
		Command: &command.UseCase{
			OperationRouteRepo:   mockOpRouteRepo,
			TransactionRepo:      nil,
			TransactionRouteRepo: mockTrxRouteRepo,
			AssetRateRepo:        nil,
			BalanceRepo:          nil,
			MetadataRepo:         mockMetadataRepo,
			RabbitMQRepo:         nil,
			RedisRepo:            mockRedisRepo,
		},
		Query: &query.UseCase{
			OperationRepo:        nil,
			TransactionRepo:      nil,
			TransactionRouteRepo: mockTrxRouteRepo,
			AssetRateRepo:        nil,
			BalanceRepo:          nil,
			MetadataRepo:         mockMetadataRepo,
			RabbitMQRepo:         nil,
			RedisRepo:            mockRedisRepo,
		},
	}

	return trh, ctrl, mockOpRouteRepo, mockTrxRouteRepo, mockMetadataRepo, mockRedisRepo
}

func TestCreateTransactionRoute_Success(t *testing.T) {
	trh, ctrl, mockOpRouteRepo, mockTrxRouteRepo, mockMetadataRepo, mockRedisRepo := newMockedHandler(t)
	defer ctrl.Finish()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes",
		libHTTP.ParseUUIDPathParameters("transaction_route"),
		libHTTP.WithBody(new(mmodel.CreateTransactionRouteInput), trh.CreateTransactionRoute),
	)

	orgID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	ledgerID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174001")
	op1 := uuid.MustParse("123e4567-e89b-12d3-a456-426614174002")
	op2 := uuid.MustParse("123e4567-e89b-12d3-a456-426614174003")

	mockOpRouteRepo.EXPECT().FindByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).Return([]*mmodel.OperationRoute{
		{ID: op1, OrganizationID: orgID, LedgerID: ledgerID, OperationType: "source"},
		{ID: op2, OrganizationID: orgID, LedgerID: ledgerID, OperationType: "destination"},
	}, nil)

	mockTrxRouteRepo.EXPECT().Create(gomock.Any(), orgID, ledgerID, gomock.Any()).DoAndReturn(
		func(_ interface{}, _ uuid.UUID, _ uuid.UUID, tr *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error) {
			return tr, nil
		},
	)

	mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockRedisRepo.EXPECT().SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	payload := map[string]any{
		"title":           "Route A",
		"description":     "desc",
		"operationRoutes": []string{op1.String(), op2.String()},
		"metadata":        map[string]any{"k": "v"},
	}
	b, _ := json.Marshal(payload)
	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transaction-routes"
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
}

func TestGetTransactionRouteByID_Success(t *testing.T) {
	trh, ctrl, _, mockTrxRouteRepo, mockMetadataRepo, _ := newMockedHandler(t)
	defer ctrl.Finish()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id",
		libHTTP.ParseUUIDPathParameters("transaction_route"),
		trh.GetTransactionRouteByID,
	)

	orgID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	ledgerID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174001")
	id := uuid.MustParse("123e4567-e89b-12d3-a456-426614174004")

	mockTrxRouteRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, id).Return(&mmodel.TransactionRoute{ID: id, OrganizationID: orgID, LedgerID: ledgerID}, nil)
	mockMetadataRepo.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transaction-routes/" + id.String()
	req := httptest.NewRequest("GET", url, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestUpdateTransactionRoute_Success(t *testing.T) {
	trh, ctrl, _, mockTrxRouteRepo, mockMetadataRepo, mockRedisRepo := newMockedHandler(t)
	defer ctrl.Finish()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id",
		libHTTP.ParseUUIDPathParameters("transaction_route"),
		libHTTP.WithBody(new(mmodel.UpdateTransactionRouteInput), trh.UpdateTransactionRoute),
	)

	orgID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	ledgerID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174001")
	id := uuid.MustParse("123e4567-e89b-12d3-a456-426614174004")

	mockTrxRouteRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, id, gomock.Any(), gomock.Nil(), gomock.Nil()).Return(&mmodel.TransactionRoute{ID: id, OrganizationID: orgID, LedgerID: ledgerID}, nil)
	mockTrxRouteRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, id).Return(&mmodel.TransactionRoute{ID: id, OrganizationID: orgID, LedgerID: ledgerID}, nil)
	mockMetadataRepo.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	mockMetadataRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockRedisRepo.EXPECT().SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	body := []byte(`{"title":"updated"}`)
	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transaction-routes/" + id.String()
	req := httptest.NewRequest("PATCH", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDeleteTransactionRouteByID_Success(t *testing.T) {
	trh, ctrl, _, mockTrxRouteRepo, _, mockRedisRepo := newMockedHandler(t)
	defer ctrl.Finish()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id",
		libHTTP.ParseUUIDPathParameters("transaction_route"),
		trh.DeleteTransactionRouteByID,
	)

	orgID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	ledgerID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174001")
	id := uuid.MustParse("123e4567-e89b-12d3-a456-426614174004")
	op1 := uuid.MustParse("123e4567-e89b-12d3-a456-426614174002")

	mockTrxRouteRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, id).Return(&mmodel.TransactionRoute{ID: id, OrganizationID: orgID, LedgerID: ledgerID, OperationRoutes: []mmodel.OperationRoute{{ID: op1}}}, nil)
	mockTrxRouteRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, id, gomock.Any()).Return(nil)
	mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transaction-routes/" + id.String()
	req := httptest.NewRequest("DELETE", url, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestGetAllTransactionRoutes_Success(t *testing.T) {
	trh, ctrl, _, mockTrxRouteRepo, mockMetadataRepo, _ := newMockedHandler(t)
	defer ctrl.Finish()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes",
		libHTTP.ParseUUIDPathParameters("transaction_route"),
		trh.GetAllTransactionRoutes,
	)

	orgID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	ledgerID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174001")

	mockTrxRouteRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).Return([]*mmodel.TransactionRoute{}, libCommonsHTTP.CursorPagination{Next: "", Prev: ""}, nil)
	mockMetadataRepo.EXPECT().FindList(gomock.Any(), gomock.Any(), gomock.Any()).Return([]*mongodb.Metadata{}, nil)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transaction-routes"
	req := httptest.NewRequest("GET", url, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
