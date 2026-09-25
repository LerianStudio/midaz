// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// This opt-in measurement includes the real primary database, account locks,
// exact counters, policy cache and mandatory audit. It excludes TLS, transport,
// Ledger enrichment/journal and completion. Results are local observations, not
// production latency guarantees. Each request has a new durable identity.
func TestIntegrationReserveAdmissionMeasuredEnvelope(t *testing.T) {
	if os.Getenv("TRACER_MEASURE_ADMISSION") != "true" {
		t.Skip("set TRACER_MEASURE_ADMISSION=true for the isolated measurement")
	}
	for _, accounts := range []int{2, 10, 50, 100} {
		t.Run(fmt.Sprintf("accounts_%d", accounts), func(t *testing.T) {
			db := completionDatabase(t)
			admission, policies, request := admissionFixtureWithConnection(t, db, &testutil.IntegrationDBAdapter{DB: db}, testutil.FixedTime(), true, accounts)
			admissionPolicy(t, db, policies, model.DecisionAllow)
			limit := admissionLimit(t, db, request, 89501, "1000000000000")
			account, entry := request.Context.Accounts[0], request.Context.Entries[0]
			scopes := make([]model.Scope, 0, accounts)
			request.Context.Accounts = nil
			request.Context.Entries = nil
			for index := range accounts {
				next := account
				next.ID = testutil.MustDeterministicUUID(89510 + int64(index))
				nextEntry := entry
				nextEntry.AccountID = next.ID
				request.Context.Accounts = append(request.Context.Accounts, next)
				request.Context.Entries = append(request.Context.Entries, nextEntry)
				scopes = append(scopes, model.Scope{AccountID: &next.ID})
			}
			raw, err := json.Marshal(scopes)
			require.NoError(t, err)
			_, err = db.ExecContext(t.Context(), "UPDATE limits SET scopes=$2 WHERE id=$1", limit, raw)
			require.NoError(t, err)
			ctx := completionContext(t.Context(), "producer")
			iteration := 0
			measurementFailed := false
			result := testing.Benchmark(func(b *testing.B) {
				defer func() { measurementFailed = b.Failed() }()
				b.ReportAllocs()
				for b.Loop() {
					iteration++
					request.TransactionID = uuid.NewSHA1(account.ID, []byte(fmt.Sprintf("transaction-%d", iteration)))
					request.RequestID = uuid.NewSHA1(account.ID, []byte(fmt.Sprintf("request-%d", iteration)))
					response, err := admission.Execute(ctx, request)
					if err != nil {
						b.Fatal(err)
					}
					if response.Decision != tracercontract.DecisionAllow || len(response.ReservationIDs) != accounts {
						b.Fatalf("unexpected decision or reservation count")
					}
				}
			})
			require.False(t, measurementFailed)
			require.Positive(t, result.N)
			t.Logf("accounts=%d entries=%d rules=1 unique_requests=%d ns/op=%d bytes/op=%d allocs/op=%d", accounts, accounts, iteration, result.NsPerOp(), result.AllocedBytesPerOp(), result.AllocsPerOp())
			var decisions, reservations int
			require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM reserve_decisions").Scan(&decisions))
			require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM usage_reservations").Scan(&reservations))
			require.Equal(t, iteration, decisions)
			require.Equal(t, accounts*iteration, reservations)
		})
	}
}

// Request latencies include SQL pool waits and hot-account lock contention.
// This is the admission command, not network or end-to-end Ledger latency.
func TestIntegrationReserveAdmissionLatencyUnderContention(t *testing.T) {
	if os.Getenv("TRACER_MEASURE_ADMISSION") != "true" {
		t.Skip("set TRACER_MEASURE_ADMISSION=true for the isolated measurement")
	}
	const requests = 200
	const timeout = 10 * time.Second
	for _, workers := range []int{1, 4} {
		for _, ruleCount := range []int{1, 10} {
			t.Run(fmt.Sprintf("workers_%d_rules_%d", workers, ruleCount), func(t *testing.T) {
				db := completionDatabase(t)
				db.SetMaxOpenConns(8)
				db.SetMaxIdleConns(8)
				admission, policies, request := admissionFixture(t, db)
				rules := make([]model.ContextPolicyRule, ruleCount)
				for index := range rules {
					rules[index] = model.ContextPolicyRule{ID: testutil.MustDeterministicUUID(89650 + int64(index)), Revision: 1, Expression: `debits.all(d, d.amount.equal(decimal("10.125")))`, Action: model.DecisionAllow}
				}
				admissionPolicy(t, db, policies, model.DecisionAllow, rules...)
				limit := admissionLimit(t, db, request, 89649, "1000000000000")
				type observation struct {
					duration time.Duration
					err      error
				}
				results := make(chan observation, requests)
				start := make(chan struct{})
				var running sync.WaitGroup
				for worker := range workers {
					running.Go(func() {
						<-start
						for index := worker; index < requests; index += workers {
							next := request
							next.TransactionID = testutil.MustDeterministicUUID(90000 + int64(index))
							next.RequestID = testutil.MustDeterministicUUID(91000 + int64(index))
							ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), timeout)
							deadline, _ := ctx.Deadline()
							response, err := admission.Execute(ctx, next)
							elapsed := timeout - time.Until(deadline)
							cancel()
							if err == nil && (response == nil || response.Decision != tracercontract.DecisionAllow || len(response.ReservationIDs) != 1) {
								err = fmt.Errorf("unexpected response for request %d", index)
							}
							results <- observation{duration: elapsed, err: err}
						}
					})
				}
				close(start)
				running.Wait()
				close(results)
				durations := make([]time.Duration, 0, requests)
				failures := 0
				for result := range results {
					if result.err != nil {
						failures++
						t.Logf("request failed: %v", result.err)
					}
					durations = append(durations, result.duration)
				}
				require.Zero(t, failures, "do not report success-only percentiles")
				require.Len(t, durations, requests)
				slices.Sort(durations)
				// Nearest-rank quantiles over individual requests, not benchmark averages.
				t.Logf("requests=%d workers=%d accounts=1 rules=%d pool=8 p50=%s p95=%s p99=%s max=%s", requests, workers, ruleCount, durations[(requests*50+99)/100-1], durations[(requests*95+99)/100-1], durations[(requests*99+99)/100-1], durations[len(durations)-1])
				var decisions, reservations int
				require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM reserve_decisions").Scan(&decisions))
				require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_reservations").Scan(&reservations))
				require.Equal(t, requests, decisions)
				require.Equal(t, requests, reservations)
				current, held := readCounterDecimal(t, db, limit, "acct:"+request.Context.Accounts[0].ID.String(), testutil.FixedTime().Format("2006-01-02"))
				require.True(t, current.IsZero())
				require.Equal(t, "2025", held.String(), "200 exact debits of 10.125, with no lost updates")
			})
		}
	}
}
