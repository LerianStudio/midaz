// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query_test

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query/compiledmocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestCompiledContextPolicyUsesCurrentBinding(t *testing.T) {
	repo := mocks.NewMockActiveContextPolicyRepository(gomock.NewController(t))
	compiler := compiledmocks.NewMockContextPolicyCompiler(gomock.NewController(t))
	resolver, err := query.NewResolveContextPolicyQuery(repo, 10)
	require.NoError(t, err)
	cache, err := query.NewCompiledContextPolicyQuery(resolver, compiler, query.CompiledPolicyCacheConfig{MaxEntries: 2, MaxCompilations: 1, SingleTenant: true})
	require.NoError(t, err)
	evaluator := policyEvaluator(t, policyEngine(t), 100000)
	policy := policySnapshot()
	program, err := evaluator.Compile(t.Context(), policy)
	require.NoError(t, err)
	ctx := producerContext()
	for _, version := range []int64{1, 2} {
		repo.EXPECT().GetActive(gomock.Any(), model.PolicyBindingKey{IntegrationID: "producer", ContextID: "official"}).Return(&model.BoundContextPolicy{ContextPolicy: policy, BindingVersion: version}, nil)
		if version == 1 {
			compiler.EXPECT().Compile(gomock.Any(), policy).Return(program, nil)
		}
		result, err := cache.Execute(ctx, "official")
		require.NoError(t, err)
		require.Same(t, program, result.Program)
		require.Equal(t, version, result.Resolved.Policy.BindingVersion)
	}
	repo.EXPECT().GetActive(gomock.Any(), gomock.Any()).Return(nil, errors.New("binding unavailable"))
	result, err := cache.Execute(ctx, "official")
	require.Error(t, err)
	require.Nil(t, result)
	// Same policy identity in another tenant cannot hit this tenant's program.
	repo.EXPECT().GetActive(gomock.Any(), gomock.Any()).Return(&model.BoundContextPolicy{ContextPolicy: policy, BindingVersion: 1}, nil)
	compiler.EXPECT().Compile(gomock.Any(), policy).Return(program, nil)
	_, err = cache.Execute(tmcore.ContextWithTenantID(ctx, "other-tenant"), "official")
	require.NoError(t, err)
}

func TestCompiledContextPolicyFailureDoesNotPoisonCache(t *testing.T) {
	repo := mocks.NewMockActiveContextPolicyRepository(gomock.NewController(t))
	compiler := compiledmocks.NewMockContextPolicyCompiler(gomock.NewController(t))
	resolver, err := query.NewResolveContextPolicyQuery(repo, 10)
	require.NoError(t, err)
	cache, err := query.NewCompiledContextPolicyQuery(resolver, compiler, query.CompiledPolicyCacheConfig{MaxEntries: 1, MaxCompilations: 1, SingleTenant: true})
	require.NoError(t, err)
	policy := policySnapshot()
	repo.EXPECT().GetActive(gomock.Any(), gomock.Any()).Return(&model.BoundContextPolicy{ContextPolicy: policy, BindingVersion: 1}, nil).Times(2)
	compiler.EXPECT().Compile(gomock.Any(), policy).Return(nil, context.Canceled)
	_, err = cache.Execute(producerContext(), "official")
	require.ErrorIs(t, err, context.Canceled)
	evaluator := policyEvaluator(t, policyEngine(t), 100000)
	program, err := evaluator.Compile(t.Context(), policy)
	require.NoError(t, err)
	compiler.EXPECT().Compile(gomock.Any(), policy).Return(program, nil)
	result, err := cache.Execute(producerContext(), "official")
	require.NoError(t, err)
	require.Same(t, program, result.Program)
}

func TestCompiledContextPolicyEvictsAndRejectsMissingTenant(t *testing.T) {
	repo := mocks.NewMockActiveContextPolicyRepository(gomock.NewController(t))
	compiler := compiledmocks.NewMockContextPolicyCompiler(gomock.NewController(t))
	resolver, err := query.NewResolveContextPolicyQuery(repo, 10)
	require.NoError(t, err)
	cache, err := query.NewCompiledContextPolicyQuery(resolver, compiler, query.CompiledPolicyCacheConfig{MaxEntries: 1, MaxCompilations: 1})
	require.NoError(t, err)
	_, err = cache.Execute(producerContext(), "official")
	require.ErrorIs(t, err, constant.ErrReservationTenantRequired)
	policy := policySnapshot()
	program, err := policyEvaluator(t, policyEngine(t), 100000).Compile(t.Context(), policy)
	require.NoError(t, err)
	repo.EXPECT().GetActive(gomock.Any(), gomock.Any()).Return(&model.BoundContextPolicy{ContextPolicy: policy, BindingVersion: 1}, nil).Times(3)
	compiler.EXPECT().Compile(gomock.Any(), policy).Return(program, nil).Times(3)
	for _, tenant := range []string{"first", "second", "first"} {
		_, err = cache.Execute(tmcore.ContextWithTenantID(producerContext(), tenant), "official")
		require.NoError(t, err)
	}
}

