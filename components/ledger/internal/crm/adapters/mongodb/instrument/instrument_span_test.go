// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package instrument

import (
	"context"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/mongotest"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
)

// Each instrument repository operation is one mongodb span with the driver failure on it,
// and no driver child. The driver is driven against an unreachable server; the success
// path is covered by the integration suite. Field-encryption service spans are not driver
// children and are left out of the comparison.
func TestInstrumentSpans_OneSpanPerOperation(t *testing.T) {
	t.Parallel()

	organizationID := uuid.NewString()
	holderID, id := uuid.New(), uuid.New()

	tests := []struct {
		name string
		call func(ctx context.Context, repo *MongoDBRepository) error
	}{
		{"mongodb.find_instrument", func(ctx context.Context, repo *MongoDBRepository) error {
			_, err := repo.Find(ctx, organizationID, holderID, id, false)
			return err
		}},
		{"mongodb.find_all_instruments", func(ctx context.Context, repo *MongoDBRepository) error {
			_, err := repo.FindAll(ctx, organizationID, holderID, http.QueryHeader{Limit: 10, Page: 1}, false)
			return err
		}},
		{"mongodb.count_instruments", func(ctx context.Context, repo *MongoDBRepository) error {
			_, err := repo.Count(ctx, organizationID, holderID)
			return err
		}},
		{"mongodb.find_live_instrument_refs_by_account", func(ctx context.Context, repo *MongoDBRepository) error {
			_, err := repo.FindLiveRefsByAccount(ctx, organizationID, uuid.New(), uuid.New())
			return err
		}},
		{"mongodb.update_instrument", func(ctx context.Context, repo *MongoDBRepository) error {
			_, err := repo.Update(ctx, organizationID, holderID, id, &mmodel.Instrument{ID: &id, Type: testutils.Ptr("NATURAL_PERSON")}, nil)
			return err
		}},
		{"mongodb.delete_instrument", func(ctx context.Context, repo *MongoDBRepository) error {
			return repo.Delete(ctx, organizationID, holderID, id, true)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, recorder := spantest.RecordingContext()
			ctx = tmcore.ContextWithMB(ctx, mongotest.UnreachableDatabase(t))

			repo := &MongoDBRepository{FieldEncryptor: setupTestFieldEncryptor(t)}
			require.Error(t, tt.call(ctx, repo))

			var mongoSpans []string

			for _, name := range spantest.SpanNames(recorder) {
				if strings.HasPrefix(name, "mongodb.") {
					mongoSpans = append(mongoSpans, name)
				}
			}

			assert.Equal(t, []string{tt.name}, mongoSpans, "one mongodb span per operation, no driver child")
			assert.Equal(t, codes.Error, spantest.FindSpan(t, recorder, tt.name).Status().Code)
		})
	}
}
