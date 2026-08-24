// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TransactionV1 is the /v1 wire projection of a transaction: the canonical
// transaction.Transaction with feesSkipped and tracerSkipped withheld. Those two
// keys are /v2 ONLY, so a v1 client that already parses this body never receives a
// field it was not written against.
//
// The withholding is a SHADOW, not a removal: the embedded struct still carries both
// values, which is what the /v2 projection (newTransactionV2), the streaming event
// payloads, and the idempotency replay cache read. Only this response shape hides
// them.
//
// Two mechanisms have to agree, one per surface:
//
// BODY — encoding/json's field-conflict rule. A field at depth 0 wins over an embedded
// field of the same JSON name at depth 1, so declaring the two names here stops the
// embedded ones being promoted. The type is `any` left nil and tagged omitempty, which
// is what makes the winning field emit NOTHING. A `json:"-"` shadow does NOT work here:
// that tag drops the field from consideration entirely and the embedded field is
// promoted again.
//
// SCHEMA — Huma's `hidden:"true"`. Huma walks outer fields before embedded ones and
// dedups on the GO field name, so these two suppress the embedded properties; the tag
// then keeps the shadows themselves out of the marshalled document. Without it the
// contract would advertise two keys the body never sends.
//
// Every other field rides through the embed, so a new field on transaction.Transaction
// reaches /v1 with no change here.
type TransactionV1 struct {
	*transaction.Transaction

	// FeesSkipped shadows the embedded feesSkipped so the key is absent from /v1.
	// Always nil; never set it.
	FeesSkipped any `json:"feesSkipped,omitempty" hidden:"true"`

	// TracerSkipped shadows the embedded tracerSkipped so the key is absent from /v1.
	// Always nil; never set it.
	TracerSkipped any `json:"tracerSkipped,omitempty" hidden:"true"`
}

// newTransactionV1 wraps a transaction in the /v1 response shape. A nil transaction
// stays nil so a bodiless answer is unaffected.
func newTransactionV1(t *transaction.Transaction) *TransactionV1 {
	if t == nil {
		return nil
	}

	return &TransactionV1{Transaction: t}
}

// newTransactionV1Items re-projects a page of transactions onto the /v1 shape. The
// list core is shared with the /v2 mirror read, which needs the two fields, so the
// projection happens HERE in the v1 transport rather than in getAllTransactions.
//
// A page whose items are not []*transaction.Transaction is returned untouched: the
// core sets that concrete type, and a mismatch means there is nothing to project.
func newTransactionV1Items(p pkgHTTP.Pagination) pkgHTTP.Pagination {
	src, ok := p.Items.([]*transaction.Transaction)
	if !ok || src == nil {
		return p
	}

	out := make([]*TransactionV1, 0, len(src))
	for _, t := range src {
		out = append(out, newTransactionV1(t))
	}

	p.Items = out

	return p
}
