// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestFailRevertClaim_OnlyProvenPreMutationFailureReleases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		execution     revertExecutionState
		cause         error
		expectRelease bool
	}{
		{
			name:          "failure before balance attempt releases",
			execution:     revertExecutionState{},
			cause:         errors.New("route validation unavailable"),
			expectRelease: true,
		},
		{
			name:          "Lua business rejection proves rollback and releases",
			execution:     revertExecutionState{BalanceAttempted: true},
			cause:         pkg.ValidateBusinessError(constant.ErrInsufficientFunds, constant.EntityBalance),
			expectRelease: true,
		},
		{
			name:      "lost Lua response remains fenced for reconciliation",
			execution: revertExecutionState{BalanceAttempted: true},
			cause:     errors.New("i/o timeout after command dispatch"),
		},
		{
			name:      "failure after committed balances remains fenced for reconciliation",
			execution: revertExecutionState{BalanceAttempted: true, BalanceCommitted: true},
			cause:     errors.New("postgres persistence unavailable"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			claimRepo := revertclaim.NewMockRepository(ctrl)
			redisRepo := redis.NewMockRedisRepository(ctrl)
			claim := &revertclaim.Claim{
				OrganizationID:       uuid.New(),
				LedgerID:             uuid.New(),
				OriginTransactionID:  uuid.New(),
				ReverseTransactionID: uuid.New(),
				State:                revertclaim.StateClaimed,
			}

			if tc.expectRelease {
				redisRepo.EXPECT().Del(gomock.Any(), "legacy-fence").Return(nil)
				claimRepo.EXPECT().Release(gomock.Any(), claim.OrganizationID, claim.LedgerID,
					claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)
			} else {
				claimRepo.EXPECT().Transition(gomock.Any(), claim.OrganizationID, claim.LedgerID,
					claim.OriginTransactionID, claim.ReverseTransactionID,
					revertclaim.StateReconciliationRequired, gomock.Not(gomock.Nil())).Return(nil)
			}

			handler := &TransactionHandler{Command: &command.UseCase{
				RevertClaimRepo:      claimRepo,
				TransactionRedisRepo: redisRepo,
			}}
			err := handler.failRevertClaim(context.Background(), claim, &tc.execution, "legacy-fence", tc.cause)

			if tc.expectRelease {
				assert.ErrorIs(t, err, tc.cause)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "being reconciled")
			}
		})
	}
}

func TestMayReleaseRevertFences_AmbiguousAndPostMutationFailuresStayFenced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		execution *revertExecutionState
		cause     error
		want      bool
	}{
		{
			name: "ordinary transaction cleanup remains unchanged",
			want: true,
		},
		{
			name:      "pre movement failure releases all fences",
			execution: &revertExecutionState{},
			cause:     errors.New("route validation unavailable"),
			want:      true,
		},
		{
			name:      "Lua rejection proves rollback and releases all fences",
			execution: &revertExecutionState{BalanceAttempted: true},
			cause:     pkg.ValidateBusinessError(constant.ErrInsufficientFunds, constant.EntityBalance),
			want:      true,
		},
		{
			name:      "lost Lua response preserves postgres legacy and origin fences",
			execution: &revertExecutionState{BalanceAttempted: true},
			cause:     errors.New("i/o timeout after command dispatch"),
		},
		{
			name:      "post movement failure preserves postgres legacy and origin fences",
			execution: &revertExecutionState{BalanceAttempted: true, BalanceCommitted: true},
			cause:     errors.New("postgres persistence unavailable"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, mayReleaseRevertFences(tc.execution, tc.cause))
		})
	}
}

func TestResolveDurableRevertClaim_CrashAfterBalanceNeverStartsAnotherMutation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		State:                revertclaim.StateMutated,
	}
	transactionRepo.EXPECT().FindByParentID(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID).Return(nil, nil)

	handler := &TransactionHandler{Query: &query.UseCase{TransactionRepo: transactionRepo}}
	replay, replayed, err := handler.resolveDurableRevertClaim(context.Background(), claim)

	assert.Nil(t, replay)
	assert.False(t, replayed)
	require.Error(t, err)
	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
}

func TestReverseMatchesClaim_RequiresReservedIDAndExactOrigin(t *testing.T) {
	t.Parallel()

	claim := &revertclaim.Claim{
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
	}
	originID := claim.OriginTransactionID.String()
	matching := &transaction.Transaction{ID: claim.ReverseTransactionID.String(), ParentTransactionID: &originID}
	assert.True(t, reverseMatchesClaim(matching, claim))

	wrongOrigin := uuid.NewString()
	assert.False(t, reverseMatchesClaim(&transaction.Transaction{
		ID: claim.ReverseTransactionID.String(), ParentTransactionID: &wrongOrigin,
	}, claim))
	assert.False(t, reverseMatchesClaim(&transaction.Transaction{
		ID: uuid.NewString(), ParentTransactionID: &originID,
	}, claim))
}

func TestRevertIdempotencyModesAndKeys_DoNotAssumeOneRedisClusterSlot(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	originID := uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb503")

	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, "payload-hash")
	originKey := utils.IdempotencyInternalKey(organizationID, ledgerID,
		libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID)))
	balanceOutcomeKey := utils.TransactionInternalKey(organizationID, ledgerID, uuid.NewString())

	assert.Contains(t, legacyKey, "{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:payload-hash}")
	assert.NotEqual(t, legacyKey, originKey)
	assert.NotEqual(t, redisClusterSlot(legacyKey), redisClusterSlot(originKey),
		"combining bridge barriers in one Lua script would fail with CROSSSLOT")
	assert.Contains(t, balanceOutcomeKey, "{transactions}")
	assert.NotContains(t, originKey, "{transactions}")
	assert.Equal(t, revertIdempotencyModeBridge, (&TransactionHandler{}).activeRevertIdempotencyMode())
	assert.Equal(t, revertIdempotencyModeFinal, (&TransactionHandler{RevertIdempotencyMode: "FINAL"}).activeRevertIdempotencyMode())
}

func redisClusterSlot(key string) uint16 {
	hashInput := key
	if open := strings.IndexByte(key, '{'); open >= 0 {
		if closeOffset := strings.IndexByte(key[open+1:], '}'); closeOffset > 0 {
			hashInput = key[open+1 : open+1+closeOffset]
		}
	}

	var crc uint16
	for i := range len(hashInput) {
		crc ^= uint16(hashInput[i]) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}

	return crc % 16384
}
