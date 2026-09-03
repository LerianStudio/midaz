// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the transport layer of the MONEY-WRITE transaction resource. Every shell
// decodes/validates the request and delegates to the use case
// (command.CreateTransaction / commitOrCancelTransaction / UpdateTransaction command +
// query), then projects the result onto a typed Huma Out. The create orchestration
// (validate -> fee -> reserve -> ProcessBalanceOperations -> BuildOperations ->
// WriteTransaction, with its 9 cleanup points) lives in the command package; this file
// only reads path params + idempotency headers and writes the response — the same split
// account/holder/instrument use. Conventions (see asset_handler.go's header for the full
// rationale):
//
//  1. Path params are plain strings with only `doc:` (no format tag) so the sole UUID
//     validator stays the ParseUUIDPathParameters Fiber middleware attached BEFORE the
//     Huma terminal — never a native Huma 422. The shells re-parse via parsePathUUID
//     (mirrors GetUUIDFromLocals' 0065 envelope).
//  2. Body ops carry RawBody []byte + SkipValidateBody so http.DecodeAndValidate is the
//     sole body validator. The idempotency HASH is computed by the use case over the built
//     mtransaction.Transaction (StructToJSONString).
//  3. CREATE + commit/cancel/revert + idempotent replay all return 201 (matching
//     http.Created); PATCH/GET return 200. The X-Idempotency-Replayed response header is
//     driven off the `replayed` bool the create use case returns.
//  4. UpdateTransaction is NOT merge-patch: the command takes a plain decoded
//     *transaction.UpdateTransactionInput (no FindNilFields / RawBody null-field
//     derivation), so the shell decodes that type and delegates unchanged.
//  5. GET-by-id sets X-Cache-Hit off the flag the core returns.
//  6. Errors go through the shared pkgHTTP.HumaProblem (RFC 9457). Auth stays the Fiber
//     guard chain (auth.Authorize("midaz","transactions",verb) + tenant +
//     ParseUUIDPathParameters("transaction")) attached BEFORE the Huma terminal — the
//     per-op Security metadata is SPEC-ONLY.

// secTransactionBearer advertises a JWT bearer token per operation (Bearer-only,
// matching the Fiber guard chain on every transaction route). SPEC metadata only;
// runtime auth is the Fiber guard chain.
var secTransactionBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- shared transaction-create shell ------------------------------------------

// createTransactionShellV1 is the common body of the six /v1 Huma CREATE shells. It
// re-parses the org/ledger path strings (the ParseUUIDPathParameters middleware is
// the sole UUID validator), resolves the idempotency key/TTL from headers, delegates
// to command.CreateTransactionV1, and projects the built transaction + the replayed
// flag onto the typed Out. The create routes carry no :transaction_id segment, so the
// use case records no parent transaction id.
//
// The route version is the method name: the /v1 contract carries neither the fee
// engine nor the tracer reservation, and CreateTransactionV1 references neither.
func (handler *TransactionHandler) createTransactionShellV1(ctx context.Context, orgStr, ledgerStr string, transactionInput mtransaction.Transaction, transactionStatus, idempotencyKey, idempotencyTTL string) (*CreateTransactionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, err := parseOrgLedger(orgStr, ledgerStr)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, replayed, err := handler.Command.CreateTransactionV1(ctx, command.CreateTransactionV1Input{
		OrganizationID:    orgID,
		LedgerID:          ledgerID,
		Transaction:       transactionInput,
		TransactionStatus: transactionStatus,
		IdempotencyKey:    idempotencyKey,
		IdempotencyTTL:    pkgHTTP.ParseIdempotencyTTL(idempotencyTTL),
	})
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateTransactionResponse{
		Status:              http.StatusCreated,
		IdempotencyReplayed: replayedHeader(replayed),
		Body:                newTransactionV1(tran),
	}, nil
}

// replayedHeader maps the core's replayed bool to the X-Idempotency-Replayed value.
func replayedHeader(replayed bool) string {
	if replayed {
		return "true"
	}

	return "false"
}

