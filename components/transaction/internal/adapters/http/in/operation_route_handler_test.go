package in

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	libHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
)

func newOperationRouteTestApp(orh *OperationRouteHandler) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes",
		libHTTP.ParseUUIDPathParameters("operation_route"),
		libHTTP.WithBody(new(mmodel.CreateOperationRouteInput), orh.CreateOperationRoute),
	)
	app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id",
		libHTTP.ParseUUIDPathParameters("operation_route"),
		orh.GetOperationRouteByID,
	)
	app.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id",
		libHTTP.ParseUUIDPathParameters("operation_route"),
		libHTTP.WithBody(new(mmodel.UpdateOperationRouteInput), orh.UpdateOperationRoute),
	)
	app.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id",
		libHTTP.ParseUUIDPathParameters("operation_route"),
		orh.DeleteOperationRouteByID,
	)
	app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes",
		libHTTP.ParseUUIDPathParameters("operation_route"),
		orh.GetAllOperationRoutes,
	)
	return app
}

func TestCreateOperationRoute_InvalidUUID(t *testing.T) {
	orh := &OperationRouteHandler{}
	app := newOperationRouteTestApp(orh)

	req := httptest.NewRequest("POST", "/v1/organizations/not-uuid/ledgers/also-bad/operation-routes", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateOperationRoute_UnknownField(t *testing.T) {
	orh := &OperationRouteHandler{}
	app := newOperationRouteTestApp(orh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/operation-routes"
	body := []byte(`{"title":"t","description":"d","operationType":"source","metadata":{},"foo":"bar"}`)
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

func TestCreateOperationRoute_MissingRequired(t *testing.T) {
	orh := &OperationRouteHandler{}
	app := newOperationRouteTestApp(orh)

	// missing operationType
	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/operation-routes"
	body := []byte(`{"title":"t","description":"d","metadata":{}}`)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for missing operationType, got %d", resp.StatusCode)
	}
}

func TestCreateOperationRoute_NullByteInTitle(t *testing.T) {
	orh := &OperationRouteHandler{}
	app := newOperationRouteTestApp(orh)

	url := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/operation-routes"
	body := []byte(`{"title":"bad\u0000title","description":"d","operationType":"source","metadata":{}}`)
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

func TestGetOperationRouteByID_InvalidUUID(t *testing.T) {
	orh := &OperationRouteHandler{}
	app := newOperationRouteTestApp(orh)

	req := httptest.NewRequest("GET", "/v1/organizations/not-uuid/ledgers/123e4567-e89b-12d3-a456-426614174001/operation-routes/123e4567-e89b-12d3-a456-426614174002", nil)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid org UUID, got %d", resp.StatusCode)
	}
}

func TestGetAllOperationRoutes_QueryValidation(t *testing.T) {
	orh := &OperationRouteHandler{}
	app := newOperationRouteTestApp(orh)

	base := "/v1/organizations/123e4567-e89b-12d3-a456-426614174000/ledgers/123e4567-e89b-12d3-a456-426614174001/operation-routes"

	// invalid limit
	req := httptest.NewRequest("GET", base+"?limit=100000", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid limit, got %d", resp.StatusCode)
	}

	// invalid sort_order
	req = httptest.NewRequest("GET", base+"?sort_order=sideways", nil)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid sort order, got %d", resp.StatusCode)
	}

	// invalid cursor
	req = httptest.NewRequest("GET", base+"?cursor=not-a-cursor", nil)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cursor, got %d", resp.StatusCode)
	}

	// invalid date range
	req = httptest.NewRequest("GET", base+"?start_date=2025-01-01", nil)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid date range, got %d", resp.StatusCode)
	}

	// invalid date format
	req = httptest.NewRequest("GET", base+"?start_date=2025-99-99&end_date=2025-99-99", nil)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for invalid date format, got %d", resp.StatusCode)
	}
}
