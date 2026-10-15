// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/codes"

	mmongoDB "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/mongotest"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
)

// Each billing package repository operation is one span with the driver failure on it,
// and no driver child. The driver is driven against an unreachable server; the success
// path is covered by the integration suite.
func TestBillingPackageSpans_OneSpanPerOperation(t *testing.T) {
	t.Parallel()

	id, org, ledger := uuid.NewString(), uuid.NewString(), uuid.NewString()

	tests := []struct {
		name string
		call func(ctx context.Context, repo *BillingPackageMongoDBRepository) error
	}{
		{"repository.billing_package.find_by_id", func(ctx context.Context, repo *BillingPackageMongoDBRepository) error {
			_, err := repo.FindByID(ctx, id, org, ledger)
			return err
		}},
		{"repository.billing_package.find_all", func(ctx context.Context, repo *BillingPackageMongoDBRepository) error {
			_, _, err := repo.FindAll(ctx, org, ledger, "", 10, 1)
			return err
		}},
		{"repository.billing_package.find_matching_packages", func(ctx context.Context, repo *BillingPackageMongoDBRepository) error {
			_, err := repo.FindMatchingPackages(ctx, org, ledger, uuid.NewString())
			return err
		}},
		{"repository.billing_package.find_active_by_type", func(ctx context.Context, repo *BillingPackageMongoDBRepository) error {
			_, err := repo.FindActiveByType(ctx, org, ledger, "volume")
			return err
		}},
		{"repository.billing_package.find_not_deleted_by_ledger", func(ctx context.Context, repo *BillingPackageMongoDBRepository) error {
			_, err := repo.FindNotDeletedByLedger(ctx, org, ledger)
			return err
		}},
		{"repository.billing_package.delete", func(ctx context.Context, repo *BillingPackageMongoDBRepository) error {
			return repo.SoftDelete(ctx, id, org, ledger)
		}},
		{"repository.billing_package.update", func(ctx context.Context, repo *BillingPackageMongoDBRepository) error {
			_, err := repo.Update(ctx, id, org, ledger, &bson.M{"description": "d"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, recorder := spantest.RecordingContext()
			ctx = tmcore.ContextWithMB(ctx, mongotest.UnreachableDatabase(t))

			repo := &BillingPackageMongoDBRepository{connection: &mmongoDB.MongoConnection{RequireTenant: true}}
			require.Error(t, tt.call(ctx, repo))

			assert.Equal(t, []string{tt.name}, spantest.SpanNames(recorder), "one span per operation, no driver child")
			assert.Equal(t, codes.Error, spantest.FindSpan(t, recorder, tt.name).Status().Code)
		})
	}
}