// CreateTransactionResponse pins 201 (matching http.Created) and carries the
// X-Idempotency-Replayed response header.
type CreateTransactionResponse struct {
	Status              int
	IdempotencyReplayed string `header:"X-Idempotency-Replayed"`
	Body                *TransactionV1
}

// --- POST /transactions/json --------------------------------------------------

// CreateTransactionJSONRequest is the JSON-create request envelope. RawBody keeps the
// body out of Huma's validator; the idempotency headers are read so the shell runs the
// claim over the core-computed hash.
type CreateTransactionJSONRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionJSON decodes+validates the raw body imperatively via
// http.DecodeAndValidate over CreateTransactionInput, builds the transaction, and
// delegates to the shared createTransaction core.
func (handler *TransactionHandler) CreateTransactionJSON(ctx context.Context, in *CreateTransactionJSONRequest) (*CreateTransactionResponse, error) {
	payload := new(mtransaction.CreateTransactionInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput := payload.BuildTransaction()

	return handler.createTransactionShellV1(ctx, in.OrganizationID, in.LedgerID, *transactionInput, transactionInput.InitialStatus(), in.IdempotencyKey, in.IdempotencyTTL)
}

// --- POST /transactions/annotation --------------------------------------------

// CreateTransactionAnnotation mirrors CreateTransactionJSON but forces the
// NOTED status (annotation-only, no balance changes).
func (handler *TransactionHandler) CreateTransactionAnnotation(ctx context.Context, in *CreateTransactionJSONRequest) (*CreateTransactionResponse, error) {
	payload := new(mtransaction.CreateTransactionInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput := payload.BuildTransaction()

	return handler.createTransactionShellV1(ctx, in.OrganizationID, in.LedgerID, *transactionInput, constant.NOTED, in.IdempotencyKey, in.IdempotencyTTL)
}

// --- POST /transactions/inflow ------------------------------------------------

// CreateTransactionInflowRequest is the inflow-create request envelope (same
// idempotency + path shape as JSON; the body decodes into CreateTransactionInflowInput).
type CreateTransactionInflowRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionInflow decodes CreateTransactionInflowInput, builds the inflow
// entry, and delegates to the shared createTransaction core.
func (handler *TransactionHandler) CreateTransactionInflow(ctx context.Context, in *CreateTransactionInflowRequest) (*CreateTransactionResponse, error) {
	payload := new(mtransaction.CreateTransactionInflowInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput := payload.BuildInflowEntry()

	return handler.createTransactionShellV1(ctx, in.OrganizationID, in.LedgerID, *transactionInput, transactionInput.InitialStatus(), in.IdempotencyKey, in.IdempotencyTTL)
}

// --- POST /transactions/outflow -----------------------------------------------

// CreateTransactionOutflowRequest is the outflow-create request envelope.
type CreateTransactionOutflowRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionOutflow decodes CreateTransactionOutflowInput, builds the outflow
// entry, and delegates to the shared createTransaction core.
func (handler *TransactionHandler) CreateTransactionOutflow(ctx context.Context, in *CreateTransactionOutflowRequest) (*CreateTransactionResponse, error) {
	payload := new(mtransaction.CreateTransactionOutflowInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput := payload.BuildOutflowEntry()

	return handler.createTransactionShellV1(ctx, in.OrganizationID, in.LedgerID, *transactionInput, transactionInput.InitialStatus(), in.IdempotencyKey, in.IdempotencyTTL)
}

// --- POST /transactions/block -------------------------------------------------

// CreateTransactionBlockRequest is the block-create request envelope (same
// idempotency + path shape as JSON; the body decodes into CreateTransactionInput,
// identical to the JSON create body).
type CreateTransactionBlockRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionBlock decodes CreateTransactionInput, builds the transaction
// with the BLOCK operation-type override (Pending forced false), and delegates to
// the shared createTransaction core.
func (handler *TransactionHandler) CreateTransactionBlock(ctx context.Context, in *CreateTransactionBlockRequest) (*CreateTransactionResponse, error) {
	payload := new(mtransaction.CreateTransactionInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput := handler.buildOverriddenTransaction(payload, constant.BLOCK)

	return handler.createTransactionShellV1(ctx, in.OrganizationID, in.LedgerID, transactionInput, transactionInput.InitialStatus(), in.IdempotencyKey, in.IdempotencyTTL)
}

// --- POST /transactions/unblock -----------------------------------------------

// CreateTransactionUnblock decodes CreateTransactionInput, builds the transaction
// with the UNBLOCK operation-type override (Pending forced false), and delegates to
// the shared createTransaction core.
func (handler *TransactionHandler) CreateTransactionUnblock(ctx context.Context, in *CreateTransactionBlockRequest) (*CreateTransactionResponse, error) {
	payload := new(mtransaction.CreateTransactionInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput := handler.buildOverriddenTransaction(payload, constant.UNBLOCK)

	return handler.createTransactionShellV1(ctx, in.OrganizationID, in.LedgerID, transactionInput, transactionInput.InitialStatus(), in.IdempotencyKey, in.IdempotencyTTL)
}

// --- POST /transactions/{transaction_id}/commit|cancel|revert -----------------

// StateTransactionRequest is the id-only, bodiless request envelope shared by the
// commit/cancel/revert state ops. No body, no idempotency headers.
type StateTransactionRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionID  string `path:"transaction_id" doc:"Transaction ID (UUID)"`
}

// StateTransactionResponse pins 201 (matching http.Created) and carries the resulting
// transaction. Both commit and cancel return 201. Revert answers
// with CreateTransactionResponse instead: it creates a transaction and can replay.
type StateTransactionResponse struct {
	Status int
	Body   *TransactionV1
}

// CommitTransaction delegates to the commitTransaction core (fetch write-behind/DB, then
// commitOrCancelTransaction with APPROVED, which runs the tracer confirm-by-transaction
// two-phase). Returns 201.
func (handler *TransactionHandler) CommitTransaction(ctx context.Context, in *StateTransactionRequest) (*StateTransactionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, txID, err := parseOrgLedgerTx(in)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, err := handler.commitTransaction(ctx, orgID, ledgerID, txID, constant.APPROVED, command.RouteV1)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &StateTransactionResponse{Status: http.StatusCreated, Body: newTransactionV1(tran)}, nil
}

// CancelTransaction delegates to the commitTransaction core with CANCELED
// (which runs the tracer release-by-transaction two-phase). Returns 201.
func (handler *TransactionHandler) CancelTransaction(ctx context.Context, in *StateTransactionRequest) (*StateTransactionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, txID, err := parseOrgLedgerTx(in)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, err := handler.commitTransaction(ctx, orgID, ledgerID, txID, constant.CANCELED, command.RouteV1)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &StateTransactionResponse{Status: http.StatusCreated, Body: newTransactionV1(tran)}, nil
}

// RevertTransaction delegates to command.RevertTransactionV1 (parent/revert
// eligibility + bidirectional-route checks, then the /v1 create pipeline) and projects the
// use case's replayed flag onto the response header, mirroring createTransactionShellV1.
// Returns 201.
//
// It answers with CreateTransactionResponse because a revert IS a create: it runs the
// same create pipeline, answers 201 with a freshly created reverse, and can replay —
// so the create envelope already models the response, headers included. commit/cancel are
// the ones that differ (pure state transitions) and keep StateTransactionResponse.
func (handler *TransactionHandler) RevertTransaction(ctx context.Context, in *StateTransactionRequest) (*CreateTransactionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, txID, err := parseOrgLedgerTx(in)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, replayed, err := handler.Command.RevertTransactionV1(ctx, command.RevertTransactionInput{
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		TransactionID:  txID,
	})
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateTransactionResponse{
		Status:              http.StatusCreated,
		IdempotencyReplayed: replayedHeader(replayed),
		Body:                newTransactionV1(tran),
	}, nil
}

// parseOrgLedgerTx resolves the three path strings the state/patch/get-by-id shells
// carry. ParseUUIDPathParameters has already validated them on the wired path.
func parseOrgLedgerTx(in *StateTransactionRequest) (orgID, ledgerID, txID uuid.UUID, err error) {
	orgID, ledgerID, err = parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}

	txID, err = parsePathUUID(in.TransactionID, "transaction_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}

	return orgID, ledgerID, txID, nil
}

// --- PATCH /transactions/{transaction_id} -------------------------------------

// UpdateTransactionRequest is the update request envelope. RawBody keeps the body out
// of Huma's validator; PATCH is a plain decode (NOT merge-patch), so the shell passes the
// decoded input straight to the command.
type UpdateTransactionRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionID  string `path:"transaction_id" doc:"Transaction ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateTransactionResponse carries the updated transaction (200, matching http.OK).
type UpdateTransactionResponse struct {
	Status int
	Body   *TransactionV1
}

// UpdateTransaction decodes+validates the raw body imperatively then delegates to the
// shared updateTransaction core (command.UpdateTransaction + query.GetTransactionByID).
func (handler *TransactionHandler) UpdateTransaction(ctx context.Context, in *UpdateTransactionRequest) (*UpdateTransactionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, txID, err := parseOrgLedgerTx(&StateTransactionRequest{
		OrganizationID: in.OrganizationID, LedgerID: in.LedgerID, TransactionID: in.TransactionID,
	})
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(transaction.UpdateTransactionInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, err := handler.updateTransaction(ctx, orgID, ledgerID, txID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateTransactionResponse{Status: http.StatusOK, Body: newTransactionV1(tran)}, nil
}

// --- GET /transactions/{transaction_id} ---------------------------------------

// GetTransactionResponse carries the transaction verbatim (200) plus the X-Cache-Hit
// header ("true" on a write-behind cache hit, "false" otherwise).
type GetTransactionResponse struct {
	Status   int
	CacheHit string `header:"X-Cache-Hit"`
	Body     *TransactionV1
}

// GetTransaction binds the query imperatively via http.ValidateParameters then delegates
// to the shared getTransaction core, projecting the cache-hit flag onto the response
// header.
func (handler *TransactionHandler) GetTransaction(ctx context.Context, in *GetTransactionByIDRequest) (*GetTransactionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, txID, err := parseOrgLedgerTx(&StateTransactionRequest{
		OrganizationID: in.OrganizationID, LedgerID: in.LedgerID, TransactionID: in.TransactionID,
	})
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	headerParams, err := pkgHTTP.ValidateParameters(queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	headerParams.Metadata = &bson.M{}

	tran, cacheHit, err := handler.getTransaction(ctx, orgID, ledgerID, txID, headerParams)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	hit := "false"
	if cacheHit {
		hit = "true"
	}

	return &GetTransactionResponse{Status: http.StatusOK, CacheHit: hit, Body: newTransactionV1(tran)}, nil
}

// GetTransactionByIDRequest is the by-id request envelope. It captures the raw query
// via Resolve for the imperative http.ValidateParameters binder.
type GetTransactionByIDRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionID  string `path:"transaction_id" doc:"Transaction ID (UUID)"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (no validation; canonical rejection
// stays in http.ValidateParameters).
func (in *GetTransactionByIDRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// --- GET /transactions (list) -------------------------------------------------

// ListTransactionsRequest advertises the list query params (doc-only, no validation
// tags) and captures the raw query via Resolve for the imperative binder.
type ListTransactionsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Metadata       string `query:"metadata" doc:"JSON string to filter transactions by metadata fields"`
	Limit          string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	StartDate      string `query:"start_date" doc:"Filter transactions created on/after this date"`
	EndDate        string `query:"end_date" doc:"Filter transactions created on/before this date"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	Cursor         string `query:"cursor" doc:"Pagination cursor"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (no validation; canonical rejection
// stays in http.ValidateParameters).
func (in *ListTransactionsRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// ListTransactionsResponse carries the pagination envelope verbatim.
type ListTransactionsResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllTransactions binds the query imperatively then delegates to the shared
// getAllTransactions core.
func (handler *TransactionHandler) GetAllTransactions(ctx context.Context, in *ListTransactionsRequest) (*ListTransactionsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllTransactions(ctx, orgID, ledgerID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListTransactionsResponse{Status: http.StatusOK, Body: newTransactionV1Items(pagination)}, nil
}

// --- registration -------------------------------------------------------------
