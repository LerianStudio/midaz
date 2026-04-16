// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package dbpool

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePoolBudget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		maxConnections int
		ratio          float64
		pools          []PoolBudget
		wantErr        bool
	}{
		{
			name:           "within budget",
			maxConnections: 1000,
			ratio:          0.8,
			pools: []PoolBudget{
				{Name: "ledger", MaxConns: 100, ExpectedInstances: 4},
				{Name: "consumer", MaxConns: 100, ExpectedInstances: 1},
			},
			wantErr: false,
		},
		{
			name:           "exceeds budget",
			maxConnections: 200,
			ratio:          0.8, // budget = 160
			pools: []PoolBudget{
				{Name: "ledger", MaxConns: 100, ExpectedInstances: 2}, // 200
			},
			wantErr: true,
		},
		{
			name:           "default ratio when ratio<=0",
			maxConnections: 1000,
			ratio:          0,
			pools: []PoolBudget{
				{Name: "a", MaxConns: 900, ExpectedInstances: 1}, // 900 > 800
			},
			wantErr: true,
		},
		{
			name:           "zero max_connections skips validation",
			maxConnections: 0,
			ratio:          0.8,
			pools: []PoolBudget{
				{Name: "oversized", MaxConns: 1_000_000, ExpectedInstances: 10},
			},
			wantErr: false,
		},
		{
			name:           "non-positive instances skip",
			maxConnections: 100,
			ratio:          0.8,
			pools: []PoolBudget{
				{Name: "a", MaxConns: 100, ExpectedInstances: 0},
				{Name: "b", MaxConns: 0, ExpectedInstances: 10},
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePoolBudget(tc.maxConnections, tc.ratio, tc.pools)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}

				if !errors.Is(err, ErrPoolBudgetExceeded) {
					t.Fatalf("expected ErrPoolBudgetExceeded, got %v", err)
				}

				if !strings.Contains(err.Error(), "max_connections=") {
					t.Fatalf("error missing diagnostic context: %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidatePoolBudget_BootstrapRejectsWhenPoolBudgetExceedsMaxConnections(t *testing.T) {
	t.Parallel()

	// Named to match the acceptance test case in the D7 task description.
	// Simulates a realistic over-allocation: 4 ledger instances each with
	// MaxConns=150 against a small PG max_connections=500.
	pools := []PoolBudget{
		{Name: "ledger", MaxConns: 150, ExpectedInstances: 4},
		{Name: "consumer", MaxConns: 150, ExpectedInstances: 1},
	}

	err := ValidatePoolBudget(500, 0.8, pools)
	if err == nil {
		t.Fatal("expected ErrPoolBudgetExceeded, got nil")
	}

	if !errors.Is(err, ErrPoolBudgetExceeded) {
		t.Fatalf("expected ErrPoolBudgetExceeded, got %v", err)
	}
}
