// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"sync"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// CompiledPolicyCacheConfig bounds retained programs and concurrent compilations.
// The compiler's own limits bound each program's size and compilation work.
type CompiledPolicyCacheConfig struct {
	MaxEntries      int
	MaxCompilations int
	SingleTenant    bool
}

// PreparedContextPolicy retains the freshly resolved binding with its program.
// It contains no transaction decision; callers must evaluate their own facts.
type PreparedContextPolicy struct {
	Resolved *ResolvedContextPolicy
	Program  *CompiledContextPolicy
}

type compiledPolicyKey struct {
	tenant      string
	integration string
	namespace   string
	contextID   string
	policyID    uuid.UUID
	revision    int64
}

type policyCompilation struct {
	done    chan struct{}
	program *CompiledContextPolicy
	err     error
}

// CompiledContextPolicyQuery caches only immutable policy revisions. Every call
// resolves the active binding from its repository before consulting the cache.
// Compiler configuration belongs to this instance: reconfiguration requires a
// new compiler and cache. FIFO eviction does not affect binding freshness.
type CompiledContextPolicyQuery struct {
	resolver *ResolveContextPolicyQuery
	compiler ContextPolicyCompiler
	config   CompiledPolicyCacheConfig
	mu       sync.Mutex
	entries  map[compiledPolicyKey]*CompiledContextPolicy
	order    []compiledPolicyKey
	flights  map[compiledPolicyKey]*policyCompilation
}

func NewCompiledContextPolicyQuery(resolver *ResolveContextPolicyQuery, compiler ContextPolicyCompiler, config CompiledPolicyCacheConfig) (*CompiledContextPolicyQuery, error) {
	if resolver == nil || compiler == nil || config.MaxEntries <= 0 || config.MaxCompilations <= 0 {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return &CompiledContextPolicyQuery{resolver: resolver, compiler: compiler, config: config, entries: make(map[compiledPolicyKey]*CompiledContextPolicy), flights: make(map[compiledPolicyKey]*policyCompilation)}, nil
}

// Execute requires authenticated producer and resolved tenant database context.
// A binding read failure never falls back to a previously cached binding.
func (q *CompiledContextPolicyQuery) Execute(ctx context.Context, contextID string) (_ *PreparedContextPolicy, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.compiled_context_policy")
	defer span.End()
	defer func() { recordContextPolicyError(span, retErr) }()

	tenant := tmcore.GetTenantIDContext(ctx)
	if tenant == "" && !q.config.SingleTenant {
		return nil, constant.ErrReservationTenantRequired
	}

	resolved, err := q.resolver.Execute(ctx, contextID)
	if err != nil {
		return nil, err
	}

	key := compiledPolicyKey{tenant: tenant, integration: resolved.Identity.ID, namespace: resolved.Identity.AssetNamespace, contextID: contextID, policyID: resolved.Policy.ID, revision: resolved.Policy.Revision}

	program, err := q.program(ctx, key, resolved)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Context policy program prepared")

	return &PreparedContextPolicy{Resolved: resolved, Program: program}, nil
}

func (q *CompiledContextPolicyQuery) program(ctx context.Context, key compiledPolicyKey, resolved *ResolvedContextPolicy) (*CompiledContextPolicy, error) {
	q.mu.Lock()
	if program, ok := q.entries[key]; ok {
		q.mu.Unlock()
		return program, nil
	}

	if flight, ok := q.flights[key]; ok {
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-flight.done:
			return flight.program, flight.err
		}
	}

	if len(q.flights) >= q.config.MaxCompilations {
		q.mu.Unlock()
		return nil, constant.ErrContextPolicyUnavailable
	}

	flight := &policyCompilation{done: make(chan struct{})}
	q.flights[key] = flight
	q.mu.Unlock()

	program, err := q.compiler.Compile(ctx, resolved.Policy.ContextPolicy)
	if err == nil {
		err = ctx.Err()
	}

	if err == nil && program == nil {
		err = constant.ErrContextPolicyUnavailable
	}

	q.finish(key, flight, program, err)

	return flight.program, flight.err
}

func (q *CompiledContextPolicyQuery) finish(key compiledPolicyKey, flight *policyCompilation, program *CompiledContextPolicy, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if err == nil {
		if len(q.entries) >= q.config.MaxEntries {
			delete(q.entries, q.order[0])
			q.order = q.order[1:]
		}

		q.entries[key] = program
		q.order = append(q.order, key)
		flight.program = program
	}

	flight.err = err

	delete(q.flights, key)
	close(flight.done)
}
