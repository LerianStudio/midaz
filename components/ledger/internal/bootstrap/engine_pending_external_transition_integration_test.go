//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net/http"
	"strings"
	"testing"

	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestIntegrationEnginePendingTransitionRefusesDurableHoldClosedOutsideTheEngine
// closes a durable hold the way a release without the engine guard does, by
// writing only its primary row, and leaves the engine index and guard at
// PENDING. The opposite transition must be refused from the row before the
// engine runs, while a hold whose row is still PENDING transitions normally.
func TestIntegrationEnginePendingTransitionRefusesDurableHoldClosedOutsideTheEngine(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	infra.command.TransactionWriteBehindAsync = false
	app := infra.newHTTPApp("")

	for _, testCase := range []struct {
		version   string
		rowStatus string
		action    string
	}{
		{version: "v1", rowStatus: constant.APPROVED, action: "cancel"},
		{version: "v1", rowStatus: constant.CANCELED, action: "commit"},
		{version: "v2", rowStatus: constant.APPROVED, action: "cancel"},
		{version: "v2", rowStatus: constant.CANCELED, action: "commit"},
		{version: "v1", rowStatus: constant.PENDING, action: "commit"},
		{version: "v2", rowStatus: constant.PENDING, action: "cancel"},
	} {
		t.Run(testCase.version+" "+testCase.action+" over "+testCase.rowStatus+" row", func(t *testing.T) {
			ctx := context.Background()
			aliases := infra.seedTransfer(t, "external-"+testCase.version+"-"+strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
			created := infra.postPendingCreate(t, app, testCase.version, aliases)
			require.Equalf(t, http.StatusCreated, created.status, "pending create must return 201: %s", created.body)
			transactionID := created.transactionID(t)
			infra.requireBalance(t, ctx, aliases.source, 900, 100)

			resolved, err := infra.query.ResolveEngineWriteBehindTransaction(ctx, infra.organization, infra.ledger, transactionID)
			require.NoError(t, err)
			require.NotEqual(t, uuid.Nil, resolved.ExecutionID, "the hold must still be indexed")
			require.False(t, resolved.Pending, "the hold's execution must already be durable")

			_, err = infra.db.ExecContext(ctx, `UPDATE transaction SET status = $1 WHERE id = $2`, testCase.rowStatus, transactionID)
			require.NoError(t, err)

			transition := infra.postTransition(t, app, testCase.version, transactionID, testCase.action)

			guardKey, err := tmvalkey.GetKeyContext(ctx, "engine:"+cachepolicy.HashTag+":guards:"+infra.organization.String()+":"+infra.ledger.String())
			require.NoError(t, err)

			if testCase.rowStatus == constant.PENDING {
				require.Equalf(t, http.StatusCreated, transition.status, "%s over a pending row must succeed: %s", testCase.action, transition.body)
				assert.NotEqual(t, constant.PENDING, infra.redis.HGet(ctx, guardKey, transactionID.String()).Val())

				return
			}

			require.Equalf(t, http.StatusConflict, transition.status, "%s over a %s row must be refused: %s", testCase.action, testCase.rowStatus, transition.body)
			assert.Equal(t, constant.ErrCommitTransactionNotPending.Error(), transition.decoded["code"])
			// A commit would clear the source's hold and a cancel would also
			// return it to available, so an untouched source proves neither ran.
			infra.requireBalance(t, ctx, aliases.source, 900, 100)
			assert.Equal(t, constant.PENDING, infra.redis.HGet(ctx, guardKey, transactionID.String()).Val(),
				"a refused transition must leave the engine guard untouched")
		})
	}
}
