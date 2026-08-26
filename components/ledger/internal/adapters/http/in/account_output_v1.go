// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// AccountV1 is the /v1 wire projection of an account: the canonical
// mmodel.Account with holderId and holderCheckSkipped withheld. Both keys belong
// to the holder seam, which is /v2 ONLY, so a v1 client that already parses this
// body never receives a field it was not written against.
//
// The withholding is a SHADOW, not a removal: the embedded struct still carries
// both values, which is what the /v2 responses, the streaming event payloads and
// the persisted row read. Only this response shape hides them.
//
// Two mechanisms have to agree, one per surface:
//
// BODY — encoding/json's field-conflict rule. A field at depth 0 wins over an
// embedded field of the same JSON name at depth 1, so declaring the two names here
// stops the embedded ones being promoted. The type is `any` left nil and tagged
// omitempty, which is what makes the winning field emit NOTHING. A `json:"-"`
// shadow does NOT work here: that tag drops the field from consideration entirely
// and the embedded field is promoted again.
//
// SCHEMA — Huma's `hidden:"true"`. Huma walks outer fields before embedded ones and
// dedups on the GO field name, so these two suppress the embedded properties; the
// tag then keeps the shadows themselves out of the marshalled document. Without it
// the contract would advertise two keys the body never sends.
//
// Every other field rides through the embed, so a new field on mmodel.Account
// reaches /v1 with no change here.
type AccountV1 struct {
	*mmodel.Account

	// HolderID shadows the embedded holderId so the key is absent from /v1.
	// Always nil; never set it.
	HolderID any `json:"holderId,omitempty" hidden:"true"`

	// HolderCheckSkipped shadows the embedded holderCheckSkipped so the key is
	// absent from /v1. Always nil; never set it.
	HolderCheckSkipped any `json:"holderCheckSkipped,omitempty" hidden:"true"`
}

// newAccountV1 wraps an account in the /v1 response shape. A nil account stays nil
// so a bodiless answer is unaffected.
func newAccountV1(a *mmodel.Account) *AccountV1 {
	if a == nil {
		return nil
	}

	return &AccountV1{Account: a}
}

// newAccountV1Items re-projects a page of accounts onto the /v1 shape. The list
// core is shared with the /v2 read, which needs the two fields, so the projection
// happens HERE in the v1 transport rather than in getAllAccounts.
//
// A page whose items are not []*mmodel.Account is returned untouched: the core
// sets that concrete type, and a mismatch means there is nothing to project.
func newAccountV1Items(p pkgHTTP.Pagination) pkgHTTP.Pagination {
	src, ok := p.Items.([]*mmodel.Account)
	if !ok || src == nil {
		return p
	}

	out := make([]*AccountV1, 0, len(src))
	for _, a := range src {
		out = append(out, newAccountV1(a))
	}

	p.Items = out

	return p
}
