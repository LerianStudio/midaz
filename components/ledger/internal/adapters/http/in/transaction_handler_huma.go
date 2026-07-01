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
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the MONEY-WRITE transaction resource
// (Wave 4 — the money path). It is transport-only: every shell decodes/validates the
// request exactly as the Fiber wrapper does and delegates to the SAME untouched core
// (createTransaction / commitOrCancelTransaction / UpdateTransaction command + query),
// then projects the result onto a typed Huma Out. The ~480-line createTransaction
// orchestration (validate → fee → reserve → ProcessBalanceOperations → BuildOperations
// → WriteTransaction, with its 9 cleanup points) is NOT touched: the only extraction is
// the thin transport boundary that reads path params + idempotency headers and writes
// the response — the exact split account/holder/instrument already use. Conventions
// (see asset_handler_huma.go's header for the full rationale):
//
//  1. Path params are plain strings with only `doc:` (no format tag) so the sole UUID
//     validator stays the ParseUUIDPathParameters Fiber middleware attached BEFORE the
//     Huma terminal — never a native Huma 422. The shells re-parse via parsePathUUID
//     (mirrors GetUUIDFromLocals' 0065 envelope).
//  2. Body ops carry RawBody []byte + SkipValidateBody so http.DecodeAndValidate (the
//     SAME pipeline the Fiber WithBody decorator runs, over the SAME input type) is the
//     sole body validator. The idempotency HASH is computed by the untouched core over
//     the SAME built mtransaction.Transaction (StructToJSONString) — byte-identical to
//     the Fiber path.
//  3. CREATE + commit/cancel/revert + idempotent replay all return 201 (matching
//     http.Created); PATCH/GET return 200. The X-Idempotency-Replayed response header is
//     driven off the `replayed` bool the transport-neutral createTransaction core returns
//     (the transport-neutral mirror of the Fiber wrapper's c.Set).
//  4. UpdateTransaction is NOT merge-patch: the Fiber wrapper feeds a plain
//     BodyParser-decoded *transaction.UpdateTransactionInput into the command (no
//     FindNilFields / RawBody null-field derivation), so the shell decodes the same type
//     and delegates unchanged.
//  5. GET-by-id sets X-Cache-Hit exactly as the Fiber path; the core returns the flag so
//     the shell projects it onto the Out header.
//  6. Errors go through the shared pkgHTTP.HumaProblem (RFC 9457, field/status/code-
//     identical to the Fiber http.WithError path). Auth stays the Fiber guard chain
//     (auth.Authorize("midaz","transactions",verb) + tenant + ParseUUIDPathParameters
//     ("transaction")) attached BEFORE the Huma terminal — the per-op Security metadata
//     is SPEC-ONLY.
//
// POST /transactions/dsl is DELIBERATELY NOT migrated: it is a multipart .casl upload,
// SUNSET 2026-08-01, and stays a pure Fiber route (out of the Huma spec).

// secTransactionBearer advertises a JWT bearer token per operation (Bearer-only,
// matching the Fiber swaggo @Security BearerAuth on every transaction wrapper). SPEC
// metadata only; runtime auth is the Fiber guard chain.
var secTransactionBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- shared transaction-create shell ------------------------------------------

// createTransactionShell is the common body of the four Huma CREATE shells. It
// re-parses the org/ledger path strings (the ParseUUIDPathParameters middleware is
// the sole UUID validator), resolves the idempotency key/TTL from headers, delegates
// to the SAME transport-neutral createTransaction core the Fiber wrapper calls, and
// projects the built transaction + the replayed flag onto the typed Out. The parent
// transaction id is uuid.Nil on the create routes (no :transaction_id segment).
func (handler *TransactionHandler) createTransactionShell(ctx context.Context, orgStr, ledgerStr string, transactionInput mtransaction.Transaction, transactionStatus, idempotencyKey, idempotencyTTL string) (*CreateTransactionOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(orgStr, ledgerStr)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	params := &transactionPathParams{OrganizationID: orgID, LedgerID: ledgerID, TransactionID: uuid.Nil}
	ttl := pkgHTTP.ParseIdempotencyTTL(idempotencyTTL)

	tran, replayed, err := handler.createTransaction(ctx, params, transactionInput, transactionStatus, idempotencyKey, ttl)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateTransactionOutputHuma{
		Status:              http.StatusCreated,
		IdempotencyReplayed: replayedHeader(replayed),
		Body:                tran,
	}, nil
}

