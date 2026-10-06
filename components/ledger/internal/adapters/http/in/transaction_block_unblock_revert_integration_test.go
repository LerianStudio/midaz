// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"database/sql"
	nethttp "net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestIntegration_BlockUnblock_CannotBeReverted proves a block or unblock is never
// reverted on either API version: the revert is refused, nothing moves and no reversal
// is recorded, while an ordinary transaction in the same ledger still reverts.
func TestIntegration_BlockUnblock_CannotBeReverted(t *testing.T) {
	// NOT parallel: process-global huma state (see transaction_handler_v2_integration_test.go).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()
	db := infra.pgContainer.DB
	surfaces := blockUnblockSurfaces()

	revertURLs := map[string]func(txID uuid.UUID) string{
		"v1": func(txID uuid.UUID) string {
			return "/v1/organizations/" + infra.orgID.String() + "/ledgers/" + infra.ledgerID.String() + "/transactions/" + txID.String() + "/revert"
		},
		"v2": func(txID uuid.UUID) string {
			return "/v2/organizations/" + infra.orgID.String() + "/ledgers/" + infra.ledgerID.String() + "/transactions/" + txID.String() + "/revert"
		},
	}

	for _, surface := range surfaces {
		for _, action := range []string{"block", "unblock"} {
			t.Run(surface.name+"/"+action, func(t *testing.T) {
				sourceAlias := "@rv-" + surface.name + "-" + action + "-src"
				destinationAlias := "@rv-" + surface.name + "-" + action + "-dst"
				source := seedPlainBalance(t, db, infra.orgID, infra.ledgerID, sourceAlias, 100)
				destination := seedPlainBalance(t, db, infra.orgID, infra.ledgerID, destinationAlias, 0)

				created := decodeTxResponse(t, surface.post(t, infra, action, sourceAlias, destinationAlias, "40"), nethttp.StatusCreated)
				txID := uuid.MustParse(created["id"].(string))

				app := buildHumaTransactionApp(t, infra.handler, true)
				if surface.name == "v2" {
					app = buildHumaV2DirectApp(t, infra.handler)
				}

				resp := postTransaction(t, app, revertURLs[surface.name](txID), "", "")

				assert.Equal(t, cn.ErrBlockUnblockNotRevertible.Error(), decodeErrorCode(t, resp, nethttp.StatusUnprocessableEntity))
				drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
				requireDecimalEqual(t, decimal.NewFromInt(60), postgrestestutil.GetBalanceAvailable(t, db, source), "source available")
				requireDecimalEqual(t, decimal.NewFromInt(40), postgrestestutil.GetBalanceAvailable(t, db, destination), "destination available")
				assert.Zero(t, reversalCount(t, db, txID), "a refused revert must record no reversal")
			})
		}
	}

	t.Run("an ordinary transaction still reverts", func(t *testing.T) {
		seedPlainBalance(t, db, infra.orgID, infra.ledgerID, "@rv-direct-src", 100)
		seedPlainBalance(t, db, infra.orgID, infra.ledgerID, "@rv-direct-dst", 0)

		app := buildHumaTransactionApp(t, infra.handler, true)
		body := `{"description":"revertible","send":{"asset":"USD","value":"40",` +
			`"source":{"from":[{"accountAlias":"@rv-direct-src","amount":{"asset":"USD","value":"40"}}]},` +
			`"distribute":{"to":[{"accountAlias":"@rv-direct-dst","amount":{"asset":"USD","value":"40"}}]}}}`
		created := decodeTxResponse(t, postTransaction(t, app, v1JSONURL(infra.orgID, infra.ledgerID), body, ""), nethttp.StatusCreated)
		txID := uuid.MustParse(created["id"].(string))

		decodeTxResponse(t, postTransaction(t, app, revertURLs["v1"](txID), "", ""), nethttp.StatusCreated)
		assert.Equal(t, 1, reversalCount(t, db, txID))
	})
}

func reversalCount(t *testing.T, db *sql.DB, txID uuid.UUID) int {
	t.Helper()

	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM transaction WHERE parent_transaction_id = $1`, txID).Scan(&count))

	return count
}
