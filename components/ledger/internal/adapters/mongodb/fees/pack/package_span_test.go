// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"testing"
	"time"

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

// Each package repository operation is one span with the driver failure on it, and no
// .find/.find_one/.insert/... driver child. The driver is driven against an unreachable
// server; the success path is covered by the integration suite.
func TestPackageSpans_OneSpanPerOperation(t *testing.T) {
	t.Parallel()

	id, org, ledger := uuid.New(), uuid.New(), uuid.New()
	updatedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name string
		call func(ctx context.Context, repo *PackageMongoDBRepository) error
	}{
		{"repository.package.find_by_entity", func(ctx context.Context, repo *PackageMongoDBRepository) error {
			_, err := repo.FindByID(ctx, id, org, ledger)
			return err
		}},
		{"repository.package.find_by_org_and_ledger", func(ctx context.Context, repo *PackageMongoDBRepository) error {
			_, err := repo.FindByOrganizationIDAndLedgerID(ctx, org, ledger)
			return err
		}},
		{"repository.package.find_not_deleted_by_org_and_ledger", func(ctx context.Context, repo *PackageMongoDBRepository) error {
			_, err := repo.FindNotDeletedByOrganizationIDAndLedgerID(ctx, org, ledger)
			return err
		}},
		{"repository.package.find_fees_by_package_id", func(ctx context.Context, repo *PackageMongoDBRepository) error {
			_, err := repo.FindFeesAndAmountDataByPackageID(ctx, org, id)
			return err
		}},
		{"repository.package.delete", func(ctx context.Context, repo *PackageMongoDBRepository) error {
			return repo.SoftDelete(ctx, id, org, ledger)
		}},
		{"repository.package.update", func(ctx context.Context, repo *PackageMongoDBRepository) error {
			_, err := repo.Update(ctx, id, org, ledger, updatedAt, &bson.M{"description": "d"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, recorder := spantest.RecordingContext()
			ctx = tmcore.ContextWithMB(ctx, mongotest.UnreachableDatabase(t))

			repo := &PackageMongoDBRepository{connection: &mmongoDB.MongoConnection{RequireTenant: true}}
			require.Error(t, tt.call(ctx, repo))

			assert.Equal(t, []string{tt.name}, spantest.SpanNames(recorder), "one span per operation, no driver child")
			assert.Equal(t, codes.Error, spantest.FindSpan(t, recorder, tt.name).Status().Code)
		})
	}
}
