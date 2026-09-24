// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

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
