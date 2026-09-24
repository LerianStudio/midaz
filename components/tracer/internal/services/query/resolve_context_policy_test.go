// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query_test

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func producerContext() context.Context {
	return contextutil.WithIntegrationIdentity(context.Background(), contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "assets"})
}

func TestResolveContextPolicyUsesVerifiedIdentityAndPreservesTenant(t *testing.T) {
	t.Parallel()
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		t.Run(tenant, func(t *testing.T) {
			repo := mocks.NewMockActiveContextPolicyRepository(gomock.NewController(t))
			resolver, err := query.NewResolveContextPolicyQuery(repo, 10)
			require.NoError(t, err)
			ctx := tmcore.ContextWithTenantID(producerContext(), tenant)
			snapshot := &model.BoundContextPolicy{ContextPolicy: policySnapshot(), BindingVersion: 8}
			key := model.PolicyBindingKey{IntegrationID: "producer", ContextID: "official-context"}
			repo.EXPECT().GetActive(gomock.Any(), key).DoAndReturn(func(ctx context.Context, _ model.PolicyBindingKey) (*model.BoundContextPolicy, error) {
				require.Equal(t, tenant, tmcore.GetTenantIDContext(ctx))
				return snapshot, nil
			})
			result, err := resolver.Execute(ctx, "official-context")
			require.NoError(t, err)
			require.Equal(t, key, result.Binding)
			require.Equal(t, "assets", result.Identity.AssetNamespace)
			require.Equal(t, *snapshot, result.Policy)
			// Detached rules prevent a repository/cache owner mutating the result.
			require.NotEmpty(t, snapshot.Rules)
			snapshot.Rules[0].Expression = "false"
			require.NotEqual(t, snapshot.Rules[0].Expression, result.Policy.Rules[0].Expression)
		})
	}
}

func TestResolveContextPolicyRejectsBeforeRepository(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(producerContext())
	cancel()
	for _, tc := range []struct {
		name      string
		ctx       context.Context
		contextID string
		want      error
	}{
		{"missing identity", context.Background(), "official", constant.ErrInsufficientPrivileges},
		{"admin is not producer", contextutil.WithPrincipal(context.Background(), contextutil.Principal{Type: "user", ID: "admin"}), "official", constant.ErrInsufficientPrivileges},
		{"invalid identity", contextutil.WithIntegrationIdentity(context.Background(), contextutil.IntegrationIdentity{ID: "producer"}), "official", constant.ErrInsufficientPrivileges},
		{"missing context", producerContext(), "", constant.ErrInvalidRequestBody},
		{"noncanonical context", producerContext(), " official", constant.ErrInvalidRequestBody},
		{"canceled", canceled, "official", context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockActiveContextPolicyRepository(gomock.NewController(t))
			resolver, err := query.NewResolveContextPolicyQuery(repo, 10)
			require.NoError(t, err)
			result, err := resolver.Execute(tc.ctx, tc.contextID)
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, result)
		})
	}
}

func TestResolveContextPolicyNeverFallsBack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		policy *model.BoundContextPolicy
		err    error
		want   error
	}{
		{"not configured", nil, constant.ErrContextPolicyUnavailable, constant.ErrContextPolicyUnavailable},
		{"nil snapshot", nil, nil, constant.ErrContextPolicyUnavailable},
		{"unversioned binding", &model.BoundContextPolicy{ContextPolicy: policySnapshot()}, nil, constant.ErrContextPolicyUnavailable},
		{"invalid snapshot", &model.BoundContextPolicy{BindingVersion: 1}, nil, constant.ErrContextPolicyUnavailable},
		{"technical failure", nil, constant.ErrInternalServer, constant.ErrInternalServer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockActiveContextPolicyRepository(gomock.NewController(t))
			resolver, err := query.NewResolveContextPolicyQuery(repo, 10)
			require.NoError(t, err)
			repo.EXPECT().GetActive(gomock.Any(), model.PolicyBindingKey{IntegrationID: "producer", ContextID: "official"}).Return(tc.policy, tc.err)
			result, err := resolver.Execute(producerContext(), "official")
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, result)
		})
	}
}

func TestResolveContextPolicyChecksCancellationAfterRead(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockActiveContextPolicyRepository(gomock.NewController(t))
	resolver, err := query.NewResolveContextPolicyQuery(repo, 10)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(producerContext())
	defer cancel()
	repo.EXPECT().GetActive(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, model.PolicyBindingKey) (*model.BoundContextPolicy, error) {
		cancel()
		return &model.BoundContextPolicy{ContextPolicy: policySnapshot(), BindingVersion: 1}, nil
	})
	result, err := resolver.Execute(ctx, "official")
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
}
