// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestClassifyAssignsEveryIntegrationTestExactlyOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pkg       string
		test      string
		wantShard string
		wantMode  string
	}{
		{
			pkg:       "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account",
			test:      "TestIntegration_AccountRepository_Create",
			wantShard: shardLedgerPostgres,
			wantMode:  modeParallel,
		},
		{
			pkg:       "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding",
			test:      "TestIntegration_MetadataRepository_Create",
			wantShard: shardLedgerMongoCRM,
			wantMode:  modeParallel,
		},
		{
			pkg:       "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq",
			test:      "TestIntegration_Producer_Publish",
			wantShard: shardAsyncBroker,
			wantMode:  modeParallel,
		},
		{
			pkg:       "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers",
			test:      "TestIntegration_Worker_ProcessesEvent",
			wantShard: shardTracer,
			wantMode:  modeParallel,
		},
		{
			pkg:       "github.com/LerianStudio/midaz/v4/tests/integration",
			test:      "TestIntegration_LedgerMigrations_UpDownIdempotency",
			wantShard: shardLifecycleMigration,
			wantMode:  modeSerial,
		},
	}

	for _, tt := range tests {
		t.Run(tt.test, func(t *testing.T) {
			t.Parallel()

			assignment, err := classify(testRecord{Package: tt.pkg, Test: tt.test})
			if err != nil {
				t.Fatalf("classify() error = %v", err)
			}
			if assignment.Shard != tt.wantShard {
				t.Errorf("shard = %q, want %q", assignment.Shard, tt.wantShard)
			}
			if assignment.Mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", assignment.Mode, tt.wantMode)
			}
		})
	}
}

func TestClassifyKeepsSerialExclusionsOutOfParallelWork(t *testing.T) {
	t.Parallel()

	tests := []testRecord{
		{
			Package: "github.com/LerianStudio/midaz/v4/components/tracer/tests/integration",
			Test:    "TestValidation_CompletePayload",
		},
		{
			Package: "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction",
			Test:    "TestIntegration_Chaos_Transaction_NetworkPartition",
		},
		{
			Package: "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in",
			Test:    "TestIntegration_TransactionV2Lifecycle_StateAndIDErrors",
		},
		{
			Package: "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres",
			Test:    "TestIntegration_Migration000021_DownRefusesLiveReceipt",
		},
		{
			Package: "github.com/LerianStudio/midaz/v4/components/ledger/internal/bootstrap",
			Test:    "TestIntegration_Service_Run_StartsAllServers",
		},
		{
			Package: "github.com/LerianStudio/midaz/v4/components/tracer/internal/bootstrap",
			Test:    "TestStreamingSmoke",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Test, func(t *testing.T) {
			t.Parallel()

			assignment, err := classify(tt)
			if err != nil {
				t.Fatalf("classify() error = %v", err)
			}
			if assignment.Mode != modeSerial {
				t.Fatalf("mode = %q, want %q", assignment.Mode, modeSerial)
			}
			if assignment.Shard != shardLifecycleMigration && tt.Package != tracerJourneyPackage {
				t.Fatalf("serial exclusion shard = %q, want %q", assignment.Shard, shardLifecycleMigration)
			}
		})
	}
}

func TestReadInventoryRejectsMalformedDuplicateAndEmptyInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "zero integration tests"},
		{name: "malformed", input: "only-one-column\n", want: "want package and test"},
		{
			name:  "duplicate",
			input: "example.test/pkg TestOne\nexample.test/pkg TestOne\n",
			want:  "duplicate integration test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := readInventory(strings.NewReader(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("readInventory() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestVerifyAssignmentsRejectsOverlapOmissionAndUnknownShard(t *testing.T) {
	t.Parallel()

	inventory := []testRecord{
		{Package: "example.test/a", Test: "TestOne"},
		{Package: "example.test/b", Test: "TestTwo"},
	}

	tests := []struct {
		name        string
		assignments []assignment
		want        string
	}{
		{
			name: "omission",
			assignments: []assignment{
				{testRecord: inventory[0], Shard: shardLedgerPostgres, Mode: modeParallel},
			},
			want: "omitted",
		},
		{
			name: "overlap",
			assignments: []assignment{
				{testRecord: inventory[0], Shard: shardLedgerPostgres, Mode: modeParallel},
				{testRecord: inventory[0], Shard: shardAsyncBroker, Mode: modeParallel},
				{testRecord: inventory[1], Shard: shardTracer, Mode: modeParallel},
			},
			want: "assigned 2 times",
		},
		{
			name: "unknown shard",
			assignments: []assignment{
				{testRecord: inventory[0], Shard: "invented", Mode: modeParallel},
				{testRecord: inventory[1], Shard: shardTracer, Mode: modeParallel},
			},
			want: "unknown shard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := verifyAssignments(inventory, tt.assignments)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("verifyAssignments() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestVerifyEventCoverageRequiresEveryAndOnlySelectedTopLevelTest(t *testing.T) {
	t.Parallel()

	expected := []string{"TestOne", "TestTwo"}
	events := strings.Join([]string{
		`{"Action":"run","Package":"example.test/pkg","Test":"TestOne"}`,
		`{"Action":"run","Package":"example.test/pkg","Test":"TestOne/child"}`,
		`{"Action":"pass","Package":"example.test/pkg","Test":"TestOne/child"}`,
		`{"Action":"pass","Package":"example.test/pkg","Test":"TestOne"}`,
		`{"Action":"run","Package":"example.test/pkg","Test":"TestTwo"}`,
		`{"Action":"skip","Package":"example.test/pkg","Test":"TestTwo"}`,
	}, "\n")

	if err := verifyEventCoverage("example.test/pkg", expected, strings.NewReader(events)); err != nil {
		t.Fatalf("verifyEventCoverage() error = %v", err)
	}
}

func TestVerifyEventCoverageFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events string
		want   string
	}{
		{
			name:   "missing",
			events: `{"Action":"run","Package":"example.test/pkg","Test":"TestOne"}`,
			want:   "did not start selected test TestTwo",
		},
		{
			name: "unexpected",
			events: strings.Join([]string{
				`{"Action":"run","Package":"example.test/pkg","Test":"TestOne"}`,
				`{"Action":"run","Package":"example.test/pkg","Test":"TestTwo"}`,
				`{"Action":"run","Package":"example.test/pkg","Test":"TestThree"}`,
			}, "\n"),
			want: "started unselected test TestThree",
		},
		{
			name: "duplicate",
			events: strings.Join([]string{
				`{"Action":"run","Package":"example.test/pkg","Test":"TestOne"}`,
				`{"Action":"run","Package":"example.test/pkg","Test":"TestOne"}`,
				`{"Action":"run","Package":"example.test/pkg","Test":"TestTwo"}`,
			}, "\n"),
			want: "started selected test TestOne 2 times",
		},
		{name: "invalid json", events: `{`, want: "decode Go test event"},
		{name: "empty", events: "", want: "contained zero events"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := verifyEventCoverage(
				"example.test/pkg",
				[]string{"TestOne", "TestTwo"},
				bytes.NewBufferString(tt.events),
			)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("verifyEventCoverage() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
