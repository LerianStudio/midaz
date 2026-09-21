// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
)

// This file is the REGISTRY half of the closing contract (AC-12, AC-13, AC-19 and
// the create half of AC-20), driven through the PRODUCTION registrars of BOTH
// contracts mounted side by side — RegisterAccountRoutesToApp on /v1 and
// RegisterAccountV2RoutesToApp on /v2 — so what is pinned is the surface the
// deployed binary serves rather than a hand-assembled subset of it.
//
// The instant itself is never an input: the guard that refuses it on the way in
// lives in account_closed_at_input_test.go (AC-17/18 and the refusal half of
// AC-20). Here the instant only ever comes OUT, and the claim is that every
// registry read reports the same one, on either contract, and that a permitted
// PATCH leaves it alone.
//
// MUST-NOT-PARALLELIZE: mounting the contract swaps process-global huma state.

var (
	registryOrgID     = uuid.MustParse("dddddddd-0000-0000-0000-000000000001")
	registryLedgerID  = uuid.MustParse("dddddddd-0000-0000-0000-000000000002")
	registryAccountID = uuid.MustParse("dddddddd-0000-0000-0000-000000000003")

	registryAlias = "@closed-account"
)

// registryInstantWire is closeRouteInstant as it reaches the wire. Both spellings
// come from the same fixed time, so a drift in the marshalling shows up here
// rather than in a comparison of two independently written strings.
const registryInstantWire = "2026-09-18T10:30:00Z"

// closedRegistryAccount is the row every read in this file resolves: an account
// closed at the fixed instant, carrying the holder fields so the /v1 projection
// has something to withhold.
func closedRegistryAccount() *mmodel.Account {
	closedAt := closeRouteInstant
	holderID := uuid.MustParse("dddddddd-0000-0000-0000-000000000009").String()

	return &mmodel.Account{
		ID:                 registryAccountID.String(),
		OrganizationID:     registryOrgID.String(),
		LedgerID:           registryLedgerID.String(),
		Name:               "Closed Account",
		AssetCode:          "USD",
		Type:               "deposit",
		Alias:              testutils.Ptr(registryAlias),
		Status:             mmodel.Status{Code: "ACTIVE"},
		HolderID:           &holderID,
		HolderCheckSkipped: true,
		ClosedAt:           &closedAt,
	}
}

// mountAccountRegistrySurface mounts the account surface of BOTH contracts over one
// handler, mirroring the unified server: one Huma contract on the app root, each
// version behind its own Fiber group and Huma group, and the ErrorEnvelope on the
// root so each version answers in its own envelope.
func mountAccountRegistrySurface(t *testing.T, handler *AccountHandler) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	app.Use(ledgerMiddleware.ErrorEnvelope())

	auth := &middleware.AuthClient{Enabled: false}

	api := AssembleHumaContract(app, app, openapi.Config{
		Title: "account-registry", Version: "test", Servers: []string{"/"},
	})

	RegisterAccountRoutesToApp(app.Group("/v1"), huma.NewGroup(api, "/v1"), auth, handler, nil)
	RegisterAccountV2RoutesToApp(app.Group("/v2"), huma.NewGroup(api, "/v2"), auth, handler, nil)

	FinalizeContract(api)

	return app
}

// registryPath builds a path under the account surface of one contract.
func registryPath(version, suffix string) string {
	return "/" + version + "/organizations/" + registryOrgID.String() +
		"/ledgers/" + registryLedgerID.String() + "/accounts" + suffix
}

// sendRegistryRequest issues one request and decodes the JSON body.
func sendRegistryRequest(t *testing.T, app *fiber.App, method, path string, body []byte) (int, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded), "body: %s", string(raw))

	return resp.StatusCode, decoded
}

