// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func decodePatchMetadata[T any](metadata func(*T) map[string]any) func(string) (map[string]any, error) {
	return func(body string) (map[string]any, error) {
		in := new(T)
		_, err := pkgHTTP.DecodeAndValidate([]byte(body), in)

		return metadata(in), err
	}
}

// TestPatchBodyMetadataDecode pins what every metadata-bearing PATCH body hands its use case. The
// /v1 and /v2 routes of an entity decode the same input type, so each row covers both contracts.
func TestPatchBodyMetadataDecode(t *testing.T) {
	t.Parallel()

	entities := []struct {
		name   string
		decode func(string) (map[string]any, error)
	}{
		{"organization", decodePatchMetadata(func(in *mmodel.UpdateOrganizationInput) map[string]any { return in.Metadata })},
		{"ledger", decodePatchMetadata(func(in *mmodel.UpdateLedgerInput) map[string]any { return in.Metadata })},
		{"portfolio", decodePatchMetadata(func(in *mmodel.UpdatePortfolioInput) map[string]any { return in.Metadata })},
		{"segment", decodePatchMetadata(func(in *mmodel.UpdateSegmentInput) map[string]any { return in.Metadata })},
		{"account", decodePatchMetadata(func(in *mmodel.UpdateAccountInput) map[string]any { return in.Metadata })},
		{"account type", decodePatchMetadata(func(in *mmodel.UpdateAccountTypeInput) map[string]any { return in.Metadata })},
		{"asset", decodePatchMetadata(func(in *mmodel.UpdateAssetInput) map[string]any { return in.Metadata })},
		{"transaction", decodePatchMetadata(func(in *transaction.UpdateTransactionInput) map[string]any { return in.Metadata })},
		{"operation", decodePatchMetadata(func(in *operation.UpdateOperationInput) map[string]any { return in.Metadata })},
		{"operation route", decodePatchMetadata(func(in *mmodel.UpdateOperationRouteInput) map[string]any { return in.Metadata })},
		{"transaction route", decodePatchMetadata(func(in *mmodel.UpdateTransactionRouteInput) map[string]any { return in.Metadata })},
		{"holder", decodePatchMetadata(func(in *mmodel.UpdateHolderInput) map[string]any { return in.Metadata })},
		{"instrument", decodePatchMetadata(func(in *mmodel.UpdateInstrumentInput) map[string]any { return in.Metadata })},
	}

	// An empty map is a merge of nothing, so the stored metadata stays as it is; a nil value
	// deletes its key; a nil map clears the client keys.
	bodies := []struct {
		name string
		body string
		want map[string]any
	}{
		{name: "absent metadata leaves the stored metadata untouched", body: `{}`, want: map[string]any{}},
		{name: "empty metadata leaves the stored metadata untouched", body: `{"metadata":{}}`, want: map[string]any{}},
		{name: "a null-valued key deletes that key", body: `{"metadata":{"k":null}}`, want: map[string]any{"k": nil}},
		{name: "null metadata clears the client keys", body: `{"metadata":null}`, want: nil},
	}

	for _, entity := range entities {
		for _, body := range bodies {
			t.Run(entity.name+"/"+body.name, func(t *testing.T) {
				t.Parallel()

				got, err := entity.decode(body.body)
				require.NoError(t, err)
				assert.Equal(t, body.want, got)
			})
		}
	}
}