// replayedHeader maps the core's replayed bool to the header string the Fiber path sets.
func replayedHeader(replayed bool) string {
	if replayed {
		return "true"
	}

	return "false"
}

// CreateTransactionOutputHuma pins 201 (matching http.Created) and carries the
// X-Idempotency-Replayed response header (parity with the Fiber c.Set).
type CreateTransactionOutputHuma struct {
	Status              int
	IdempotencyReplayed string `header:"X-Idempotency-Replayed"`
	Body                *transaction.Transaction
}

// --- POST /transactions/json --------------------------------------------------

// CreateTransactionJSONInputHuma is the JSON-create request envelope. RawBody keeps the
// body out of Huma's validator; the idempotency headers are read so the shell runs the
// same claim the Fiber wrapper does (over the core-computed hash).
type CreateTransactionJSONInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionJSONHuma decodes+validates the raw body imperatively (the SAME
// http.DecodeAndValidate the Fiber WithBody decorator runs over CreateTransactionInput),
// builds the transaction, and delegates to the shared createTransaction core.
func (handler *TransactionHandler) CreateTransactionJSONHuma(ctx context.Context, in *CreateTransactionJSONInputHuma) (*CreateTransactionOutputHuma, error) {
	payload := new(mtransaction.CreateTransactionInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput := payload.BuildTransaction()

	return handler.createTransactionShell(ctx, in.OrganizationID, in.LedgerID, *transactionInput, transactionInput.InitialStatus(), in.IdempotencyKey, in.IdempotencyTTL)
}

// --- POST /transactions/annotation --------------------------------------------

// CreateTransactionAnnotationHuma mirrors CreateTransactionJSONHuma but forces the
// NOTED status (annotation-only, no balance changes), matching the Fiber wrapper.
func (handler *TransactionHandler) CreateTransactionAnnotationHuma(ctx context.Context, in *CreateTransactionJSONInputHuma) (*CreateTransactionOutputHuma, error) {
	payload := new(mtransaction.CreateTransactionInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput := payload.BuildTransaction()

	return handler.createTransactionShell(ctx, in.OrganizationID, in.LedgerID, *transactionInput, constant.NOTED, in.IdempotencyKey, in.IdempotencyTTL)
}

// --- POST /transactions/inflow ------------------------------------------------

// CreateTransactionInflowInputHuma is the inflow-create request envelope (same
// idempotency + path shape as JSON; the body decodes into CreateTransactionInflowInput).
type CreateTransactionInflowInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionInflowHuma decodes CreateTransactionInflowInput, builds the inflow
// entry, and delegates to the shared createTransaction core.
func (handler *TransactionHandler) CreateTransactionInflowHuma(ctx context.Context, in *CreateTransactionInflowInputHuma) (*CreateTransactionOutputHuma, error) {
	payload := new(mtransaction.CreateTransactionInflowInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput := payload.BuildInflowEntry()

	return handler.createTransactionShell(ctx, in.OrganizationID, in.LedgerID, *transactionInput, transactionInput.InitialStatus(), in.IdempotencyKey, in.IdempotencyTTL)
}

// --- POST /transactions/outflow -----------------------------------------------

// CreateTransactionOutflowInputHuma is the outflow-create request envelope.
type CreateTransactionOutflowInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionOutflowHuma decodes CreateTransactionOutflowInput, builds the outflow
// entry, and delegates to the shared createTransaction core.
func (handler *TransactionHandler) CreateTransactionOutflowHuma(ctx context.Context, in *CreateTransactionOutflowInputHuma) (*CreateTransactionOutputHuma, error) {
	payload := new(mtransaction.CreateTransactionOutflowInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput := payload.BuildOutflowEntry()

	return handler.createTransactionShell(ctx, in.OrganizationID, in.LedgerID, *transactionInput, transactionInput.InitialStatus(), in.IdempotencyKey, in.IdempotencyTTL)
}

// --- POST /transactions/{transaction_id}/commit|cancel|revert -----------------

// StateTransactionInputHuma is the id-only, bodiless request envelope shared by the
// commit/cancel/revert state ops. No body, no idempotency headers (the Fiber wrappers
// read none).
type StateTransactionInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionID  string `path:"transaction_id" doc:"Transaction ID (UUID)"`
}

// StateTransactionOutputHuma pins 201 (matching http.Created) and carries the resulting
// transaction. commit/cancel/revert all return 201, matching the Fiber path.
type StateTransactionOutputHuma struct {
	Status int
	Body   *transaction.Transaction
}

// CommitTransactionHuma delegates to the SAME commitTransaction core the Fiber wrapper
// calls (fetch write-behind/DB, then commitOrCancelTransaction with APPROVED, which runs
// the tracer confirm-by-transaction two-phase). Returns 201.
func (handler *TransactionHandler) CommitTransactionHuma(ctx context.Context, in *StateTransactionInputHuma) (*StateTransactionOutputHuma, error) {
	orgID, ledgerID, txID, err := parseOrgLedgerTx(in)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, err := handler.commitTransaction(ctx, orgID, ledgerID, txID, constant.APPROVED)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &StateTransactionOutputHuma{Status: http.StatusCreated, Body: tran}, nil
}

// CancelTransactionHuma delegates to the SAME commitTransaction core with CANCELED
// (which runs the tracer release-by-transaction two-phase). Returns 201.
func (handler *TransactionHandler) CancelTransactionHuma(ctx context.Context, in *StateTransactionInputHuma) (*StateTransactionOutputHuma, error) {
	orgID, ledgerID, txID, err := parseOrgLedgerTx(in)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, err := handler.commitTransaction(ctx, orgID, ledgerID, txID, constant.CANCELED)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &StateTransactionOutputHuma{Status: http.StatusCreated, Body: tran}, nil
}

// RevertTransactionHuma delegates to the SAME revertTransaction core (parent/revert
// eligibility + bidirectional-route checks, then createRevertTransaction). Returns 201.
func (handler *TransactionHandler) RevertTransactionHuma(ctx context.Context, in *StateTransactionInputHuma) (*StateTransactionOutputHuma, error) {
	orgID, ledgerID, txID, err := parseOrgLedgerTx(in)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, err := handler.revertTransaction(ctx, orgID, ledgerID, txID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &StateTransactionOutputHuma{Status: http.StatusCreated, Body: tran}, nil
}

// parseOrgLedgerTx resolves the three path strings the state/patch/get-by-id shells
// carry. ParseUUIDPathParameters has already validated them on the wired path.
func parseOrgLedgerTx(in *StateTransactionInputHuma) (orgID, ledgerID, txID uuid.UUID, err error) {
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

// UpdateTransactionInputHuma is the update request envelope. RawBody keeps the body out
// of Huma's validator; the Fiber PATCH wrapper is a plain BodyParser decode (NOT merge-
// patch), so the shell decodes the same type and passes it straight to the command.
type UpdateTransactionInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionID  string `path:"transaction_id" doc:"Transaction ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateTransactionOutputHuma carries the updated transaction (200, matching http.OK).
type UpdateTransactionOutputHuma struct {
	Status int
	Body   *transaction.Transaction
}

// UpdateTransactionHuma decodes+validates the raw body imperatively then delegates to the
// shared updateTransaction core (command.UpdateTransaction + query.GetTransactionByID).
func (handler *TransactionHandler) UpdateTransactionHuma(ctx context.Context, in *UpdateTransactionInputHuma) (*UpdateTransactionOutputHuma, error) {
	orgID, ledgerID, txID, err := parseOrgLedgerTx(&StateTransactionInputHuma{
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

	return &UpdateTransactionOutputHuma{Status: http.StatusOK, Body: tran}, nil
}

// --- GET /transactions/{transaction_id} ---------------------------------------

// GetTransactionOutputHuma carries the transaction verbatim (200) plus the X-Cache-Hit
// header the Fiber path sets ("true" on a write-behind cache hit, "false" otherwise).
type GetTransactionOutputHuma struct {
	Status   int
	CacheHit string `header:"X-Cache-Hit"`
	Body     *transaction.Transaction
}

// GetTransactionHuma binds the query imperatively (the SAME http.ValidateParameters the
// Fiber wrapper runs) then delegates to the shared getTransaction core, projecting the
// cache-hit flag onto the response header.
func (handler *TransactionHandler) GetTransactionHuma(ctx context.Context, in *GetTransactionByIDInputHuma) (*GetTransactionOutputHuma, error) {
	orgID, ledgerID, txID, err := parseOrgLedgerTx(&StateTransactionInputHuma{
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

	return &GetTransactionOutputHuma{Status: http.StatusOK, CacheHit: hit, Body: tran}, nil
}

// GetTransactionByIDInputHuma is the by-id request envelope. It captures the raw query
// via Resolve for the imperative http.ValidateParameters binder (the Fiber wrapper runs
// ValidateParameters over c.Queries()).
type GetTransactionByIDInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionID  string `path:"transaction_id" doc:"Transaction ID (UUID)"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (no validation; canonical rejection
// stays in http.ValidateParameters).
func (in *GetTransactionByIDInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// --- GET /transactions (list) -------------------------------------------------

// ListTransactionsInputHuma advertises the list query params (doc-only, no validation
// tags) and captures the raw query via Resolve for the imperative binder.
type ListTransactionsInputHuma struct {
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
func (in *ListTransactionsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// ListTransactionsOutputHuma carries the pagination envelope verbatim.
type ListTransactionsOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllTransactionsHuma binds the query imperatively then delegates to the shared
// getAllTransactions core.
func (handler *TransactionHandler) GetAllTransactionsHuma(ctx context.Context, in *ListTransactionsInputHuma) (*ListTransactionsOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllTransactions(ctx, orgID, ledgerID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListTransactionsOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- registration -------------------------------------------------------------

// RegisterTransactionRoutes registers the ten migrated transaction operations on the
// shared Huma API. It is the per-file seam the unified server calls; the auth
// (auth.Authorize("midaz","transactions",verb)) + tenant + ParseUUIDPathParameters
// ("transaction") chain for these routes is attached in the unified server (Fiber level)
// BEFORE the Huma terminal, not here. POST /transactions/dsl is NOT registered (SUNSET
// 2026-08-01, stays pure Fiber). Paths are GROUP-RELATIVE (the /v1 prefix rides the
// OpenAPI servers entry).
func RegisterTransactionRoutes(api huma.API, h *TransactionHandler) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions"
		idPath   = listPath + "/{transaction_id}"
		tag      = "Transactions"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionJSON",
		Method:           http.MethodPost,
		Path:             listPath + "/json",
		Summary:          "Create a Transaction using JSON",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate) — see file header.
	}, h.CreateTransactionJSONHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionInflow",
		Method:           http.MethodPost,
		Path:             listPath + "/inflow",
		Summary:          "Create a Transaction without passing from source",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
	}, h.CreateTransactionInflowHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionOutflow",
		Method:           http.MethodPost,
		Path:             listPath + "/outflow",
		Summary:          "Create a Transaction without passing to distribution",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
	}, h.CreateTransactionOutflowHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionAnnotation",
		Method:           http.MethodPost,
		Path:             listPath + "/annotation",
		Summary:          "Create a Transaction Annotation using JSON",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
	}, h.CreateTransactionAnnotationHuma)

	huma.Register(api, huma.Operation{
		OperationID: "commitTransaction",
		Method:      http.MethodPost,
		Path:        idPath + "/commit",
		Summary:     "Commit a Transaction",
		Tags:        []string{tag},
		Security:    secTransactionBearer,
		// commit returns 201 (matching http.Created); no request body.
		DefaultStatus: http.StatusCreated,
	}, h.CommitTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "cancelTransaction",
		Method:        http.MethodPost,
		Path:          idPath + "/cancel",
		Summary:       "Cancel a pre transaction",
		Tags:          []string{tag},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated,
	}, h.CancelTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "revertTransaction",
		Method:        http.MethodPost,
		Path:          idPath + "/revert",
		Summary:       "Revert a Transaction",
		Tags:          []string{tag},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated,
	}, h.RevertTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateTransaction",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a Transaction",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body validated imperatively — plain decode, not merge-patch.
	}, h.UpdateTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getTransaction",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get a Transaction by ID",
		Tags:        []string{tag},
		Security:    secTransactionBearer,
	}, h.GetTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAllTransactions",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Transactions",
		Tags:        []string{tag},
		Security:    secTransactionBearer,
	}, h.GetAllTransactionsHuma)
}
