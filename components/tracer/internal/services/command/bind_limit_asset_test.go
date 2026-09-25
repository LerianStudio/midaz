// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func limitAssetBindingFixture() (model.ContextAccountLimit, []tracercontract.AccountAsset, LimitAssetBindingConfig) {
	account := testutil.MustDeterministicUUID(83001)
	asset := tracercontract.AssetRef{Namespace: "ledger", ID: "opaque-asset", Code: "USD"}
	limit := model.ContextAccountLimit{Definition: model.Limit{ID: testutil.MustDeterministicUUID(83002), Asset: "USD", Status: model.LimitStatusDraft, LimitType: model.LimitTypeDaily, MaxAmount: decimal.NewFromInt(100), Scopes: []model.Scope{{AccountID: &account}}}}
	return limit, []tracercontract.AccountAsset{{AccountID: account, Asset: asset}}, LimitAssetBindingConfig{SingleTenant: true, MaxScopes: 10, Facts: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 30, MaxFractionDigits: 20}}
}

func limitAssetBindingContext(ctx context.Context) context.Context {
	ctx = contextutil.WithIntegrationIdentity(ctx, contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "ledger"})
	return contextutil.WithPrincipal(ctx, contextutil.Principal{ID: "operator", Type: "user"})
}

func TestBindLimitAssetAtomicity(t *testing.T) {
	failure := errors.New("database failure")
	for _, scenario := range []string{"success", "begin failure", "read failure", "already bound", "scope changed", "extra account", "code mismatch", "mixed assets", "unsupported scope", "write failure", "audit failure", "commit failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := mocks.NewMockLimitAssetRepository(ctrl)
			audit := mocks.NewMockAuditEventRepository(ctrl)
			tx := dbmocks.NewMockTx(ctrl)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			limit, facts, config := limitAssetBindingFixture()
			command, err := NewBindLimitAssetCommand(repo, audit, beginner, clock.NewFixedClock(testutil.FixedTime()), config)
			require.NoError(t, err)
			calls := []any{}
			if scenario == "begin failure" {
				beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, failure)
				result, err := command.Execute(limitAssetBindingContext(t.Context()), limit.Definition.ID, facts)
				require.ErrorIs(t, err, failure)
				require.Nil(t, result)
				return
			}
			readErr := error(nil)
			want := error(nil)
			switch scenario {
			case "read failure":
				readErr = failure
				want = failure
			case "already bound":
				limit.Asset = facts[0].Asset
				want = constant.ErrLimitAssetReferenceConflict
			case "scope changed":
				other := testutil.MustDeterministicUUID(83003)
				limit.Definition.Scopes[0].AccountID = &other
				want = constant.ErrContextLimitsUnavailable
			case "extra account":
				facts = append(facts, tracercontract.AccountAsset{AccountID: testutil.MustDeterministicUUID(83003), Asset: facts[0].Asset})
				want = constant.ErrContextLimitsUnavailable

			case "mixed assets":
				other := testutil.MustDeterministicUUID(83003)
				asset := facts[0].Asset
				asset.ID = "another-asset"
				facts = append(facts, tracercontract.AccountAsset{AccountID: other, Asset: asset})
				limit.Definition.Scopes = append(limit.Definition.Scopes, model.Scope{AccountID: &other})
				want = constant.ErrContextLimitsUnavailable
			case "unsupported scope":
				segment := testutil.MustDeterministicUUID(83003)
				limit.Definition.Scopes[0].SegmentID = &segment
				want = constant.ErrContextLimitsUnavailable
			case "code mismatch":
				limit.Definition.Asset = "EUR"
				want = constant.ErrContextLimitsUnavailable
			case "write failure", "audit failure", "commit failure":
				want = failure
			}
			calls = append(calls, beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil), repo.EXPECT().GetForAssetBindingWithTx(gomock.Any(), tx, limit.Definition.ID).Return(&limit, readErr))
			if scenario == "success" || scenario == "write failure" || scenario == "audit failure" || scenario == "commit failure" {
				writeErr := error(nil)
				if scenario == "write failure" {
					writeErr = failure
				}
				calls = append(calls, repo.EXPECT().BindAssetWithTx(gomock.Any(), tx, limit.Definition.ID, facts[0].Asset).Return(writeErr))
				if writeErr == nil {
					calls = append(calls, audit.EXPECT().InsertWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.DB, event *model.AuditEvent) error {
						require.Equal(t, model.AuditEventLimitUpdated, event.EventType)
						require.Equal(t, model.ResourceTypeLimit, event.ResourceType)
						require.Equal(t, model.AuditActionUpdate, event.Action)
						require.Equal(t, limit.Definition.ID.String(), event.ResourceID)
						require.Equal(t, "operator", event.Actor.ID)
						require.Equal(t, testutil.FixedTime(), event.CreatedAt)
						require.Equal(t, "producer", event.Context["integrationId"])
						require.Equal(t, "asset_reference_binding", event.Context["operation"])
						require.Equal(t, facts, event.Context["accountAssets"])
						before, ok := event.Context["before"].(map[string]any)
						require.True(t, ok)
						after, ok := event.Context["after"].(map[string]any)
						require.True(t, ok)
						require.Equal(t, model.LimitStatusDraft, before["status"])
						require.Equal(t, model.LimitStatusDraft, after["status"])
						require.Equal(t, limit.Definition.ID.String(), before["id"])
						require.Nil(t, before["assetRef"])
						require.Equal(t, facts[0].Asset, after["assetRef"])

						if scenario == "audit failure" {
							return failure
						}
						return nil
					}))
				}
			}
			if want == nil || scenario == "commit failure" {
				calls = append(calls, tx.EXPECT().Commit().Return(want))
			}
			if want != nil {
				calls = append(calls, tx.EXPECT().Rollback().Return(nil))
			}
			gomock.InOrder(calls...)
			result, err := command.Execute(limitAssetBindingContext(t.Context()), limit.Definition.ID, facts)
			if want != nil {
				require.ErrorIs(t, err, want)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, facts[0].Asset, *result)
			}
			require.Equal(t, model.LimitStatusDraft, limit.Definition.Status)
		})
	}
}

func TestBindLimitAssetRequiresBothIdentities(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockLimitAssetRepository(ctrl)
	audit := mocks.NewMockAuditEventRepository(ctrl)
	beginner := dbmocks.NewMockTxBeginner(ctrl)
	limit, facts, config := limitAssetBindingFixture()
	command, err := NewBindLimitAssetCommand(repo, audit, beginner, clock.NewFixedClock(testutil.FixedTime()), config)
	require.NoError(t, err)
	contexts := []context.Context{t.Context(), contextutil.WithPrincipal(t.Context(), contextutil.Principal{ID: "admin", Type: "user"}), contextutil.WithIntegrationIdentity(t.Context(), contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "ledger"}), contextutil.WithPrincipal(limitAssetBindingContext(t.Context()), contextutil.Principal{ID: "key", Type: "api_key"})}
	for _, ctx := range contexts {
		result, err := command.Execute(ctx, limit.Definition.ID, facts)
		require.Error(t, err)
		require.Nil(t, result)
	}
	facts[0].Asset.Namespace = "forged"
	result, err := command.Execute(limitAssetBindingContext(t.Context()), limit.Definition.ID, facts)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	require.Nil(t, result)
	_, err = command.Execute(limitAssetBindingContext(t.Context()), uuid.Nil, facts)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = command.Execute(ctx, limit.Definition.ID, facts)
	require.ErrorIs(t, err, context.Canceled)
}
