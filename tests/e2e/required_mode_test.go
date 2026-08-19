// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestE2ERequired(t *testing.T) {
	t.Setenv("E2E_REQUIRED", "")
	if e2eRequired() {
		t.Fatal("empty E2E_REQUIRED must preserve the local opt-in skip mode")
	}

	t.Setenv("E2E_REQUIRED", "0")
	if e2eRequired() {
		t.Fatal("E2E_REQUIRED=0 must preserve the local opt-in skip mode")
	}

	t.Setenv("E2E_REQUIRED", "1")
	if !e2eRequired() {
		t.Fatal("E2E_REQUIRED=1 must enable the mandatory gate")
	}

	if got := ledgerE2EWorkerLimitFrom(""); got != 4 {
		t.Fatalf("default Ledger E2E worker limit = %d, want 4", got)
	}
	if got := ledgerE2EWorkerLimitFrom("2"); got != 2 {
		t.Fatalf("configured Ledger E2E worker limit = %d, want 2", got)
	}
	if got := ledgerE2EWorkerLimitFrom("invalid"); got != 4 {
		t.Fatalf("invalid Ledger E2E worker limit = %d, want safe default 4", got)
	}
}

func TestCheckRequiredStack(t *testing.T) {
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ready.Close)

	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(unavailable.Close)

	t.Run("both deploy surfaces ready", func(t *testing.T) {
		if err := checkRequiredStack(ready.Client(), ready.URL, ready.URL); err != nil {
			t.Fatalf("checkRequiredStack() error = %v", err)
		}
	})

	t.Run("ledger dependencies unavailable", func(t *testing.T) {
		err := checkRequiredStack(unavailable.Client(), unavailable.URL, ready.URL)
		if err == nil || !strings.Contains(err.Error(), "ledger") || !strings.Contains(err.Error(), "503") {
			t.Fatalf("checkRequiredStack() error = %v, want ledger readiness failure with status", err)
		}
	})

	t.Run("tracer dependencies unavailable", func(t *testing.T) {
		err := checkRequiredStack(unavailable.Client(), ready.URL, unavailable.URL)
		if err == nil || !strings.Contains(err.Error(), "tracer") || !strings.Contains(err.Error(), "503") {
			t.Fatalf("checkRequiredStack() error = %v, want tracer readiness failure with status", err)
		}
	})
}

func TestClassifyTracerWiringProbe(t *testing.T) {
	tests := []struct {
		name string
		resp response
		want tracerWiringProbeClass
	}{
		{
			name: "expected functional denial proves wiring",
			resp: response{status: http.StatusUnprocessableEntity, json: map[string]any{"code": "0177"}},
			want: tracerWiringFunctionalDenial,
		},
		{
			name: "successful transfer proves wiring absent",
			resp: response{status: http.StatusCreated},
			want: tracerWiringAbsent,
		},
		{
			name: "technical server failure is not a functional denial",
			resp: response{status: http.StatusInternalServerError, json: map[string]any{"code": "0001"}},
			want: tracerWiringTechnicalFailure,
		},
		{
			name: "wrong business error does not prove tracer wiring",
			resp: response{status: http.StatusUnprocessableEntity, json: map[string]any{"code": "0490"}},
			want: tracerWiringTechnicalFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyTracerWiringProbe(tt.resp); got != tt.want {
				t.Fatalf("classifyTracerWiringProbe() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMissingReservationIDsFailsRequiredMode(t *testing.T) {
	const child = "E2E_REQUIRED_MISSING_RESERVATION_CHILD"
	if os.Getenv(child) == "1" {
		t.Setenv("E2E_REQUIRED", "1")
		trlcRequireReservationIDs(t, response{
			body: []byte(`{"denied":false,"reservationIds":[]}`),
			json: map[string]any{"denied": false, "reservationIds": []any{}},
		})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMissingReservationIDsFailsRequiredMode$", "-test.v")
	cmd.Env = append(os.Environ(), child+"=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("required reservation capability accepted an empty reservation set\noutput: %s", output)
	}
	if !strings.Contains(string(output), "required reservation tuple idempotency capability produced no reservationIds") {
		t.Fatalf("required reservation failure was not actionable\noutput: %s", output)
	}
}

// TestRequiredStackLane is the non-vacuity sentinel consumed by the mandatory
// runner. It passes only after both services are ready and an over-limit
// transfer produces the tracer's exact functional-denial contract.
func TestRequiredStackLane(t *testing.T) {
	if !e2eRequired() {
		t.Skip("mandatory stack sentinel runs only with E2E_REQUIRED=1")
	}

	requireStack(t)
	requireTracer(t)
	requireTracerWired(t)
}
