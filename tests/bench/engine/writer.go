//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// posting moves Amount on one balance: positive credits, negative debits. AccountID keys the account control.
type posting struct {
	BalanceID uuid.UUID `json:"balanceId"`
	AccountID uuid.UUID `json:"accountId"`
	Amount    int64     `json:"amount"`
}

// request is one identity's frozen payload. A retry keeps identity, payload, scheduledAt and admission cutoff and
// may only wait under a new response deadline; it never mints a transaction ID or renews admission. newRequest
// derives cutoff and deadline from scheduledAt, never from dispatch.
type request struct {
	id, transactionID                      uuid.UUID
	postings                               []posting
	fingerprint                            [32]byte // the transaction ID and postings, never times or attempts
	scheduledAt, admissionCutoff, deadline time.Time
}

// admissible is the pure entry check: at the cutoff instant the request is already late.
func (r request) admissible(now time.Time) bool { return now.Before(r.admissionCutoff) }

type outcome string

const (
	outcomeApproved outcome = "approved"
	outcomeRefused  outcome = "refused"
	outcomeExpired  outcome = "expired"
	outcomeReplay   outcome = "replay"
	outcomeConflict outcome = "conflict"
	outcomeUnknown  outcome = "unknown"
)

// result is one attempt's answer. Zero timestamps are unmeasured on that path, never a zero latency.
type result struct {
	requestID                               uuid.UUID
	outcome, original                       outcome // original: the stored outcome a replay repeats
	reason                                  string
	transactionID                           uuid.UUID // set when money moved: approved, or a replay of approved
	decisionStartedAt, dbAckAt, brokerAckAt time.Time
}

// writer is the generator's only I/O port; owner, ownerless and transport adapters implement it.
type writer interface {
	// Submit answers by about req.deadline, unknown while unresolved, and never mutates the frozen postings. At most
	// one attempt per identity gets a new decision, none if callers time out; others replay it or, on a different
	// payload, conflict.
	Submit(ctx context.Context, req request) result
	// Close ends the writer; later Submits answer unknown. The caller closes once after its last run, on every path.
	Close(ctx context.Context) error
}

// budget is the caller's wait and the admission window from scheduledAt: experimental, not public defaults.
type budget struct{ caller, admission time.Duration }

var (
	usualBudget   = budget{caller: 30 * time.Second, admission: 28 * time.Second}
	backlogBudget = budget{caller: 2 * time.Second, admission: 1500 * time.Millisecond}
)

// newRequest is the one place request invariants are checked; it freezes its own copy of the postings.
func newRequest(id, transactionID uuid.UUID, postings []posting, scheduledAt time.Time, b budget) (request, error) {
	valid := id != uuid.Nil && transactionID != uuid.Nil && id != transactionID && len(postings) >= 2

	var sum int64

	for _, p := range postings {
		valid = valid && p.Amount != 0 && p.BalanceID != uuid.Nil && p.AccountID != uuid.Nil
		sum += p.Amount
	}

	if !valid || sum != 0 {
		return request{}, fmt.Errorf("request %s: want distinct non-nil IDs and two or more keyed, non-zero postings summing to zero", id)
	}

	frozen := slices.Clone(postings)

	payload, err := json.Marshal(struct {
		TransactionID uuid.UUID `json:"transactionId"`
		Postings      []posting `json:"postings"`
	}{transactionID, frozen})
	if err != nil {
		return request{}, fmt.Errorf("fingerprint request %s: %w", id, err)
	}

	return request{
		id: id, transactionID: transactionID, postings: frozen, fingerprint: sha256.Sum256(payload),
		scheduledAt: scheduledAt, admissionCutoff: scheduledAt.Add(b.admission), deadline: scheduledAt.Add(b.caller),
	}, nil
}
