// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

type patchMetadataDecoder func(body string, policy metadataNullPolicy) (metadata map[string]any, removed []string, err error)

func decodePatchMetadata[T any](metadata func(*T) *map[string]any) patchMetadataDecoder {
	return func(body string, policy metadataNullPolicy) (map[string]any, []string, error) {
		in := new(T)
		originalMap, err := decodePatchBody([]byte(body), in, metadata(in), policy)

		return *metadata(in), pkgHTTP.FindNilFields(originalMap, ""), err
	}
}

// TestDecodePatchBodyMetadata pins what every metadata-bearing PATCH body hands its use case on
// each contract that serves it: the metadata map, and the nil fields a CRM patch unsets.
func TestDecodePatchBodyMetadata(t *testing.T) {
	t.Parallel()

	v1Only := []metadataNullPolicy{metadataNullClearsV1}
	v2Only := []metadataNullPolicy{metadataNullKeepsV2}
	both := append(v1Only, v2Only...)

	entities := []struct {
		name      string
		contracts []metadataNullPolicy
		decode    patchMetadataDecoder
	}{
		{"organization", both, decodePatchMetadata(func(in *mmodel.UpdateOrganizationInput) *map[string]any { return &in.Metadata })},
		{"ledger", both, decodePatchMetadata(func(in *mmodel.UpdateLedgerInput) *map[string]any { return &in.Metadata })},
		{"portfolio", both, decodePatchMetadata(func(in *mmodel.UpdatePortfolioInput) *map[string]any { return &in.Metadata })},
		{"segment", both, decodePatchMetadata(func(in *mmodel.UpdateSegmentInput) *map[string]any { return &in.Metadata })},
		{"account", both, decodePatchMetadata(func(in *mmodel.UpdateAccountInput) *map[string]any { return &in.Metadata })},
		{"account type", both, decodePatchMetadata(func(in *mmodel.UpdateAccountTypeInput) *map[string]any { return &in.Metadata })},
		{"asset", both, decodePatchMetadata(func(in *mmodel.UpdateAssetInput) *map[string]any { return &in.Metadata })},
		{"transaction", both, decodePatchMetadata(func(in *transaction.UpdateTransactionInput) *map[string]any { return &in.Metadata })},
		{"operation", both, decodePatchMetadata(func(in *operation.UpdateOperationInput) *map[string]any { return &in.Metadata })},
		{"operation route", both, decodePatchMetadata(func(in *mmodel.UpdateOperationRouteInput) *map[string]any { return &in.Metadata })},
		{"transaction route", both, decodePatchMetadata(func(in *mmodel.UpdateTransactionRouteInput) *map[string]any { return &in.Metadata })},
		{"holder", v2Only, decodePatchMetadata(func(in *mmodel.UpdateHolderInput) *map[string]any { return &in.Metadata })},
		{"instrument", v2Only, decodePatchMetadata(func(in *mmodel.UpdateInstrumentInput) *map[string]any { return &in.Metadata })},
	}

	// An empty map merges nothing, so the stored metadata stays as it is; a nil value deletes its
	// key; a nil map clears the client keys.
	bodies := []struct {
		name        string
		body        string
		want        map[string]any
		wantRemoved []string
		on          []metadataNullPolicy
	}{
		{name: "absent metadata leaves the stored metadata untouched", body: `{}`, want: map[string]any{}, wantRemoved: []string{}, on: both},
		{name: "empty metadata leaves the stored metadata untouched", body: `{"metadata":{}}`, want: map[string]any{}, wantRemoved: []string{}, on: both},
		{name: "a null-valued key deletes that key", body: `{"metadata":{"k":null}}`, want: map[string]any{"k": nil}, wantRemoved: []string{"metadata.k"}, on: both},
		{name: "v1 null metadata clears the client keys", body: `{"metadata":null}`, wantRemoved: []string{"metadata"}, on: v1Only},
		{name: "v2 null metadata leaves the stored metadata untouched", body: `{"metadata":null}`, want: map[string]any{}, wantRemoved: []string{}, on: v2Only},
	}

	for _, entity := range entities {
		for _, contract := range entity.contracts {
			for _, body := range bodies {
				if !slices.Contains(body.on, contract) {
					continue
				}

				t.Run(entity.name+"/"+body.name, func(t *testing.T) {
					t.Parallel()

					got, removed, err := entity.decode(body.body, contract)
					require.NoError(t, err)
					assert.Equal(t, body.want, got)
					assert.Equal(t, body.wantRemoved, removed)
				})
			}
		}
	}
}