// TestAccountClosingRegistryReads_AgreeOnTheInstant covers AC-13: by id, by alias
// and in a listing, on either contract, a closed account reports the SAME instant.
// The version difference that survives is the pre-existing one — /v1 withholds the
// holder fields — and closedAt is deliberately not part of it.
func TestAccountClosingRegistryReads_AgreeOnTheInstant(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	accountRepo := account.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	accountRepo.EXPECT().Find(gomock.Any(), registryOrgID, registryLedgerID, gomock.Nil(), registryAccountID, gomock.Any()).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, uuid.UUID, mmodel.HolderPolicy) (*mmodel.Account, error) {
			return closedRegistryAccount(), nil
		}).AnyTimes()
	accountRepo.EXPECT().FindAlias(gomock.Any(), registryOrgID, registryLedgerID, gomock.Nil(), registryAlias, gomock.Any()).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string, mmodel.HolderPolicy) (*mmodel.Account, error) {
			return closedRegistryAccount(), nil
		}).AnyTimes()
	accountRepo.EXPECT().FindAll(gomock.Any(), registryOrgID, registryLedgerID, gomock.Nil(), gomock.Nil(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, *uuid.UUID, pkgHTTP.QueryHeader, mmodel.HolderPolicy) ([]*mmodel.Account, error) {
			return []*mmodel.Account{closedRegistryAccount()}, nil
		}).AnyTimes()
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), cn.EntityAccount, gomock.Any()).Return(nil, nil).AnyTimes()
	metadataRepo.EXPECT().FindByEntityIDs(gomock.Any(), cn.EntityAccount, gomock.Any()).Return(nil, nil).AnyTimes()

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo}}
	app := mountAccountRegistrySurface(t, handler)

	reads := []struct {
		name   string
		suffix string
		// list reads answer a pagination envelope, so the account is one item deep.
		fromPage bool
	}{
		{name: "by id", suffix: "/" + registryAccountID.String()},
		{name: "by alias", suffix: "/alias/" + registryAlias},
		{name: "in a listing", suffix: "?limit=10&page=1", fromPage: true},
	}

	for _, read := range reads {
		for _, version := range []string{"v1", "v2"} {
			t.Run(read.name+"_"+version, func(t *testing.T) {
				status, body := sendRegistryRequest(t, app, http.MethodGet, registryPath(version, read.suffix), nil)
				require.Equal(t, http.StatusOK, status)

				got := body
				if read.fromPage {
					items, ok := body["items"].([]any)
					require.True(t, ok, "the listing must answer a page of items")
					require.Len(t, items, 1)

					got, ok = items[0].(map[string]any)
					require.True(t, ok)
				}

				require.Contains(t, got, "closedAt", "%s must publish closedAt", version)
				assert.Equal(t, registryInstantWire, got["closedAt"],
					"both contracts report the instant the row carries")

				if version == "v1" {
					assert.NotContains(t, got, "holderId", "/v1 withholds the holder seam")
					assert.NotContains(t, got, "holderCheckSkipped", "/v1 withholds the holder seam")
				} else {
					assert.Contains(t, got, "holderId", "/v2 keeps its own fields")
					assert.Contains(t, got, "holderCheckSkipped")
				}
			})
		}
	}
}

// TestAccountClosingRegistryCreate_ReportsANullInstant covers the second half of
// AC-20: a create that names neither spelling is accepted, and the account it
// produces is OPEN — closedAt is present on the wire and null, not absent.
func TestAccountClosingRegistryCreate_ReportsANullInstant(t *testing.T) {
	// NOT parallel: process-global huma state.
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			accountRepo := account.NewMockRepository(ctrl)
			assetRepo := asset.NewMockRepository(ctrl)
			metadataRepo := mongodb.NewMockRepository(ctrl)
			balanceRepo := balance.NewMockRepository(ctrl)
			ledgerRepo := ledger.NewMockRepository(ctrl)

			ledgerRepo.EXPECT().GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), registryOrgID, registryLedgerID, "", "USD").Return(true, nil).Times(1)
			accountRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, acc *mmodel.Account) (*mmodel.Account, error) {
					assert.Nil(t, acc.ClosedAt, "a created account carries no closing instant")

					acc.ID = registryAccountID.String()
					acc.OrganizationID = registryOrgID.String()
					acc.LedgerID = registryLedgerID.String()
					acc.CreatedAt = fixedTestTime
					acc.UpdatedAt = fixedTestTime

					return acc, nil
				}).Times(1)
			balanceRepo.EXPECT().ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
			balanceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
			metadataRepo.EXPECT().Create(gomock.Any(), cn.EntityAccount, gomock.Any()).Return(nil).AnyTimes()

			handler := &AccountHandler{Command: &command.UseCase{
				AccountRepo:            accountRepo,
				AssetRepo:              assetRepo,
				OnboardingMetadataRepo: metadataRepo,
				BalanceRepo:            balanceRepo,
				LedgerRepo:             ledgerRepo,
			}}

			app := mountAccountRegistrySurface(t, handler)

			body, err := json.Marshal(map[string]any{"name": "Treasury", "assetCode": "USD", "type": "deposit"})
			require.NoError(t, err)

			status, got := sendRegistryRequest(t, app, http.MethodPost, registryPath(version, ""), body)
			require.Equal(t, http.StatusCreated, status)

			require.Contains(t, got, "closedAt", "an open account still publishes the key")
			assert.Nil(t, got["closedAt"], "a freshly created account is open")
		})
	}
}

