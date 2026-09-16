// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestCreateAtomicTransactionBatchV2_RejectsSingularBody(t *testing.T) {
	app := buildHumaV2DirectApp(t, &TransactionHandler{TransactionBatchMaxSize: 50})
	body := mustMarshalAtomicBatchV2Item(t, validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID))

	resp := postAtomicBatchV2(t, app, body, nil)
	defer func() { _ = resp.Body.Close() }()

	detail := decodeAtomicBatchHTTPProblem(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), detail.Code)
}

func TestCreateTransactionDirectV2_RejectsBatchWrapper(t *testing.T) {
	app := buildHumaV2DirectApp(t, &TransactionHandler{})
	item := mustMarshalAtomicBatchV2Item(t, validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID))
	wrapper := marshalAtomicBatchV2Wrapper(t, item)

	resp := postDirectV2(t, app, string(wrapper))
	defer func() { _ = resp.Body.Close() }()

	detail := decodeAtomicBatchHTTPProblem(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), detail.Code)
}

func TestCreateAtomicTransactionBatchV2_RejectsExactDecodedBodyCeiling(t *testing.T) {
	app := buildHumaV2DirectApp(t, &TransactionHandler{TransactionBatchMaxSize: 50})
	body := []byte(oversizedV2CreateBody(v2CreateMaxBodyBytes))
	require.Len(t, body, int(v2CreateMaxBodyBytes))

	resp := postAtomicBatchV2(t, app, body, nil)
	defer func() { _ = resp.Body.Close() }()

	detail := decodeAtomicBatchHTTPProblem(t, resp)
	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	assert.Equal(t, constant.ErrPayloadTooLarge.Error(), detail.Code)
}

func TestCreateAtomicTransactionBatchV2_AggregatesOrderedStructuralErrors(t *testing.T) {
	app := buildHumaV2DirectApp(t, &TransactionHandler{TransactionBatchMaxSize: 50})

	first := validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID)
	first.Amount = "0"
	second := validAtomicBatchV2Request("@source-two", "@destination-two", batchTestLedgerID)
	second.Asset = ""
	body := marshalAtomicBatchV2Wrapper(
		t,
		revisedAtomicBatchV2Item(t, first, "direct", 1),
		revisedAtomicBatchV2Item(t, second, "hold", 2),
	)

	resp := postAtomicBatchV2(t, app, body, nil)
	defer func() { _ = resp.Body.Close() }()

	detail := decodeAtomicBatchHTTPProblem(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, constant.ErrTransactionBatchStructuralValidation.Error(), detail.Code)
	require.Len(t, detail.Errors, 2)
	assert.Equal(t, "body.transactions[0].amount", detail.Errors[0].Location)
	assert.Equal(t, "body.transactions[1].asset", detail.Errors[1].Location)
}

