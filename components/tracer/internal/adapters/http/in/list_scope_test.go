// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// tracerPartnerContext is the request context a filtering tracer list hands its
// handler for a partner the authorization service answers with allowed.
func tracerPartnerContext(t *testing.T, allowed string, dimensions ...string) context.Context {
	t.Helper()

	authz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorized":true` + allowed + `}`))
	}))
	t.Cleanup(authz.Close)

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type": "application", "owner": "list-org", "sub": "list-org/list-app", "partner": "list-partner",
	}).SignedString([]byte("list-secret"))
	require.NoError(t, err)

	auth := &authMiddleware.AuthClient{Enabled: true, Address: authz.URL}

	var got context.Context

	app := fiber.New()
	app.Get("/things", auth.Authorize("tracer", "validations", "get", authMiddleware.RequireScope("tracer").Filter(dimensions...)),
		func(c fiber.Ctx) error {
			got = c.Context()

			return c.SendStatus(fiber.StatusOK)
		})

	req := httptest.NewRequest(fiber.MethodGet, "/things", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, fiber.StatusOK, resp.StatusCode)

	return got
}

func TestTracerLists_ConfinedToThePartnerScope(t *testing.T) {
	account, merchant, portfolio := uuid.New(), uuid.New(), uuid.New()
	allowed := `,"allowed":{"accountId":["` + account.String() + `"],"merchantId":["` + merchant.String() + `"],"segmentId":[],"portfolioId":["` + portfolio.String() + `"]}`
	want := pkgHTTP.ScopeConfinement{"accountId": {account}, "merchantId": {merchant}, "segmentId": {}, "portfolioId": {portfolio}}

	t.Run("transaction validations", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		service := mocks.NewMockTransactionValidationService(ctrl)

		service.EXPECT().ListTransactionValidations(gomock.Any(), gomock.Cond(func(f *model.TransactionValidationFilters) bool {
			return assert.Equal(t, want, f.Scope)
		})).Return(&query.ListTransactionValidationsResult{}, nil)

		ctx := tracerPartnerContext(t, allowed, tracerListScopeDimensions...)
		_, err := NewTransactionValidationHandler(service).listTransactionValidations(ctx, func(any) error { return nil })
		require.NoError(t, err)
	})

	t.Run("audit events", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		service := NewMockAuditEventService(ctrl)

		service.EXPECT().ListAuditEvents(gomock.Any(), gomock.Cond(func(f *model.AuditEventFilters) bool {
			return assert.Equal(t, want, f.Scope)
		})).Return(&model.ListAuditEventsResult{}, nil)

		ctx := tracerPartnerContext(t, allowed, tracerListScopeDimensions...)
		_, err := NewAuditEventHandler(service).listAuditEvents(ctx, func(any) error { return nil })
		require.NoError(t, err)
	})

	t.Run("a caller bound to no partner is not confined", func(t *testing.T) {
		assert.Nil(t, tracerListScope(context.Background(), tracerListScopeDimensions...))
	})
}

// TestTracerLists_AbsentDimensionIsUnrestricted pins the two readings of an allowed
// answer: a dimension the authorization service leaves out confines nothing, and one
// it answers with an empty list confines to nothing.
func TestTracerLists_AbsentDimensionIsUnrestricted(t *testing.T) {
	account := uuid.New()

	t.Run("only accountId answered: the other dimensions confine nothing", func(t *testing.T) {
		ctx := tracerPartnerContext(t, `,"allowed":{"accountId":["`+account.String()+`"]}`, tracerListScopeDimensions...)
		assert.Equal(t, pkgHTTP.ScopeConfinement{"accountId": {account}}, tracerListScope(ctx, tracerListScopeDimensions...))
	})

	t.Run("an empty answer confines to nothing", func(t *testing.T) {
		ctx := tracerPartnerContext(t, `,"allowed":{"merchantId":[]}`, tracerListScopeDimensions...)

		scope := tracerListScope(ctx, tracerListScopeDimensions...)
		assert.Equal(t, pkgHTTP.ScopeConfinement{"merchantId": {}}, scope)
		assert.True(t, scope.ListsNothing())
	})
}
