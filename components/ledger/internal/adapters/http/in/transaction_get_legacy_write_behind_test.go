// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// unindexedEngineWriteBehindRepo is an engine index that holds no entry, or
// whose reads fail with indexErr.
type unindexedEngineWriteBehindRepo struct {
	indexErr error
}

func (repo unindexedEngineWriteBehindRepo) GetEngineTransactionIndex(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) ([]byte, error) {
	if repo.indexErr != nil {
		return nil, repo.indexErr
	}

	return nil, redis.ErrEngineWriteBehindNotFound
}

func (unindexedEngineWriteBehindRepo) GetEngineTransactionEvidence(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) ([]byte, []byte, error) {
	return nil, nil, errors.New("no evidence behind an absent index")
}

func (unindexedEngineWriteBehindRepo) GetEngineMaterializedTransaction(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*redis.EngineMaterializedTransaction, error) {
	return nil, redis.ErrEngineWriteBehindNotFound
}

func (unindexedEngineWriteBehindRepo) MaterializeEngineTransaction(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, []byte, time.Duration) (bool, error) {
	return false, errors.New("nothing to materialize behind an absent index")
}

type legacyWriteBehindGetFixture struct {
	orgID, ledgerID, tranID uuid.UUID
	transactionRepo         *transaction.MockRepository
	redisRepo               *redis.MockRedisRepository
	app                     *fiber.App
}

func newLegacyWriteBehindGetFixture(t *testing.T, index unindexedEngineWriteBehindRepo) legacyWriteBehindGetFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	fixture := legacyWriteBehindGetFixture{
		orgID:           uuid.Must(libCommons.GenerateUUIDv7()),
		ledgerID:        uuid.Must(libCommons.GenerateUUIDv7()),
		tranID:          uuid.Must(libCommons.GenerateUUIDv7()),
		transactionRepo: transaction.NewMockRepository(ctrl),
		redisRepo:       redis.NewMockRedisRepository(ctrl),
	}
	queryUC := &query.UseCase{
		TransactionRepo: fixture.transactionRepo, TransactionRedisRepo: fixture.redisRepo,
		EngineWriteBehindRepo: index, EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{},
	}
	fixture.app = buildHumaTransactionApp(t, &TransactionHandler{Command: &command.UseCase{TransactionReader: queryUC}, Query: queryUC}, true)

	return fixture
}

func (fixture legacyWriteBehindGetFixture) expectPrimaryMiss() {
	fixture.transactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), fixture.orgID, fixture.ledgerID, fixture.tranID).
		Return(&transaction.Transaction{}, nil).
		Times(1)
	fixture.transactionRepo.EXPECT().
		Find(gomock.Any(), fixture.orgID, fixture.ledgerID, fixture.tranID).
		Return(nil, pkg.ValidateBusinessError(cn.ErrEntityNotFound, cn.EntityTransaction)).
		Times(1)
}

func (fixture legacyWriteBehindGetFixture) legacyKey() string {
	return utils.WriteBehindTransactionKey(fixture.orgID, fixture.ledgerID, fixture.tranID.String())
}

func (fixture legacyWriteBehindGetFixture) get(t *testing.T) (*http.Response, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, humaTransactionURL(fixture.orgID, fixture.ledgerID, "/"+fixture.tranID.String()), nil)
	resp, err := fixture.app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoErrorf(t, json.Unmarshal(body, &decoded), "response must be JSON: %s", body)

	return resp, decoded
}

func TestGetTransaction_UnprojectedAnnotationIsServedFromTheLegacyEntry(t *testing.T) {
	fixture := newLegacyWriteBehindGetFixture(t, unindexedEngineWriteBehindRepo{})
	fixture.expectPrimaryMiss()

	noted := newTestTransactionData(fixture.orgID, fixture.ledgerID, fixture.tranID)
	noted.Status = transaction.Status{Code: cn.NOTED}
	entry, err := msgpack.Marshal(noted)
	require.NoError(t, err)
	fixture.redisRepo.EXPECT().GetBytes(gomock.Any(), fixture.legacyKey()).Return(entry, nil).Times(1)

	resp, decoded := fixture.get(t)

	require.Equal(t, http.StatusOK, resp.StatusCode, decoded)
	assert.Equal(t, fixture.tranID.String(), decoded["id"])
	status, ok := decoded["status"].(map[string]any)
	require.Truef(t, ok, "response must carry a status object: %v", decoded)
	assert.Equal(t, cn.NOTED, status["code"])
	assert.Equal(t, "true", resp.Header.Get("X-Cache-Hit"))
}

func TestGetTransaction_UnknownTransactionIsNotFoundAfterEverySource(t *testing.T) {
	fixture := newLegacyWriteBehindGetFixture(t, unindexedEngineWriteBehindRepo{})
	fixture.expectPrimaryMiss()
	fixture.redisRepo.EXPECT().GetBytes(gomock.Any(), fixture.legacyKey()).Return(nil, errors.New("cache miss")).Times(1)

	resp, decoded := fixture.get(t)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, cn.ErrEntityNotFound.Error(), decoded["code"])
}

func TestGetTransaction_PersistedTransactionIsNeverReadFromTheLegacyEntry(t *testing.T) {
	fixture := newLegacyWriteBehindGetFixture(t, unindexedEngineWriteBehindRepo{})
	persisted := newTestTransactionData(fixture.orgID, fixture.ledgerID, fixture.tranID)
	fixture.transactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), fixture.orgID, fixture.ledgerID, fixture.tranID).
		Return(persisted, nil).
		Times(1)
	fixture.redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Times(0)

	resp, decoded := fixture.get(t)

	require.Equal(t, http.StatusOK, resp.StatusCode, decoded)
	assert.Equal(t, fixture.tranID.String(), decoded["id"])
	assert.Equal(t, "false", resp.Header.Get("X-Cache-Hit"))
}

func TestGetTransaction_EngineIndexFailureIsNotAnAbsence(t *testing.T) {
	fixture := newLegacyWriteBehindGetFixture(t, unindexedEngineWriteBehindRepo{indexErr: errors.New("connection refused")})
	fixture.transactionRepo.EXPECT().FindWithOperations(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	fixture.redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Times(0)

	resp, decoded := fixture.get(t)

	assert.GreaterOrEqual(t, resp.StatusCode, http.StatusInternalServerError, decoded)
}
