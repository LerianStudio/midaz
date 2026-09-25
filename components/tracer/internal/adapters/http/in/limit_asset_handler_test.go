// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func assetAdminBounds() tracercontract.Limits {
	return tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 30, MaxFractionDigits: 20}
}

func TestLimitAssetHandler(t *testing.T) {
	for _, scenario := range []string{"success", "missing identity", "namespace forged", "missing facts", "unknown field", "oversize", "conflict", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			binder := NewMockLimitAssetBinder(gomock.NewController(t))
			resolver, err := seamidentity.NewResolver([]seamidentity.Binding{{URI: "spiffe://example.test/ledger", IntegrationID: "ledger", AssetNamespace: "ledger", Purposes: []seamidentity.Purpose{seamidentity.PurposeAssetAdmin}}}, 256)
			require.NoError(t, err)
			h, err := NewLimitAssetHandler(binder, resolver, assetAdminBounds(), 4096)
			require.NoError(t, err)
			id := testutil.MustDeterministicUUID(89101)
			facts := []tracercontract.AccountAsset{{AccountID: testutil.MustDeterministicUUID(89102), Asset: tracercontract.AssetRef{Namespace: "ledger", ID: "official", Code: "wBTC"}}}
			ctx := contextutil.WithIntegrationIdentity(t.Context(), contextutil.IntegrationIdentity{ID: "ledger", AssetNamespace: "ledger"})
			switch scenario {
			case "missing identity":
				ctx = t.Context()
			case "namespace forged":
				facts[0].Asset.Namespace = "forged"
			case "missing facts":
				facts = nil
			}
			raw, err := json.Marshal(LimitAssetDocument{AccountAssets: facts})
			require.NoError(t, err)
			if scenario == "unknown field" {
				raw = []byte(`{"accountAssets":[],"integrationId":"forged"}`)
			}
			if scenario == "oversize" {
				raw = make([]byte, 4097)
			}
			switch scenario {
			case "success":
				binder.EXPECT().Execute(gomock.Any(), id, facts).Return(&facts[0].Asset, nil)
			case "conflict":
				binder.EXPECT().Execute(gomock.Any(), id, facts).Return(nil, constant.ErrLimitAssetReferenceConflict)
			case "unavailable":
				binder.EXPECT().Execute(gomock.Any(), id, facts).Return(nil, constant.ErrContextLimitsUnavailable)
			}
			result, err := h.Bind(ctx, &BindLimitAssetInput{ID: id.String(), RawBody: raw})
			if scenario == "success" {
				require.NoError(t, err)
				require.Equal(t, facts[0].Asset, *result.Body)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
			}
		})
	}
}
