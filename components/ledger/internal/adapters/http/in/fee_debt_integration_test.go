//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/fee_debt"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	services "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

// buildFeeDebtApp mounts the /v2 fee-debt reads through the production registrar over
// a real MongoDB projection holding three debts of one ledger, oldest first. It returns
// the app, the list path and the debt ids.
func buildFeeDebtApp(t *testing.T) (*fiber.App, string, []string) {
	t.Helper()

	container := mongotestutil.SetupReusableContainer(t)
	repo, err := fee_debt.NewRepository(&feesmongo.MongoConnection{Database: container.DBName, DB: container.Client}, nil)
	require.NoError(t, err)

	orgID, ledgerID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	appliedAt := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	debtors := []string{"@payer#default", "@other#default", "@payer#default"}
	ids := make([]string, len(debtors))

	for i, debtor := range debtors {
		origin := uuid.MustParse(fmt.Sprintf("01920000-0000-7000-8000-%012d", i+1))
		ids[i] = origin.String() + ":from:1:debit"

		require.NoError(t, repo.Apply(context.Background(), command.FeeDebtRecord{
			OrganizationID: orgID, LedgerID: ledgerID, AppliedAt: appliedAt,
			Changes: []accounting.FeeDebtChange{{
				TransactionID: origin, PostingRef: "from:1:debit", Kind: accounting.FeeDebtOpened, DebtID: ids[i],
				DebtorRef: debtor, CreditRef: "@fees#default", OriginTransactionID: origin,
				Seq: int64(i + 1), AssetCode: "BRL", Amount: decimal.RequireFromString("12.5"), Opened: decimal.RequireFromString("12.5"),
			}},
		}))
	}

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	libProblem.Install()

	apiV2 := app.Group("/v2")
	hAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-fee-debts", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)
	RegisterFeeDebtV2RoutesToApp(apiV2, hAPI, &middleware.AuthClient{Enabled: false},
		&FeeDebtHandler{Service: &services.FeeDebtService{Repo: repo}}, nil)

	return app, fmt.Sprintf("/v2/organizations/%s/ledgers/%s/fee-debts", orgID, ledgerID), ids
}

func TestFeeDebtRoutes(t *testing.T) {
	app, base, ids := buildFeeDebtApp(t)

	itemIDs := func(body map[string]any) []string {
		var got []string
		for _, item := range body["items"].([]any) {
			got = append(got, item.(map[string]any)["id"].(string))
		}

		return got
	}

	t.Run("pages oldest first", func(t *testing.T) {
		status, page1 := driveFeeV2(t, app, http.MethodGet, base+"?limit=2", "")
		require.Equal(t, http.StatusOK, status, page1)
		assert.Equal(t, ids[0:2], itemIDs(page1))
		assert.Equal(t, "12.5", page1["items"].([]any)[0].(map[string]any)["remaining"])

		status, page2 := driveFeeV2(t, app, http.MethodGet, base+"?limit=2&cursor="+url.QueryEscape(page1["next_cursor"].(string)), "")
		require.Equal(t, http.StatusOK, status, page2)
		assert.Equal(t, ids[2:3], itemIDs(page2))
		assert.Empty(t, page2["next_cursor"])
	})

	t.Run("filters by debtor account", func(t *testing.T) {
		status, body := driveFeeV2(t, app, http.MethodGet, base+"?account_alias=%40payer", "")
		require.Equal(t, http.StatusOK, status, body)
		assert.Equal(t, []string{ids[0], ids[2]}, itemIDs(body))
	})

	t.Run("gets one debt whether its colons are encoded or not", func(t *testing.T) {
		for _, segment := range []string{ids[1], strings.ReplaceAll(ids[1], ":", "%3A")} {
			status, body := driveFeeV2(t, app, http.MethodGet, base+"/"+segment, "")
			require.Equal(t, http.StatusOK, status, body)
			assert.Equal(t, ids[1], body["id"])
			assert.Equal(t, "@other#default", body["debtorBalance"])
		}
	})

	scope := strings.TrimSuffix(base, "/fee-debts")
	otherLedger := scope[:strings.LastIndex(scope, "/")+1] + uuid.NewString() + "/fee-debts"

	rejections := []struct {
		name, url, code string
		status          int
	}{
		{"limit over the maximum", base + "?limit=101", "0080", http.StatusBadRequest},
		{"non-numeric limit", base + "?limit=ten", "0082", http.StatusBadRequest},
		{"balance key without an account", base + "?balance_key=default", "0082", http.StatusBadRequest},
		{"undecodable cursor", base + "?cursor=not-a-cursor", "0082", http.StatusBadRequest},
		{"unknown debt", base + "/" + uuid.NewString() + ":from:1:debit", "0007", http.StatusNotFound},
		{"debt of another ledger", otherLedger + "/" + ids[0], "0007", http.StatusNotFound},
	}

	for _, tc := range rejections {
		t.Run(tc.name, func(t *testing.T) {
			status, body := driveFeeV2(t, app, http.MethodGet, tc.url, "")
			assert.Equal(t, tc.status, status, body)
			assert.Equal(t, tc.code, body["code"], body)
		})
	}
}