// registryPatchHandler wires the command and the query over one account repository
// whose row is the closed account, so the PATCH mutates and then re-reads the same
// fixture. The Update expectation is where the durable claim sits: the entity the
// command hands the repository carries NO instant, so there is nothing for the
// generic UPDATE to write back.
func registryPatchHandler(t *testing.T, blocked bool) *AccountHandler {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	accountRepo := account.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	accountRepo.EXPECT().Find(gomock.Any(), registryOrgID, registryLedgerID, gomock.Nil(), registryAccountID, gomock.Any()).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, uuid.UUID, mmodel.HolderPolicy) (*mmodel.Account, error) {
			return closedRegistryAccount(), nil
		}).AnyTimes()
	accountRepo.EXPECT().Update(gomock.Any(), registryOrgID, registryLedgerID, gomock.Nil(), registryAccountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
			assert.Nil(t, acc.ClosedAt, "the update entity must carry no closing instant")

			updated := closedRegistryAccount()
			updated.Name = acc.Name

			return updated, nil
		}).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), cn.EntityAccount, gomock.Any()).Return(nil, nil).AnyTimes()
	metadataRepo.EXPECT().Update(gomock.Any(), cn.EntityAccount, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	if blocked {
		balanceRepo.EXPECT().ListByAccountID(gomock.Any(), registryOrgID, registryLedgerID, registryAccountID).
			Return([]*mmodel.Balance{{Alias: registryAlias, Key: cn.DefaultBalanceKey}}, nil).Times(1)
		redisRepo.EXPECT().UpdateBalanceCacheBlocked(gomock.Any(), registryOrgID, registryLedgerID, gomock.Any(), false).
			Return(nil).Times(1)
	}

	return &AccountHandler{
		Command: &command.UseCase{
			AccountRepo:            accountRepo,
			OnboardingMetadataRepo: metadataRepo,
			BalanceRepo:            balanceRepo,
			TransactionRedisRepo:   redisRepo,
		},
		Query: &query.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo},
	}
}

// TestAccountClosingRegistryPatch_PreservesTheInstant covers AC-19 and the registry
// half of AC-12: a permitted PATCH — a rename, or the unblocking that propagates
// Blocked to the cached balances — follows its normal contract, and the response
// the caller reads back still carries the same instant. Unblocking is not a
// reopening.
func TestAccountClosingRegistryPatch_PreservesTheInstant(t *testing.T) {
	// NOT parallel: process-global huma state.
	patches := []struct {
		name    string
		body    map[string]any
		blocked bool
	}{
		{name: "rename", body: map[string]any{"name": "Renamed while closed"}},
		{name: "unblock", body: map[string]any{"blocked": false}, blocked: true},
	}

	for _, patch := range patches {
		for _, version := range []string{"v1", "v2"} {
			t.Run(patch.name+"_"+version, func(t *testing.T) {
				handler := registryPatchHandler(t, patch.blocked)
				app := mountAccountRegistrySurface(t, handler)

				body, err := json.Marshal(patch.body)
				require.NoError(t, err)

				status, got := sendRegistryRequest(t, app, http.MethodPatch,
					registryPath(version, "/"+registryAccountID.String()), body)
				require.Equal(t, http.StatusOK, status)

				require.Contains(t, got, "closedAt")
				assert.Equal(t, registryInstantWire, got["closedAt"],
					"a permitted PATCH neither clears nor moves the instant")
			})
		}
	}
}
