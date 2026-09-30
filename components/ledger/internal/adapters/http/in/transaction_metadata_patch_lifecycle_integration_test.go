//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestMetadataPatchBeforeLifecycleCompletes PATCHes a transaction's metadata, then
// reverts it or, held, commits it. The completion re-completes the earlier execution
// over the edited document: the record persists and the money moves once.
func TestMetadataPatchBeforeLifecycleCompletes(t *testing.T) {
	h := setupFeeHarness(t)

	engineRedis, ok := h.redisRepo.(*redistransaction.RedisConsumerRepository)
	require.True(t, ok)

	h.commandUC.EngineRecoveryAcknowledger = &atomicBatchHTTPRecoveryAcknowledger{repository: engineRedis, completedAt: time.Now()}
	app := h.newV2App()

	for index, tc := range []struct {
		name string
		hold bool
	}{{"v2_revert_after_metadata_patch", false}, {"v2_commit_after_metadata_patch", true}} {
		t.Run(tc.name, func(t *testing.T) {
			hold := tc.hold
			src, dst := "@patch-src-"+string(rune('a'+index)), "@patch-dst-"+string(rune('a'+index))
			srcID := h.seedBalance(t, src, "USD", decimal.NewFromInt(1000), "deposit")
			dstID := h.seedBalance(t, dst, "USD", decimal.Zero, "deposit")
			body := strings.TrimSuffix(h.v2Body("patch", "USD", "100", []string{h.v2Leg(src, "100")}, []string{h.v2Leg(dst, "100")}), "}") +
				`,"metadata":{"origin":"frozen"}}`

			create, action := h.createV2Direct, "revert"
			if hold {
				create, action = h.createV2Hold, "commit"
			}

			created := create(t, app, body, nil)

			require.Equalf(t, 201, created.status, "create: %s", created.rawBody)
			txID := mustTxID(t, created)
			_, err := engineRedis.GetEngineTransactionIndex(h.ctx(), h.orgID, h.ledgerID, txID)
			require.NoError(t, err, "the earlier execution is still indexed, so the lifecycle re-completes it")

			patched := patchV2(t, app, v2TxByIDURL(h.orgID, h.ledgerID, txID), `{"metadata":{"client":"added"}}`)
			require.Equal(t, nethttp.StatusOK, patched.StatusCode)

			resp := h.post(t, app, h.v2StatePath(txID, action), "", nil)
			require.Equalf(t, 201, resp.status, "%s: %s", action, resp.rawBody)

			completedID, wantSrc, wantDst := mustTxID(t, resp), decimal.NewFromInt(1000), decimal.Zero
			if hold {
				wantSrc, wantDst = decimal.NewFromInt(900), decimal.NewFromInt(100)
				assert.Equal(t, cn.APPROVED, dbTxStatus(t, h.db, txID))
			} else {
				assert.Equal(t, 2, postgrestestutil.CountOperationsByTransactionID(t, h.db, completedID), "revert record persisted")
			}

			records, err := engineRedis.ReadAllRecoveryMessages(h.ctx(), redistransaction.RecoveryQueueSourceEngineRecover)
			require.NoError(t, err)
			for field := range records {
				assert.Falsef(t, strings.HasPrefix(field, completedID.String()+":"), "recovery record %s acknowledged", field)
			}

			for _, balance := range []struct {
				alias string
				id    uuid.UUID
				want  decimal.Decimal
			}{{src, srcID, wantSrc}, {dst, dstID, wantDst}} {
				live, err := h.queryUC.GetBalances(h.ctx(), h.orgID, h.ledgerID, []string{mtransaction.AliasKey(balance.alias, "default")})
				require.NoError(t, err)
				require.Len(t, live, 1)
				assert.Truef(t, live[0].Available.Equal(balance.want), "%s live %s, want %s", balance.alias, live[0].Available, balance.want)
				drainBalanceSync(t, h.ctx(), h.commandUC, h.redisRepo, h.orgID, h.ledgerID)
				stored := postgrestestutil.GetBalanceAvailable(t, h.db, balance.id)
				assert.Truef(t, stored.Equal(balance.want), "%s stored %s, want %s", balance.alias, stored, balance.want)
			}

			stored, err := h.metaRepo.FindByEntity(h.ctx(), cn.EntityTransaction, txID.String())
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.Equal(t, "added", stored.Data["client"], "the client's edit is kept")
			assert.Equal(t, "frozen", stored.Data["origin"], "the create-time key is kept")
		})
	}
}
