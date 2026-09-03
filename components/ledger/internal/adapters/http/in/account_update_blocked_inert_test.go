// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// =============================================================================
// PATCH /accounts — THE RETIRED blocked KEY OVER REAL HTTP
// =============================================================================
// Two facts have to hold together, and only an end-to-end request can show both:
//
//  1. `"blocked": true` still returns 200. The body decoder
//     (pkgHTTP.DecodeAndValidate) rejects unknown keys with a 400, so this is
//     NOT free — it holds only because the field is still declared on
//     UpdateAccountInput. Deleting it would break every client still sending it.
//  2. It changes nothing. No block write reaches the account row, and the
//     enforcement index and the legacy projection are never touched.
//
// The block state of an account moves through POST /block and /unblock only,
// which carry their own authorization resource.

// patchInertRepos are the ports a block transition would reach. The balance and
// redis mocks are handed to the use case with NOTHING armed, so any attempt to
// propagate a block fails the gomock controller.
type patchInertRepos struct {
	accountRepo *account.MockRepository
	balanceRepo *balance.MockRepository
	redisRepo   *redis.MockRedisRepository

	updatedBlocked []*bool
}

// newPatchInertHandler builds an AccountHandler over the REAL update command,
// backed by mocks, with a pre-existing account in the given block state.
func newPatchInertHandler(t *testing.T, orgID, ledgerID, accountID uuid.UUID, currentlyBlocked bool) (*AccountHandler, *patchInertRepos) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	repos := &patchInertRepos{
		accountRepo: account.NewMockRepository(ctrl),
		balanceRepo: balance.NewMockRepository(ctrl),
		redisRepo:   redis.NewMockRedisRepository(ctrl),
	}

	blocked := currentlyBlocked

	pre := &mmodel.Account{
		ID:             accountID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Name:           "Patchable Account",
		Type:           "deposit",
		AssetCode:      "USD",
		Blocked:        &blocked,
		UpdatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	repos.accountRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, nil, accountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, _ mmodel.HolderPolicy) (*mmodel.Account, error) {
			out := *pre
			return &out, nil
		}).AnyTimes()

	repos.accountRepo.EXPECT().
		Update(gomock.Any(), orgID, ledgerID, gomock.Any(), accountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, in *mmodel.Account) (*mmodel.Account, error) {
			repos.updatedBlocked = append(repos.updatedBlocked, in.Blocked)

			out := *in
			out.ID = accountID.String()
			out.UpdatedAt = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

			return &out, nil
		}).Times(1)

	metadataRepo := mongodb.NewMockRepository(ctrl)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mongodb.Metadata{Data: map[string]any{}}, nil).AnyTimes()
	metadataRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	handler := &AccountHandler{
		Command: &command.UseCase{
			AccountRepo:            repos.accountRepo,
			BalanceRepo:            repos.balanceRepo,
			TransactionRedisRepo:   repos.redisRepo,
			OnboardingMetadataRepo: metadataRepo,
			Streaming:              pkgStreaming.NewMockEmitter(),
		},
		// The handler re-reads the account after the update, so the response body
		// carries what the repository actually holds — which is what makes the
		// blocked assertion below meaningful rather than an echo of the request.
		Query: &query.UseCase{
			AccountRepo:            repos.accountRepo,
			OnboardingMetadataRepo: metadataRepo,
		},
	}

	return handler, repos
}

// buildHumaAccountPatchApp mounts one version group carrying the account surface,
// mirroring the production wiring the block app test uses.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps a process-global huma hook.
func buildHumaAccountPatchApp(t *testing.T, handler *AccountHandler, version string) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	libProblem.Install()

	f.Use(ledgerMiddleware.ErrorEnvelope())

	group := f.Group(version)
	api := openapi.New(f, group, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{version}})

	group.Patch("/organizations/:organization_id/ledgers/:ledger_id/accounts/:id",
		pkgHTTP.ParseUUIDPathParameters("account"))

	if version == "/v2" {
		RegisterAccountV2Routes(api, handler, v2OpSuffix)
	} else {
		RegisterAccountRoutes(api, handler, v1OpSuffix)
	}

	return f
}

// TestAccountPatch_BlockedKeyIsAcceptedAndIgnored drives the retired key over a
// real request in both directions and on both version groups.
func TestAccountPatch_BlockedKeyIsAcceptedAndIgnored(t *testing.T) {
	// NOT parallel: process-global huma state.
	cases := []struct {
		name             string
		version          string
		currentlyBlocked bool
		body             string
	}{
		{
			name:             "v1 cannot block",
			version:          "/v1",
			currentlyBlocked: false,
			body:             `{"name":"Renamed","blocked":true}`,
		},
		{
			name:             "v1 cannot unblock",
			version:          "/v1",
			currentlyBlocked: true,
			body:             `{"name":"Renamed","blocked":false}`,
		},
		{
			name:             "v2 cannot block",
			version:          "/v2",
			currentlyBlocked: false,
			body:             `{"name":"Renamed","blocked":true}`,
		},
		{
			name:             "v2 cannot unblock",
			version:          "/v2",
			currentlyBlocked: true,
			body:             `{"name":"Renamed","blocked":false}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
			accountID := uuid.Must(libCommons.GenerateUUIDv7())

			handler, repos := newPatchInertHandler(t, orgID, ledgerID, accountID, tc.currentlyBlocked)
			app := buildHumaAccountPatchApp(t, handler, tc.version)

			req := httptest.NewRequest(http.MethodPatch, tc.version+"/organizations/"+orgID.String()+
				"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			respBody, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)

			// The decoder rejects unknown keys with a 400, so a 200 here is the
			// proof that the field is still declared and clients are not broken.
			require.Equal(t, http.StatusOK, resp.StatusCode,
				"a request still sending the retired blocked key must not be rejected; body: %s", string(respBody))

			require.Len(t, repos.updatedBlocked, 1)
			assert.Nil(t, repos.updatedBlocked[0],
				"the retired field must never reach the account row's update payload")

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))

			assert.Equal(t, tc.currentlyBlocked, got["blocked"],
				"the response must report the block state the account already had, not the one the payload asked for")
		})
	}
}