func TestCompiledContextPolicyBoundsConcurrentCompilation(t *testing.T) {
	repo := mocks.NewMockActiveContextPolicyRepository(gomock.NewController(t))
	compiler := compiledmocks.NewMockContextPolicyCompiler(gomock.NewController(t))
	resolver, err := query.NewResolveContextPolicyQuery(repo, 10)
	require.NoError(t, err)
	cache, err := query.NewCompiledContextPolicyQuery(resolver, compiler, query.CompiledPolicyCacheConfig{MaxEntries: 2, MaxCompilations: 1, SingleTenant: true})
	require.NoError(t, err)
	policy := policySnapshot()
	program, err := policyEvaluator(t, policyEngine(t), 100000).Compile(t.Context(), policy)
	require.NoError(t, err)
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	leaderCtx, cancelLeader := context.WithCancel(producerContext())
	defer cancelLeader()
	compiler.EXPECT().Compile(gomock.Any(), policy).DoAndReturn(func(ctx context.Context, _ model.ContextPolicy) (*query.CompiledContextPolicy, error) {
		close(started)
		select {
		case <-release:
			return program, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	repo.EXPECT().GetActive(gomock.Any(), gomock.Any()).Return(&model.BoundContextPolicy{ContextPolicy: policy, BindingVersion: 1}, nil).Times(2)
	done := make(chan error, 1)
	go func() { _, callErr := cache.Execute(leaderCtx, "official"); done <- callErr }()
	<-started
	_, err = cache.Execute(tmcore.ContextWithTenantID(producerContext(), "different"), "official")
	require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
	waiterCtx, cancelWaiter := context.WithCancel(producerContext())
	repo.EXPECT().GetActive(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, model.PolicyBindingKey) (*model.BoundContextPolicy, error) {
		cancelWaiter()
		return &model.BoundContextPolicy{ContextPolicy: policy, BindingVersion: 1}, nil
	})
	_, err = cache.Execute(waiterCtx, "official")
	require.ErrorIs(t, err, context.Canceled)
	cancelLeader()
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestCompiledContextPolicySharesConcurrentProgram(t *testing.T) {
	const callers = 16
	repo := mocks.NewMockActiveContextPolicyRepository(gomock.NewController(t))
	compiler := compiledmocks.NewMockContextPolicyCompiler(gomock.NewController(t))
	resolver, err := query.NewResolveContextPolicyQuery(repo, 10)
	require.NoError(t, err)
	cache, err := query.NewCompiledContextPolicyQuery(resolver, compiler, query.CompiledPolicyCacheConfig{MaxEntries: 1, MaxCompilations: 1, SingleTenant: true})
	require.NoError(t, err)
	policy := policySnapshot()
	program, err := policyEvaluator(t, policyEngine(t), 100000).Compile(t.Context(), policy)
	require.NoError(t, err)
	resolved := make(chan struct{}, callers)
	release := make(chan struct{})
	repo.EXPECT().GetActive(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, model.PolicyBindingKey) (*model.BoundContextPolicy, error) {
		resolved <- struct{}{}
		return &model.BoundContextPolicy{ContextPolicy: policy, BindingVersion: 1}, nil
	}).Times(callers)
	compiler.EXPECT().Compile(gomock.Any(), policy).DoAndReturn(func(context.Context, model.ContextPolicy) (*query.CompiledContextPolicy, error) {
		<-release
		return program, nil
	}).Times(1)
	results := make(chan *query.PreparedContextPolicy, callers)
	failures := make(chan error, callers)
	for range callers {
		go func() {
			result, err := cache.Execute(producerContext(), "official")
			results <- result
			failures <- err
		}()
	}
	for range callers {
		<-resolved
	}
	close(release)
	for range callers {
		require.NoError(t, <-failures)
		require.Same(t, program, (<-results).Program)
	}
}

func TestCompiledContextPolicyRevisionAndEmptyCompilation(t *testing.T) {
	repo := mocks.NewMockActiveContextPolicyRepository(gomock.NewController(t))
	compiler := compiledmocks.NewMockContextPolicyCompiler(gomock.NewController(t))
	resolver, err := query.NewResolveContextPolicyQuery(repo, 10)
	require.NoError(t, err)
	config := query.CompiledPolicyCacheConfig{MaxEntries: 2, MaxCompilations: 1, SingleTenant: true}
	_, err = query.NewCompiledContextPolicyQuery(nil, compiler, config)
	require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
	_, err = query.NewCompiledContextPolicyQuery(resolver, nil, config)
	require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
	_, err = query.NewCompiledContextPolicyQuery(resolver, compiler, query.CompiledPolicyCacheConfig{})
	require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
	cache, err := query.NewCompiledContextPolicyQuery(resolver, compiler, config)
	require.NoError(t, err)
	policy := policySnapshot()
	repo.EXPECT().GetActive(gomock.Any(), gomock.Any()).Return(&model.BoundContextPolicy{ContextPolicy: policy, BindingVersion: 1}, nil)
	compiler.EXPECT().Compile(gomock.Any(), policy).Return(nil, nil)
	result, err := cache.Execute(producerContext(), "official")
	require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
	require.Nil(t, result)
	evaluator := policyEvaluator(t, policyEngine(t), 100000)
	for _, revision := range []int64{policy.Revision, policy.Revision + 1} {
		policy.Revision = revision
		program, err := evaluator.Compile(t.Context(), policy)
		require.NoError(t, err)
		repo.EXPECT().GetActive(gomock.Any(), gomock.Any()).Return(&model.BoundContextPolicy{ContextPolicy: policy, BindingVersion: revision}, nil)
		compiler.EXPECT().Compile(gomock.Any(), policy).Return(program, nil)
		result, err = cache.Execute(producerContext(), "official")
		require.NoError(t, err)
		require.Same(t, program, result.Program)
	}
}
