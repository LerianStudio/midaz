// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"strings"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/mongotest"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
)

// mongoSpanNames lists the recorded mongodb.* spans. The field-encryption service opens
// its own service.protection.* spans while the model is built; they are not driver
// children and are left out.
func mongoSpanNames(recorder *tracetest.SpanRecorder) []string {
	var names []string

	for _, name := range spantest.SpanNames(recorder) {
		if strings.HasPrefix(name, "mongodb.") {
			names = append(names, name)
		}
	}

	return names
}

// Scenario: Holder do CRM mantém os flags de presença no span da operação. The insert
// runs against an unreachable server, so it fails after the presence flags are recorded:
// they ride the mongodb.create_holder span, and there is no .insert child. The success
// path is covered by the integration suite.
func TestHolderSpans_CreateKeepsPresenceFlagsOnTheOperationSpan(t *testing.T) {
	organizationID := uuid.NewString()
	db := mongotest.UnreachableDatabase(t)

	// The holder collection's indexes are created once per collection; mark them done so
	// Create reaches the insert instead of failing on index creation.
	indexKey := db.Name() + ":" + strings.ToLower("holders_"+organizationID)
	require.NoError(t, globalIndexTracker.ensureOnce(indexKey, func() error { return nil }))
	t.Cleanup(func() { globalIndexTracker.reset(indexKey) })

	holderID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	holder := &mmodel.Holder{
		ID:        &holderID,
		Type:      testutils.Ptr("NATURAL_PERSON"),
		Name:      testutils.Ptr("John Doe"),
		Document:  testutils.Ptr("12345678901"),
		CreatedAt: now,
		UpdatedAt: now,
	}

	ctx, recorder := spantest.RecordingContext()
	ctx = tmcore.ContextWithMB(ctx, db)

	repo := &MongoDBRepository{FieldEncryptor: setupTestFieldEncryptor(t)}

	_, err := repo.Create(ctx, organizationID, holder)
	require.Error(t, err)

	spantest.AssertNoSpan(t, recorder, "mongodb.create_holder.insert")
	assert.Equal(t, []string{"mongodb.create_holder"}, mongoSpanNames(recorder))

	span := spantest.FindSpan(t, recorder, "mongodb.create_holder")
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "Failed to insert holder")

	for _, kv := range []attribute.KeyValue{
		attribute.Bool("app.request.repository_input.has_metadata", false),
		attribute.Bool("app.request.repository_input.has_external_id", false),
		attribute.Bool("app.request.repository_input.has_contact", false),
		attribute.Bool("app.request.repository_input.has_addresses", false),
		attribute.Bool("app.request.repository_input.has_natural_person", false),
		attribute.Bool("app.request.repository_input.has_legal_person", false),
	} {
		assert.Contains(t, span.Attributes(), kv)
	}

	assert.Contains(t, span.Attributes(), attribute.String("app.request.organization_id", organizationID))
}

// The other holder operations are one span each, with the driver failure on it.
func TestHolderSpans_OneSpanPerOperation(t *testing.T) {
	t.Parallel()

	organizationID := uuid.NewString()
	id := uuid.New()

	tests := []struct {
		name string
		call func(ctx context.Context, repo *MongoDBRepository) error
	}{
		{"mongodb.find_holder", func(ctx context.Context, repo *MongoDBRepository) error {
			_, err := repo.Find(ctx, organizationID, id, false)
			return err
		}},
		{"mongodb.find_all_holders", func(ctx context.Context, repo *MongoDBRepository) error {
			_, err := repo.FindAll(ctx, organizationID, http.QueryHeader{Limit: 10, Page: 1}, false)
			return err
		}},
		{"mongodb.update_holder", func(ctx context.Context, repo *MongoDBRepository) error {
			_, err := repo.Update(ctx, organizationID, id, &mmodel.Holder{ID: &id, Name: testutils.Ptr("Jane")}, nil)
			return err
		}},
		{"mongodb.delete_holder", func(ctx context.Context, repo *MongoDBRepository) error {
			return repo.Delete(ctx, organizationID, id, true)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, recorder := spantest.RecordingContext()
			ctx = tmcore.ContextWithMB(ctx, mongotest.UnreachableDatabase(t))

			repo := &MongoDBRepository{FieldEncryptor: setupTestFieldEncryptor(t)}
			require.Error(t, tt.call(ctx, repo))

			assert.Equal(t, []string{tt.name}, mongoSpanNames(recorder), "one mongodb span per operation, no driver child")
			assert.Equal(t, codes.Error, spantest.FindSpan(t, recorder, tt.name).Status().Code)
		})
	}
}
