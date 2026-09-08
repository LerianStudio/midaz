// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// These tests cover the grant READ: what the pipeline hands to the validation and
// the script. Consumption is not asserted here because it is not done here — the
// resolver never deletes, so a request that dies after it leaves the identifier
// available for a retry.

// grantResolverFixture wires a UseCase over a mock cache plus the span and logger
// the resolver records into.
func grantResolverFixture(t *testing.T) (*UseCase, *txRedis.MockRedisRepository, context.Context) {
	t.Helper()

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	return &UseCase{TransactionRedisRepo: redisRepo}, redisRepo, context.Background()
}

// TestResolveAccountBlockExceptionGrant_AbsentIdentifierNeverReadsTheCache is the
// additive guarantee: a request that presents no identifier must not acquire a
// cache round trip, so the surface it had before the field existed is unchanged
// down to the I/O it performs. The mock has no expectation registered, so any read
// fails the test.
func TestResolveAccountBlockExceptionGrant_AbsentIdentifierNeverReadsTheCache(t *testing.T) {
	t.Parallel()

	uc, _, ctx := grantResolverFixture(t)

	grant, err := uc.resolveAccountBlockExceptionGrant(ctx, noop.Span{},
		libObservability.NewLoggerFromContext(ctx), uuid.New(), uuid.New(), nil)

	require.NoError(t, err)
	assert.Nil(t, grant, "no identifier presented means no grant carried")
}

// TestResolveAccountBlockExceptionGrant_ReadsThePresentedGrant locks what the
// resolver carries forward: the presented identifier alongside the alias and
// amount the grant was minted with.
func TestResolveAccountBlockExceptionGrant_ReadsThePresentedGrant(t *testing.T) {
	t.Parallel()

	uc, redisRepo, ctx := grantResolverFixture(t)
	orgID, ledgerID, exceptionID := uuid.New(), uuid.New(), uuid.New()

	redisRepo.EXPECT().
		GetAccountBlockException(gomock.Any(), orgID, ledgerID, exceptionID).
		Return(&mmodel.AccountBlockExceptionRedis{Alias: "@fraud_account", Amount: "150.5"}, nil).
		Times(1)

	grant, err := uc.resolveAccountBlockExceptionGrant(ctx, noop.Span{},
		libObservability.NewLoggerFromContext(ctx), orgID, ledgerID, &exceptionID)

	require.NoError(t, err)
	require.NotNil(t, grant)
	assert.Equal(t, exceptionID, grant.ID)
	assert.Equal(t, "@fraud_account", grant.Alias)
	assert.Equal(t, "150.5", grant.Amount)
}

// TestResolveAccountBlockExceptionGrant_MissRejectsBeforeAnyBalanceMoves proves
// the early rejection. Absent, already consumed and expired-by-TTL are one
// outcome to the caller — there is no grant to present — and answering it here
// costs nothing, because the identifier could not have been consumed: only the
// script consumes, and it never runs.
func TestResolveAccountBlockExceptionGrant_MissRejectsBeforeAnyBalanceMoves(t *testing.T) {
	t.Parallel()

	uc, redisRepo, ctx := grantResolverFixture(t)
	orgID, ledgerID, exceptionID := uuid.New(), uuid.New(), uuid.New()

	redisRepo.EXPECT().
		GetAccountBlockException(gomock.Any(), orgID, ledgerID, exceptionID).
		Return(nil, nil).
		Times(1)

	grant, err := uc.resolveAccountBlockExceptionGrant(ctx, noop.Span{},
		libObservability.NewLoggerFromContext(ctx), orgID, ledgerID, &exceptionID)

	require.Error(t, err)
	assert.Nil(t, grant)
	assert.Contains(t, err.Error(), constant.ErrAccountBlockExceptionInvalid.Error(),
		"a missing grant must carry the exception code, not the block code")
}

// TestResolveAccountBlockExceptionGrant_InfrastructureFailurePropagates keeps a
// cache failure distinguishable from a missing grant. Reporting a Redis outage as
// "your identifier is invalid" would send the operator to mint another exception
// against a cache that cannot store it.
func TestResolveAccountBlockExceptionGrant_InfrastructureFailurePropagates(t *testing.T) {
	t.Parallel()

	uc, redisRepo, ctx := grantResolverFixture(t)
	orgID, ledgerID, exceptionID := uuid.New(), uuid.New(), uuid.New()

	cacheDown := errors.New("redis unavailable")

	redisRepo.EXPECT().
		GetAccountBlockException(gomock.Any(), orgID, ledgerID, exceptionID).
		Return(nil, cacheDown).
		Times(1)

	grant, err := uc.resolveAccountBlockExceptionGrant(ctx, noop.Span{},
		libObservability.NewLoggerFromContext(ctx), orgID, ledgerID, &exceptionID)

	require.ErrorIs(t, err, cacheDown)
	assert.Nil(t, grant)
	assert.NotContains(t, err.Error(), constant.ErrAccountBlockExceptionInvalid.Error(),
		"a cache outage must not masquerade as an invalid identifier")
}
