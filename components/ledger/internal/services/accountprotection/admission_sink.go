// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountprotection

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Sink keeps the seed admissions a request took while loading balances alive until
// the accounting execution that consumes those seeds has answered.
//
// A cache-miss load takes the admission, proves the account open and rebuilds the
// seed. The seed is only admitted later, inside the engine, so the admission has
// to outlive the load: releasing it at the end of the read would let a closing
// evict and finish between the read and the execution, and the engine would then
// fill the miss with a snapshot of an account that is already closed.
//
// The sink is installed by the path that owns the whole flow — load, prepare,
// execute — and it is that path which decides when the admission ends. It is safe
// for concurrent use because one request may load balances from more than one
// goroutine.
type Sink struct {
	mu            sync.Mutex
	admissions    []*Admission
	indeterminate bool
}

type sinkContextKey struct{}

// ContextWithSink installs a sink on ctx and returns it. Every admission taken
// under the returned context is handed to that sink instead of being released
// when its load ends.
func ContextWithSink(ctx context.Context) (context.Context, *Sink) {
	sink := &Sink{}

	return context.WithValue(ctx, sinkContextKey{}, sink), sink
}

// SinkFromContext returns the sink installed on ctx, or nil when the caller did
// not install one. Nil means the admission ends with the load that took it, which
// is the behavior of every path that does not execute accounting afterwards.
func SinkFromContext(ctx context.Context) *Sink {
	sink, _ := ctx.Value(sinkContextKey{}).(*Sink)

	return sink
}

// AdoptAdmission hands one admission to the sink installed on ctx and reports
// whether it was taken over. A false result means the caller keeps the admission
// and must release it itself.
func AdoptAdmission(ctx context.Context, admission *Admission) bool {
	sink := SinkFromContext(ctx)
	if sink == nil || admission == nil || len(admission.accountIDs) == 0 {
		return false
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()

	sink.admissions = append(sink.admissions, admission)

	return true
}

// TokenFor returns the administrative token that covers one account, or the empty
// string when this request owns no admission over it. The scope is compared in
// full: a token taken over an account of another organization or ledger never
// answers here.
//
// The token stays inside the services and the cache adapter. It is not a
// transaction generation and never reaches a receipt, a recovery record, a span,
// a log or any public contract.
func (s *Sink) TokenFor(organizationID, ledgerID, accountID uuid.UUID) string {
	if s == nil {
		return ""
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, admission := range s.admissions {
		if admission.organizationID != organizationID || admission.ledgerID != ledgerID {
			continue
		}

		for _, owned := range admission.accountIDs {
			if owned == accountID {
				return admission.token
			}
		}
	}

	return ""
}

// MarkIndeterminate records that the execution the admissions were held for could
// not be resolved, so Release keeps every one of them in place for reconciliation.
func (s *Sink) MarkIndeterminate() {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.indeterminate = true

	for _, admission := range s.admissions {
		admission.MarkIndeterminate()
	}
}

// Release releases every admission the sink collected, in the reverse order in
// which they were taken. Each one then decides for itself: an admission marked
// indeterminate keeps its hold for reconciliation rather than releasing it (see
// Admission.Release), because work that may still land must not lose its
// protection on the way out.
func (s *Sink) Release(ctx context.Context) {
	if s == nil {
		return
	}

	s.mu.Lock()
	admissions := s.admissions
	s.admissions = nil
	s.mu.Unlock()

	for i := len(admissions) - 1; i >= 0; i-- {
		admissions[i].Release(ctx)
	}
}
