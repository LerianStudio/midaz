// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
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

// Compilation is shared across callers and therefore has its own deadline.
const policyCompilationTimeout = 5 * time.Second

type compiledPolicyKey struct {
	tenant   string
	policyID uuid.UUID
	revision int64
}

type compiledPolicyEntry struct {
	key     compiledPolicyKey
	program *CompiledContextPolicy
}

// CompiledContextPolicyQuery caches only immutable policy revisions. Every call
// resolves the active binding from its repository before consulting the cache.
// Compiler configuration belongs to this instance: reconfiguration requires a
// new compiler and cache. LRU eviction does not affect binding freshness.
type CompiledContextPolicyQuery struct {
	resolver  *ResolveContextPolicyQuery
	compiler  ContextPolicyCompiler
	config    CompiledPolicyCacheConfig
	mu        sync.Mutex
	entries   map[compiledPolicyKey]*list.Element
	order     *list.List
	flights   singleflight.Group
	compiling chan struct{}
}

func NewCompiledContextPolicyQuery(resolver *ResolveContextPolicyQuery, compiler ContextPolicyCompiler, config CompiledPolicyCacheConfig) (*CompiledContextPolicyQuery, error) {
	if resolver == nil || compiler == nil || config.MaxEntries <= 0 || config.MaxCompilations <= 0 {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return &CompiledContextPolicyQuery{resolver: resolver, compiler: compiler, config: config, entries: make(map[compiledPolicyKey]*list.Element), order: list.New(), compiling: make(chan struct{}, config.MaxCompilations)}, nil
}

// Execute requires authenticated producer and resolved tenant database context.
// A binding read failure never falls back to a previously cached binding.
func (q *CompiledContextPolicyQuery) Execute(ctx context.Context, contextID string) (*PreparedContextPolicy, error) {
	return q.prepare(ctx, contextID, q.resolver.Execute)
}

// ExecuteWithTx uses the admission connection for current binding resolution.
func (q *CompiledContextPolicyQuery) ExecuteWithTx(ctx context.Context, tx pgdb.Tx, contextID string) (*PreparedContextPolicy, error) {
	return q.prepare(ctx, contextID, func(ctx context.Context, id string) (*ResolvedContextPolicy, error) {
		return q.resolver.ExecuteWithTx(ctx, tx, id)
	})
}

func (q *CompiledContextPolicyQuery) prepare(ctx context.Context, contextID string, resolve func(context.Context, string) (*ResolvedContextPolicy, error)) (_ *PreparedContextPolicy, retErr error) {
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

	resolved, err := resolve(ctx, contextID)
	if err != nil {
		return nil, err
	}

	key := compiledPolicyKey{tenant: tenant, policyID: resolved.Policy.ID, revision: resolved.Policy.Revision}

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

func (q *CompiledContextPolicyQuery) cached(key compiledPolicyKey) *CompiledContextPolicy {
	q.mu.Lock()
	defer q.mu.Unlock()

	if element, ok := q.entries[key]; ok {
		q.order.MoveToBack(element)
		return element.Value.(compiledPolicyEntry).program
	}

	return nil
}

func (q *CompiledContextPolicyQuery) program(ctx context.Context, key compiledPolicyKey, resolved *ResolvedContextPolicy) (*CompiledContextPolicy, error) {
	if program := q.cached(key); program != nil {
		return program, nil
	}

	// Length-prefix the tenant to prevent ambiguous keys without constraining IDs.
	flightKey := fmt.Sprintf("%d:%s:%s:%d", len(key.tenant), key.tenant, key.policyID, key.revision)

	result := q.flights.DoChan(flightKey, func() (any, error) {
		if program := q.cached(key); program != nil {
			return program, nil
		}

		select {
		case q.compiling <- struct{}{}:
			defer func() { <-q.compiling }()
		default:
			return nil, constant.ErrContextPolicyUnavailable
		}

		compileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), policyCompilationTimeout)
		defer cancel()

		program, err := q.compile(compileCtx, resolved)
		if err != nil {
			return nil, err
		}

		q.mu.Lock()
		defer q.mu.Unlock()

		if len(q.entries) >= q.config.MaxEntries {
			oldest := q.order.Front()
			delete(q.entries, oldest.Value.(compiledPolicyEntry).key)
			q.order.Remove(oldest)
		}

		q.entries[key] = q.order.PushBack(compiledPolicyEntry{key: key, program: program})

		return program, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case completed := <-result:
		if completed.Err != nil {
			return nil, completed.Err
		}

		program, ok := completed.Val.(*CompiledContextPolicy)
		if !ok || program == nil {
			return nil, constant.ErrContextPolicyUnavailable
		}

		return program, nil
	}
}

// A compiler panic must not terminate singleflight's detached goroutine or
// publish a nil program. Resource limits bound CEL work in addition to time.
func (q *CompiledContextPolicyQuery) compile(ctx context.Context, resolved *ResolvedContextPolicy) (program *CompiledContextPolicy, retErr error) {
	defer func() {
		if program == nil && retErr == nil {
			retErr = constant.ErrContextPolicyUnavailable
		}
	}()

	logger, trackingTracer, headerID, requestID := libObservability.NewTrackingFromContext(ctx)
	_ = trackingTracer
	_ = headerID
	_ = requestID

	defer libRuntime.RecoverWithPolicyAndContext(ctx, logger, "tracer", "context-policy-compile", libRuntime.KeepRunning)

	compiled, err := q.compiler.Compile(ctx, resolved.Policy.ContextPolicy)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if compiled == nil {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return compiled, nil
}
