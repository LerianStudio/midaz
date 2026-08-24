// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file carries the /v2 read and update transaction terminals: the get-by-id, list, and PATCH
// update shells the /v2 group mounts. Each calls the SAME query/command core its v1 twin calls
// (getTransaction, getAllTransactions, updateTransaction) and differs only in the response
// envelope — the /v2 wire shape (TransactionV2, projected by newTransactionV2) instead of the
// canonical transaction.Transaction. The v1 handler methods and their v1 output types are left
// untouched, so /v1 responses stay byte-identical.

// UpdateTransactionOutputV2 carries the updated transaction in the /v2 wire shape (200,
// matching http.OK), mirroring UpdateTransactionResponse with a TransactionV2 body.
type UpdateTransactionOutputV2 struct {
	Status int
	Body   *TransactionV2
}

// UpdateTransactionV2 decodes+validates the raw body imperatively then delegates to the shared
// updateTransaction core, projecting the domain result onto the /v2 wire shape. It is the v2 twin
// of UpdateTransaction, reusing the same request type (transaction.UpdateTransactionInput).
func (handler *TransactionHandler) UpdateTransactionV2(ctx context.Context, in *UpdateTransactionRequest) (*UpdateTransactionOutputV2, error) {
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

	return &UpdateTransactionOutputV2{Status: http.StatusOK, Body: newTransactionV2(tran)}, nil
}

// GetTransactionOutputV2 carries the transaction in the /v2 wire shape (200) plus the
// X-Cache-Hit header the shared read core sets, mirroring GetTransactionResponse with a
// TransactionV2 body.
type GetTransactionOutputV2 struct {
	Status   int
	CacheHit string `header:"X-Cache-Hit"`
	Body     *TransactionV2
}

// GetTransactionV2 binds the query imperatively then delegates to the shared getTransaction
// core, projecting the domain result onto the /v2 wire shape and the cache-hit flag onto the
// response header. It is the v2 twin of GetTransaction.
func (handler *TransactionHandler) GetTransactionV2(ctx context.Context, in *GetTransactionByIDRequest) (*GetTransactionOutputV2, error) {
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

	return &GetTransactionOutputV2{Status: http.StatusOK, CacheHit: hit, Body: newTransactionV2(tran)}, nil
}

// TransactionV2ListBody is the /v2 cursor-paginated list envelope. Its wire shape matches the v1
// CursorPagination (pkgHTTP.Pagination) json tags exactly — same limit/page/next_cursor/prev_cursor
// fields — but its items are typed as TransactionV2 so the v2 wire contract carries the renamed and
// deprecated-field-dropped shape.
type TransactionV2ListBody struct {
	// The transactions on this page, in the /v2 wire shape
	Items []*TransactionV2 `json:"items"`

	// Maximum number of items per page
	// example: 10
	Limit int `json:"limit" example:"10"`

	// Current page number (offset mode)
	// example: 1
	Page int `json:"page,omitempty" example:"1"`

	// Cursor to the next page
	// example: eyJpZCI6IjAxOTI...
	NextCursor string `json:"next_cursor,omitempty" example:"eyJpZCI6IjAxOTI..."`

	// Cursor to the previous page
	// example: eyJpZCI6IjAxOTE...
	PrevCursor string `json:"prev_cursor,omitempty" example:"eyJpZCI6IjAxOTE..."`
}

// ListTransactionsOutputV2 carries the /v2 pagination envelope, mirroring
// ListTransactionsResponse with TransactionV2 items.
type ListTransactionsOutputV2 struct {
	Status int
	Body   TransactionV2ListBody
}

// GetAllTransactionsV2 binds the query imperatively then delegates to the shared
// getAllTransactions core, projecting the returned page onto the /v2 list envelope. It is the v2
// twin of GetAllTransactions.
func (handler *TransactionHandler) GetAllTransactionsV2(ctx context.Context, in *ListTransactionsRequest) (*ListTransactionsOutputV2, error) {
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

	return &ListTransactionsOutputV2{Status: http.StatusOK, Body: newTransactionV2ListBody(pagination)}, nil
}

// newTransactionV2ListBody projects the shared getAllTransactions page onto the /v2 list envelope,
// mapping each canonical transaction to its /v2 wire shape and copying the pagination fields
// verbatim. A nil item slice is preserved as nil so the encoding matches the v1 envelope's
// null-vs-empty behavior for a page with no items.
func newTransactionV2ListBody(p pkgHTTP.Pagination) TransactionV2ListBody {
	body := TransactionV2ListBody{
		Limit:      p.Limit,
		Page:       p.Page,
		NextCursor: p.NextCursor,
		PrevCursor: p.PrevCursor,
	}

	if src, ok := p.Items.([]*transaction.Transaction); ok && src != nil {
		items := make([]*TransactionV2, 0, len(src))
		for _, t := range src {
			items = append(items, newTransactionV2(t))
		}

		body.Items = items
	}

	return body
}
