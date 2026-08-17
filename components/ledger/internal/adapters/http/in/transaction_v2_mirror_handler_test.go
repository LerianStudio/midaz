// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TestNewTransactionV2ListBody maps the shared getAllTransactions page onto the /v2 list envelope:
// each canonical transaction becomes a TransactionV2 (dropping the deprecated fields), the
// pagination fields copy verbatim, and a nil item slice stays nil so the wire encoding matches the
// v1 envelope's null-for-empty behavior.
func TestNewTransactionV2ListBody(t *testing.T) {
	t.Parallel()

	t.Run("maps items and copies pagination fields", func(t *testing.T) {
		t.Parallel()

		src := []*transaction.Transaction{
			buildCanonicalTransactionFixture(),
			buildCanonicalTransactionFixture(),
		}

		body := newTransactionV2ListBody(pkgHTTP.Pagination{
			Items:      src,
			Limit:      10,
			Page:       2,
			NextCursor: "next-token",
			PrevCursor: "prev-token",
		})

		require.Len(t, body.Items, 2, "every canonical transaction must map to a TransactionV2")
		assert.Equal(t, src[0].Source, body.Items[0].Debit, "the projection must rename Source->Debit")
		assert.Equal(t, src[0].Destination, body.Items[0].Credit, "the projection must rename Destination->Credit")
		assert.Equal(t, 10, body.Limit)
		assert.Equal(t, 2, body.Page)
		assert.Equal(t, "next-token", body.NextCursor)
		assert.Equal(t, "prev-token", body.PrevCursor)
	})

	t.Run("nil item slice is preserved as nil", func(t *testing.T) {
		t.Parallel()

		var src []*transaction.Transaction

		body := newTransactionV2ListBody(pkgHTTP.Pagination{Items: src, Limit: 10})

		assert.Nil(t, body.Items, "a nil item slice must stay nil to match the v1 envelope encoding")
		assert.Equal(t, 10, body.Limit)
	})

	t.Run("non-slice Items yields no items", func(t *testing.T) {
		t.Parallel()

		body := newTransactionV2ListBody(pkgHTTP.Pagination{Items: "unexpected", Limit: 5})

		assert.Nil(t, body.Items, "an unexpected Items type must not panic and must produce no items")
		assert.Equal(t, 5, body.Limit)
	})
}

// TestNewOperationV2_NilInput proves the operation converter answers nil for a nil input rather
// than dereferencing it, matching the nil-guard convention newOperationsV2 relies on.
func TestNewOperationV2_NilInput(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newOperationV2(nil))
}

// TestNewOperationsV2_NilInput proves a nil operation slice maps to nil, so an operation-less
// transaction keeps the canonical operations encoding on the v2 wire.
func TestNewOperationsV2_NilInput(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newOperationsV2(nil))
}

// TestTransactionV2MirrorHandlers_RejectInvalidPathIDs proves the three /v2 reads/update handlers
// reject a malformed organization/ledger/transaction id at the transport guard, returning an error
// and no output before touching the shared query/command core. This locks the guard branch each v2
// twin runs ahead of the (integration-covered) happy path.
func TestTransactionV2MirrorHandlers_RejectInvalidPathIDs(t *testing.T) {
	t.Parallel()

	handler := &TransactionHandler{}
	ctx := context.Background()

	const badUUID = "not-a-uuid"
	const goodUUID = "00000000-0000-0000-0000-000000000000"

	t.Run("get by id", func(t *testing.T) {
		t.Parallel()

		out, err := handler.GetTransactionV2Huma(ctx, &GetTransactionByIDInputHuma{
			OrganizationID: badUUID, LedgerID: goodUUID, TransactionID: goodUUID,
		})
		require.Error(t, err)
		assert.Nil(t, out)
	})

	t.Run("update", func(t *testing.T) {
		t.Parallel()

		out, err := handler.UpdateTransactionV2Huma(ctx, &UpdateTransactionInputHuma{
			OrganizationID: goodUUID, LedgerID: badUUID, TransactionID: goodUUID,
		})
		require.Error(t, err)
		assert.Nil(t, out)
	})

	t.Run("list", func(t *testing.T) {
		t.Parallel()

		out, err := handler.GetAllTransactionsV2Huma(ctx, &ListTransactionsInputHuma{
			OrganizationID: goodUUID, LedgerID: badUUID,
		})
		require.Error(t, err)
		assert.Nil(t, out)
	})
}
