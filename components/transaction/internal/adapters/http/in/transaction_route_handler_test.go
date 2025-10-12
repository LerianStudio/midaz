package in

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	libHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
)

func newTestApp(trh *TransactionRouteHandler) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes",
		libHTTP.ParseUUIDPathParameters("transaction_route"),
		libHTTP.WithBody(new(mmodel.CreateTransactionRouteInput), trh.CreateTransactionRoute),
	)
	app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id",
		libHTTP.ParseUUIDPathParameters("transaction_route"),
		trh.GetTransactionRouteByID,
	)
	app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes",
		libHTTP.ParseUUIDPathParameters("transaction_route"),
		trh.GetAllTransactionRoutes,
	)
	app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id",
		libHTTP.ParseUUIDPathParameters("transaction_route"),
		libHTTP.WithBody(new(mmodel.UpdateTransactionRouteInput), trh.UpdateTransactionRoute),
	)
	app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id",
		libHTTP.ParseUUIDPathParameters("transaction_route"),
		trh.DeleteTransactionRouteByID,
	)
	return app
}

func TestCreateTransactionRoute_InvalidUUID(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	req := httptest.NewRequest("POST", "/v1/organizations/not-uuid/ledgers/also-bad/transaction-routes", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateTransactionRoute_UnknownField(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	// valid UUIDs
	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes"
	// include an unknown field "foo" to trigger 400
	body := []byte(`{"title":"t","description":"d","operationRoutes":["123e4567-e89b-12d3-a456-426614174002"],"metadata":{},"foo":"bar"}`)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d", resp.StatusCode)
	}
}

func TestGetTransactionRouteByID_InvalidUUID(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	req := httptest.NewRequest("GET", "/v1/organizations/not-uuid/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes/123e4567-e89b-12d3-a456-426614174002", nil)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid org UUID, got %d", resp.StatusCode)
	}
}

func TestGetAllTransactionRoutes_InvalidLimit(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes?limit=100000"
	req := httptest.NewRequest("GET", url, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid limit, got %d", resp.StatusCode)
	}
}

func TestGetAllTransactionRoutes_InvalidSortOrder(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes?sort_order=sideways"
	req := httptest.NewRequest("GET", url, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid sort order, got %d", resp.StatusCode)
	}
}

func TestGetAllTransactionRoutes_InvalidCursor(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes?cursor=not-a-valid-cursor"
	req := httptest.NewRequest("GET", url, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cursor, got %d", resp.StatusCode)
	}
}

func TestCreateTransactionRoute_NullByteInTitle(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes"
	body := []byte(`{"title":"bad\u0000title","description":"d","operationRoutes":["123e4567-e89b-12d3-a456-426614174002"],"metadata":{}}`)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for null byte in title, got %d", resp.StatusCode)
	}
}

func TestCreateTransactionRoute_MissingOperationRoutes(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes"
	// operationRoutes omitted
	body := []byte(`{"title":"t","description":"d","metadata":{}}`)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for missing operationRoutes, got %d", resp.StatusCode)
	}
}

func TestCreateTransactionRoute_TitleTooLong(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes"
	longTitle := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz" // > 50
	body := []byte(`{"title":"` + longTitle + `","description":"d","operationRoutes":["123e4567-e89b-12d3-a456-426614174002"],"metadata":{}}`)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for too long title, got %d", resp.StatusCode)
	}
}

func TestGetAllTransactionRoutes_InvalidDateRange(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	// only start_date provided should trigger invalid date range
	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes?start_date=2025-01-01"
	req := httptest.NewRequest("GET", url, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid date range, got %d", resp.StatusCode)
	}
}

func TestGetAllTransactionRoutes_InvalidDateFormat(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	// both provided but invalid format
	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes?start_date=2025-99-99&end_date=2025-99-99"
	req := httptest.NewRequest("GET", url, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid date format, got %d", resp.StatusCode)
	}
}

func TestUpdateTransactionRoute_InvalidUUID(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/not-uuid/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes/123e4567-e89b-12d3-a456-426614174002"
	body := []byte(`{"title":"ok"}`)
	req := httptest.NewRequest("PATCH", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid org UUID, got %d", resp.StatusCode)
	}
}

func TestUpdateTransactionRoute_UnknownField(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes/123e4567-e89b-12d3-a456-426614174002"
	body := []byte(`{"title":"ok","foo":"bar"}`)
	req := httptest.NewRequest("PATCH", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d", resp.StatusCode)
	}
}

func TestUpdateTransactionRoute_TitleTooLong(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes/123e4567-e89b-12d3-a456-426614174002"
	longTitle := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz" // > 50
	body := []byte(`{"title":"` + longTitle + `"}`)
	req := httptest.NewRequest("PATCH", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for too long title, got %d", resp.StatusCode)
	}
}

func TestUpdateTransactionRoute_NullByteInDescription(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes/123e4567-e89b-12d3-a456-426614174002"
	body := []byte(`{"description":"bad\u0000desc"}`)
	req := httptest.NewRequest("PATCH", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for null byte in description, got %d", resp.StatusCode)
	}
}

func TestDeleteTransactionRouteByID_InvalidUUID(t *testing.T) {
	trh := &TransactionRouteHandler{}
	app := newTestApp(trh)

	url := "/v1/organizations/not-uuid/ledgers/123e4567-e89b-12d3-a456-426614174001/transaction-routes/123e4567-e89b-12d3-a456-426614174002"
	req := httptest.NewRequest("DELETE", url, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid org UUID, got %d", resp.StatusCode)
	}
}
