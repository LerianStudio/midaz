// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee_debt

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"

	mmongoDB "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/mongotest"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
)

// Each fee debt repository read is one span with the driver failure on it, and no
// driver child. The driver is driven against an unreachable server; the success path is
// covered by the integration suite.
func TestFeeDebtSpans_OneSpanPerOperation(t *testing.T) {
	t.Parallel()

	org, ledger := uuid.New(), uuid.New()

	tests := []struct {
		name string
		call func(ctx context.Context, repo *Repository) error
	}{
		{"repository.fee_debt.find_by_id", func(ctx context.Context, repo *Repository) error {
			_, err := repo.FindByID(ctx, org, ledger, uuid.NewString())
			return err
		}},
		{"repository.fee_debt.settled", func(ctx context.Context, repo *Repository) error {
			_, err := repo.Settled(ctx, org, ledger, []string{uuid.NewString()})
			return err
		}},
		{"repository.fee_debt.find_all", func(ctx context.Context, repo *Repository) error {
			_, _, err := repo.FindAll(ctx, org, ledger, ListQuery{Limit: 10})
			return err
		}},
		{"repository.fee_debt.open_total", func(ctx context.Context, repo *Repository) error {
			_, err := repo.OpenTotal(ctx, org, ledger, "@debtor#default")
			return err
		}},
		{"repository.fee_debt.has_open_creditor", func(ctx context.Context, repo *Repository) error {
			_, err := repo.HasOpenCreditor(ctx, org, ledger, []string{"@creditor#default"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, recorder := spantest.RecordingContext()

			repo := &Repository{connection: &mmongoDB.MongoConnection{DB: mongotest.UnreachableClient(t)}, database: "mongotest"}
			require.Error(t, tt.call(ctx, repo))

			assert.Equal(t, []string{tt.name}, spantest.SpanNames(recorder), "one span per operation, no driver child")
			assert.Equal(t, codes.Error, spantest.FindSpan(t, recorder, tt.name).Status().Code)
		})
	}
}