func TestCreateAtomicTransactionBatchV2_ReplayAndConflict(t *testing.T) {
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-000000000601")
	transactionIDs := []uuid.UUID{
		uuid.MustParse("01994f13-29b7-7000-8000-000000000602"),
		uuid.MustParse("01994f13-29b7-7000-8000-000000000603"),
	}
	response, err := json.Marshal(struct {
		BatchID      uuid.UUID                  `json:"batchId"`
		Transactions []*transaction.Transaction `json:"transactions"`
	}{
		BatchID: batchID,
		Transactions: []*transaction.Transaction{
			{
				ID:             transactionIDs[0].String(),
				OrganizationID: batchTestOrganizationID,
				LedgerID:       batchTestLedgerID,
				Description:    "first cached batch transaction",
			},
			{
				ID:             transactionIDs[1].String(),
				OrganizationID: batchTestOrganizationID,
				LedgerID:       batchTestLedgerID,
				Description:    "second cached batch transaction",
			},
		},
	})
	require.NoError(t, err)

	body := marshalAtomicBatchV2Wrapper(
		t,
		revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID), "direct", 1),
		revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@source-two", "@destination-two", batchTestLedgerID), "hold", 2),
	)
	// Whitespace makes the submitted bytes differ from their canonical identity,
	// proving the handler passes the canonical representation rather than raw bytes.
	body = append(append([]byte(" \n"), body...), '\n')
	decoded, err := decodeAndValidateRevisedAtomicTransactionBatchV2(body, 50)
	require.NoError(t, err)
	wantFingerprint := decoded.requestFingerprint

	t.Run("completed request replays its ordered response", func(t *testing.T) {
		repository := &atomicBatchHandlerClaimRepository{
			claim: func(
				_ context.Context,
				organizationID, ledgerID uuid.UUID,
				effectiveKey string,
				claim txRedis.AtomicTransactionBatchIdempotencyRecord,
			) (*txRedis.AtomicTransactionBatchClaimResult, error) {
				assert.Equal(t, batchTestOrganizationID, organizationID.String())
				assert.Equal(t, batchTestLedgerID, ledgerID.String())
				assert.Equal(t, "batch-replay-key", effectiveKey)
				assert.Equal(t, wantFingerprint, claim.RequestFingerprint)

				return &txRedis.AtomicTransactionBatchClaimResult{
					Outcome: txRedis.AtomicTransactionBatchReplayed,
					Record: txRedis.AtomicTransactionBatchIdempotencyRecord{
						FormatVersion:      txRedis.AtomicTransactionBatchIdempotencyFormatVersion,
						State:              txRedis.AtomicTransactionBatchStateComplete,
						RequestFingerprint: strings.Repeat("a", 64),
						OwnerToken:         uuid.NewString(),
						BatchID:            batchID,
						Response:           response,
					},
				}, nil
			},
		}
		handler := replayOnlyAtomicBatchHandler(repository)
		app := buildHumaV2DirectApp(t, handler)

		resp := postAtomicBatchV2(t, app, body, map[string]string{"X-Idempotency": "batch-replay-key"})
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.Equal(t, "true", resp.Header.Get("X-Idempotency-Replayed"))

		var got CreateAtomicTransactionBatchV2Response
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Equal(t, batchID.String(), got.BatchID)
		require.Len(t, got.Transactions, 2)
		assert.Equal(t, transactionIDs[0].String(), got.Transactions[0].ID)
		assert.Equal(t, transactionIDs[1].String(), got.Transactions[1].ID)
	})

	t.Run("in-progress or changed request keeps the canonical conflict", func(t *testing.T) {
		repository := &atomicBatchHandlerClaimRepository{
			claim: func(
				context.Context,
				uuid.UUID,
				uuid.UUID,
				string,
				txRedis.AtomicTransactionBatchIdempotencyRecord,
			) (*txRedis.AtomicTransactionBatchClaimResult, error) {
				return &txRedis.AtomicTransactionBatchClaimResult{
						Outcome: txRedis.AtomicTransactionBatchInProgress,
					}, pkg.ValidateBusinessError(
						constant.ErrIdempotencyKey,
						constant.EntityTransaction,
						"atomic transaction batch",
					)
			},
		}
		handler := replayOnlyAtomicBatchHandler(repository)
		app := buildHumaV2DirectApp(t, handler)

		resp := postAtomicBatchV2(t, app, body, map[string]string{"X-Idempotency": "batch-conflict-key"})
		defer func() { _ = resp.Body.Close() }()

		detail := decodeAtomicBatchHTTPProblem(t, resp)
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		assert.Equal(t, constant.ErrIdempotencyKey.Error(), detail.Code)
	})
}

type atomicBatchHandlerClaimRepository struct {
	command.AtomicTransactionBatchIdempotencyRepository
	claim func(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		string,
		txRedis.AtomicTransactionBatchIdempotencyRecord,
	) (*txRedis.AtomicTransactionBatchClaimResult, error)
}

func (repository *atomicBatchHandlerClaimRepository) ClaimAtomicTransactionBatch(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey string,
	claim txRedis.AtomicTransactionBatchIdempotencyRecord,
) (*txRedis.AtomicTransactionBatchClaimResult, error) {
	return repository.claim(ctx, organizationID, ledgerID, effectiveKey, claim)
}

func replayOnlyAtomicBatchHandler(repository command.AtomicTransactionBatchIdempotencyRepository) *TransactionHandler {
	return &TransactionHandler{
		Command: &command.UseCase{
			UUIDv7Generator: func() (uuid.UUID, error) {
				return uuid.MustParse("01994f13-29b7-7000-8000-000000000600"), nil
			},
			AtomicTransactionBatchIdempotencyRepo: repository,
		},
		TransactionBatchMaxSize: 50,
	}
}

func postAtomicBatchV2(
	t *testing.T,
	app *fiber.App,
	body []byte,
	headers map[string]string,
) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, atomicBatchV2RoutePath, strings.NewReader(string(body)))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

func decodeAtomicBatchHTTPProblem(t *testing.T, resp *http.Response) *pkgHTTP.Detail {
	t.Helper()

	var detail pkgHTTP.Detail
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&detail))

	return &detail
}
