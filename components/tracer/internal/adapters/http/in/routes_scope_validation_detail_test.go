// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// storedValidations answers the validations a test seeded, by id.
type storedValidations map[uuid.UUID]*model.TransactionValidation

func (s storedValidations) GetByID(_ context.Context, id uuid.UUID) (*model.TransactionValidation, error) {
	if v, found := s[id]; found {
		return v, nil
	}

	return nil, pkgConstant.ErrTransactionValidationNotFound
}

// scopedPartnerAuthz is an authorization service deciding for a partner scoped on
// the dimensions it is reset with: a question is allowed when every scoped
// dimension it names carries an allowed value, and every scoped dimension it does
// not name is pending resolution. denyAll refuses every question, a credential
// without the grant.
type scopedPartnerAuthz struct {
	mu        sync.Mutex
	scope     map[string][]string
	denyAll   bool
	questions []map[string]string
}

func newScopedPartnerAuthz(t *testing.T) (*scopedPartnerAuthz, *httptest.Server) {
	t.Helper()

	authz := &scopedPartnerAuthz{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
			Pending    []string          `json:"pending"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("scoped partner authz: decode authorization body: %v", err)
		}

		authz.mu.Lock()
		authz.questions = append(authz.questions, body.Attributes)
		allowed := !authz.denyAll

		for dimension, values := range authz.scope {
			value, named := body.Attributes[dimension]

			switch {
			case named && !slices.Contains(values, value):
				allowed = false
			case !named && !slices.Contains(body.Pending, dimension):
				allowed = false
			}
		}
		authz.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if allowed {
			_, _ = w.Write([]byte(`{"authorized":true}`))

			return
		}

		_, _ = w.Write([]byte(`{"authorized":false}`))
	}))

	t.Cleanup(server.Close)

	return authz, server
}

func (a *scopedPartnerAuthz) reset(scope map[string][]string, denyAll bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.scope, a.denyAll, a.questions = scope, denyAll, nil
}

// asked returns every value the questions named for a dimension.
func (a *scopedPartnerAuthz) asked(dimension string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	var out []string

	for _, question := range a.questions {
		if value, named := question[dimension]; named && !slices.Contains(out, value) {
			out = append(out, value)
		}
	}

	return out
}

// TestTracerScope_ValidationDetailResolvesItsAccount drives GET /v1/validations/{id}
// through the real guard and the embedded manifest: the validation is read for the
// account, segment, portfolio and merchant it was submitted for, so a partner reads
// the validations of its own scope and nothing else.
func TestTracerScope_ValidationDetailResolvesItsAccount(t *testing.T) {
	a1, a2 := uuid.New(), uuid.New()
	segment, portfolio, merchant := uuid.New(), uuid.New(), uuid.New()
	ofA1, ofA2, placed, unknown := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	stored := storedValidations{
		ofA1: {ID: ofA1, Account: model.AccountContext{ID: a1}},
		ofA2: {ID: ofA2, Account: model.AccountContext{ID: a2}},
		placed: {
			ID: placed, Account: model.AccountContext{ID: a2},
			Segment:   &model.SegmentContext{ID: segment},
			Portfolio: &model.PortfolioContext{ID: portfolio},
			Merchant:  &model.MerchantContext{ID: merchant},
		},
	}

	authz, server := newScopedPartnerAuthz(t)

	authClient := &authMiddleware.AuthClient{Enabled: true, Address: server.URL}
	wireTracerScope(t, authClient, stored)

	deps := newTestRouterDeps(t, middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName})
	deps.TransactionValidationService.EXPECT().GetTransactionValidation(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id uuid.UUID) (*model.TransactionValidation, error) {
			return stored.GetByID(context.Background(), id)
		}).AnyTimes()

	app := deps.buildWithAuthClient(authClient)

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type": "application", "owner": "scope-org", "sub": "scope-org/scope-app", "partner": "scope-partner",
	}).SignedString([]byte("scope-secret"))
	require.NoError(t, err)

	read := func(id uuid.UUID) int {
		t.Helper()

		req := httptest.NewRequest(fiber.MethodGet, "/v1/validations/"+id.String(), nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		return resp.StatusCode
	}

	accountPartner := map[string][]string{"accountId": {a1.String()}}

	t.Run("an account partner reads a validation of its account", func(t *testing.T) {
		authz.reset(accountPartner, false)

		assert.Equal(t, fiber.StatusOK, read(ofA1))
		assert.Equal(t, []string{a1.String()}, authz.asked("accountId"), "the account the validation was submitted for")
	})

	t.Run("an account partner is refused a validation of another account", func(t *testing.T) {
		authz.reset(accountPartner, false)

		assert.Equal(t, fiber.StatusForbidden, read(ofA2))
		assert.Equal(t, []string{a2.String()}, authz.asked("accountId"))
	})

	t.Run("an unknown validation is refused like one out of scope", func(t *testing.T) {
		authz.reset(accountPartner, false)

		assert.Equal(t, fiber.StatusForbidden, read(unknown))
		assert.Empty(t, authz.asked("accountId"), "nothing resolved, nothing asked")
	})

	t.Run("an unrestricted partner reads any validation", func(t *testing.T) {
		authz.reset(nil, false)

		assert.Equal(t, fiber.StatusOK, read(ofA2))
		assert.Equal(t, []string{a2.String()}, authz.asked("accountId"))
	})

	t.Run("an unrestricted partner is refused an unknown validation, its existence untold", func(t *testing.T) {
		authz.reset(nil, false)

		assert.Equal(t, fiber.StatusForbidden, read(unknown))
	})

	t.Run("a credential without the grant is refused", func(t *testing.T) {
		authz.reset(nil, true)

		assert.Equal(t, fiber.StatusForbidden, read(ofA1))
	})

	for _, dimension := range []struct {
		name  string
		value uuid.UUID
	}{{"segmentId", segment}, {"portfolioId", portfolio}, {"merchantId", merchant}} {
		t.Run("a "+dimension.name+" partner reads a validation submitted under it", func(t *testing.T) {
			authz.reset(map[string][]string{dimension.name: {dimension.value.String()}}, false)

			assert.Equal(t, fiber.StatusOK, read(placed))
			assert.Equal(t, []string{dimension.value.String()}, authz.asked(dimension.name))
		})

		t.Run("a "+dimension.name+" partner is refused a validation submitted without it", func(t *testing.T) {
			authz.reset(map[string][]string{dimension.name: {dimension.value.String()}}, false)

			assert.Equal(t, fiber.StatusForbidden, read(ofA1))
			assert.Empty(t, authz.asked(dimension.name))
		})
	}
}
