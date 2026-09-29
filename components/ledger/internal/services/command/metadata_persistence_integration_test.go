//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"

	onboardingmongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	txmongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

// seededMetadataTime is the created_at/updated_at of every seeded document, so
// an unchanged updated_at proves no write touched the document.
var seededMetadataTime = time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

type metadataPersistenceEnv struct {
	db *mongo.Database
	uc *UseCase
}

func setupMetadataPersistenceEnv(t *testing.T) *metadataPersistenceEnv {
	t.Helper()

	container := mongotestutil.SetupReusableContainer(t)
	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)

	return &metadataPersistenceEnv{
		db: container.Database,
		uc: &UseCase{
			OnboardingMetadataRepo:  onboardingmongodb.NewMetadataMongoDBRepository(conn),
			TransactionMetadataRepo: txmongodb.NewMetadataMongoDBRepository(conn),
		},
	}
}

func (env *metadataPersistenceEnv) count(t *testing.T, entityName, entityID string) int64 {
	t.Helper()

	return mongotestutil.CountDocuments(t, env.db, strings.ToLower(entityName), bson.M{"entity_id": entityID})
}

func (env *metadataPersistenceEnv) seed(t *testing.T, entityName, entityID string, data map[string]any) {
	t.Helper()

	mongotestutil.InsertMetadata(t, env.db, strings.ToLower(entityName), mongotestutil.MetadataFixture{
		EntityID:   entityID,
		EntityName: entityName,
		Data:       data,
		CreatedAt:  seededMetadataTime,
		UpdatedAt:  seededMetadataTime,
	})
}

func (env *metadataPersistenceEnv) read(t *testing.T, entityName, entityID string) mongotestutil.MetadataFixture {
	t.Helper()

	var doc mongotestutil.MetadataFixture

	err := env.db.Collection(strings.ToLower(entityName)).
		FindOne(context.Background(), bson.M{"entity_id": entityID}).
		Decode(&doc)
	require.NoError(t, err, "metadata document must exist")

	return doc
}

// =============================================================================
// ONBOARDING: CREATE
// =============================================================================

func TestIntegration_MetadataPersistence_OnboardingCreate(t *testing.T) {
	env := setupMetadataPersistenceEnv(t)
	ctx := context.Background()
	entity := constant.EntityLedger

	t.Run("create-com-metadata-null: nil persists no document", func(t *testing.T) {
		id := uuid.NewString()

		result, err := env.uc.CreateOnboardingMetadata(ctx, entity, id, nil)

		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, int64(0), env.count(t, entity, id))
	})

	t.Run("create-sem-chave-metadata/create-com-metadata-vazio: empty map persists no document", func(t *testing.T) {
		id := uuid.NewString()

		result, err := env.uc.CreateOnboardingMetadata(ctx, entity, id, map[string]any{})

		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, int64(0), env.count(t, entity, id))
	})

	t.Run("create-com-metadata-preenchido: non-empty map persists one document", func(t *testing.T) {
		id := uuid.NewString()

		result, err := env.uc.CreateOnboardingMetadata(ctx, entity, id, map[string]any{"k": "v"})

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"k": "v"}, result)
		assert.Equal(t, int64(1), env.count(t, entity, id))
		assert.Equal(t, map[string]any{"k": "v"}, env.read(t, entity, id).Data)
	})
}

// =============================================================================
// ONBOARDING: PATCH
// =============================================================================

func TestIntegration_MetadataPersistence_OnboardingUpdate(t *testing.T) {
	env := setupMetadataPersistenceEnv(t)
	ctx := context.Background()
	entity := constant.EntityLedger

	t.Run("patch-sem-chave-entidade-sem-doc: empty map creates no document", func(t *testing.T) {
		id := uuid.NewString()

		result, err := env.uc.UpdateOnboardingMetadata(ctx, entity, id, map[string]any{})

		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, int64(0), env.count(t, entity, id))
	})

	t.Run("patch-sem-chave-entidade-com-doc/patch-metadata-vazio-explicito: empty map leaves the document untouched", func(t *testing.T) {
		id := uuid.NewString()
		env.seed(t, entity, id, map[string]any{"k": "v"})
		before := env.read(t, entity, id)

		result, err := env.uc.UpdateOnboardingMetadata(ctx, entity, id, map[string]any{})

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"k": "v"}, result)

		after := env.read(t, entity, id)
		assert.Equal(t, before.Data, after.Data)
		assert.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "updated_at must not change")
		assert.Equal(t, int64(1), env.count(t, entity, id))
	})

	t.Run("patch-merge-cria-doc-quando-ausente: non-empty map creates the document", func(t *testing.T) {
		id := uuid.NewString()

		result, err := env.uc.UpdateOnboardingMetadata(ctx, entity, id, map[string]any{"k": "v"})

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"k": "v"}, result)
		assert.Equal(t, int64(1), env.count(t, entity, id))
		assert.Equal(t, map[string]any{"k": "v"}, env.read(t, entity, id).Data)
	})

	t.Run("patch-merge-adiciona-chave: non-empty map merges into the document", func(t *testing.T) {
		id := uuid.NewString()
		env.seed(t, entity, id, map[string]any{"k": "v"})

		result, err := env.uc.UpdateOnboardingMetadata(ctx, entity, id, map[string]any{"k2": "v2"})

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"k": "v", "k2": "v2"}, result)
		assert.Equal(t, map[string]any{"k": "v", "k2": "v2"}, env.read(t, entity, id).Data)
	})

	t.Run("patch-merge-remove-chave-com-null: null value removes the key", func(t *testing.T) {
		id := uuid.NewString()
		env.seed(t, entity, id, map[string]any{"k": "v", "k2": "v2"})

		result, err := env.uc.UpdateOnboardingMetadata(ctx, entity, id, map[string]any{"k2": nil})

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"k": "v"}, result)
		assert.Equal(t, map[string]any{"k": "v"}, env.read(t, entity, id).Data)
		assert.Equal(t, int64(1), env.count(t, entity, id))
	})

	t.Run("patch-null-entidade-com-doc: nil clears the document", func(t *testing.T) {
		id := uuid.NewString()
		env.seed(t, entity, id, map[string]any{"k": "v"})

		result, err := env.uc.UpdateOnboardingMetadata(ctx, entity, id, nil)

		require.NoError(t, err)
		assert.Empty(t, result)
		assert.Empty(t, env.read(t, entity, id).Data)
		assert.Equal(t, int64(1), env.count(t, entity, id))
	})

	t.Run("patch-null-entidade-sem-doc: nil creates no document", func(t *testing.T) {
		id := uuid.NewString()

		result, err := env.uc.UpdateOnboardingMetadata(ctx, entity, id, nil)

		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, int64(0), env.count(t, entity, id))
	})
}

// =============================================================================
// ONBOARDING: PRE-EXISTING EMPTY DOCUMENTS
// =============================================================================

func TestIntegration_MetadataPersistence_PreexistingEmptyDocument(t *testing.T) {
	env := setupMetadataPersistenceEnv(t)
	ctx := context.Background()
	entity := constant.EntityAccount

	t.Run("patch-merge-sobre-doc-vazio-preexistente: merge updates the same document", func(t *testing.T) {
		id := uuid.NewString()
		env.seed(t, entity, id, map[string]any{})

		result, err := env.uc.UpdateOnboardingMetadata(ctx, entity, id, map[string]any{"k": "v"})

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"k": "v"}, result)
		assert.Equal(t, int64(1), env.count(t, entity, id))
		assert.Equal(t, map[string]any{"k": "v"}, env.read(t, entity, id).Data)
	})

	t.Run("get-doc-vazio-e-doc-ausente-equivalentes: empty document reads as no metadata", func(t *testing.T) {
		emptyID := uuid.NewString()
		absentID := uuid.NewString()
		env.seed(t, entity, emptyID, map[string]any{})

		found, err := env.uc.OnboardingMetadataRepo.FindByEntityIDs(ctx, entity, []string{emptyID, absentID})

		require.NoError(t, err)
		require.Len(t, found, 1, "only the empty document exists")
		assert.Equal(t, emptyID, found[0].EntityID)
		assert.Empty(t, found[0].Data)
	})

	t.Run("list-filtro-metadata-ignora-vazios: metadata filter skips empty and absent documents", func(t *testing.T) {
		filledID := uuid.NewString()
		emptyID := uuid.NewString()
		marker := uuid.NewString()

		env.seed(t, entity, filledID, map[string]any{"k": marker})
		env.seed(t, entity, emptyID, map[string]any{})

		metadataFilter := bson.M{"metadata.k": marker}

		results, err := env.uc.OnboardingMetadataRepo.FindList(ctx, entity, http.QueryHeader{
			Metadata:    &metadataFilter,
			UseMetadata: true,
			Limit:       10,
			Page:        1,
		})

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, filledID, results[0].EntityID)
	})
}

// =============================================================================
// TRANSACTION MODULE
// =============================================================================

func TestIntegration_MetadataPersistence_TransactionUpdate(t *testing.T) {
	env := setupMetadataPersistenceEnv(t)
	ctx := context.Background()
	entity := constant.EntityTransactionRoute

	t.Run("patch-sem-chave-entidade-sem-doc: empty map creates no document", func(t *testing.T) {
		id := uuid.NewString()

		result, err := env.uc.UpdateTransactionMetadata(ctx, entity, id, map[string]any{})

		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, int64(0), env.count(t, entity, id))
	})

	t.Run("patch-sem-chave-entidade-com-doc/patch-metadata-vazio-explicito: empty map leaves the document untouched", func(t *testing.T) {
		id := uuid.NewString()
		env.seed(t, entity, id, map[string]any{"k": "v"})
		before := env.read(t, entity, id)

		result, err := env.uc.UpdateTransactionMetadata(ctx, entity, id, map[string]any{})

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"k": "v"}, result)

		after := env.read(t, entity, id)
		assert.Equal(t, before.Data, after.Data)
		assert.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "updated_at must not change")
	})

	t.Run("patch-null-entidade-com-doc: nil clears the document", func(t *testing.T) {
		id := uuid.NewString()
		env.seed(t, entity, id, map[string]any{"k": "v"})

		result, err := env.uc.UpdateTransactionMetadata(ctx, entity, id, nil)

		require.NoError(t, err)
		assert.Empty(t, result)
		assert.Empty(t, env.read(t, entity, id).Data)
		assert.Equal(t, int64(1), env.count(t, entity, id))
	})

	t.Run("patch-merge-cria-doc-quando-ausente: non-empty map creates the document", func(t *testing.T) {
		id := uuid.NewString()

		result, err := env.uc.UpdateTransactionMetadata(ctx, entity, id, map[string]any{"k": "v"})

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"k": "v"}, result)
		assert.Equal(t, int64(1), env.count(t, entity, id))
		assert.Equal(t, map[string]any{"k": "v"}, env.read(t, entity, id).Data)
	})

	t.Run("patch-merge-adiciona-chave: non-empty map merges into the document", func(t *testing.T) {
		id := uuid.NewString()
		env.seed(t, entity, id, map[string]any{"k": "v"})

		result, err := env.uc.UpdateTransactionMetadata(ctx, entity, id, map[string]any{"k2": "v2"})

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"k": "v", "k2": "v2"}, result)
		assert.Equal(t, map[string]any{"k": "v", "k2": "v2"}, env.read(t, entity, id).Data)
	})

	t.Run("patch-merge-remove-chave-com-null: null value removes the key", func(t *testing.T) {
		id := uuid.NewString()
		env.seed(t, entity, id, map[string]any{"k": "v", "k2": "v2"})

		result, err := env.uc.UpdateTransactionMetadata(ctx, entity, id, map[string]any{"k2": nil})

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"k": "v"}, result)
		assert.Equal(t, map[string]any{"k": "v"}, env.read(t, entity, id).Data)
		assert.Equal(t, int64(1), env.count(t, entity, id))
	})

	t.Run("patch-null-entidade-sem-doc: nil creates no document", func(t *testing.T) {
		id := uuid.NewString()

		result, err := env.uc.UpdateTransactionMetadata(ctx, entity, id, nil)

		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, int64(0), env.count(t, entity, id))
	})
}

// TestIntegration_MetadataPersistence_TransactionRouteCreate runs
// CreateTransactionRoute with mocked PostgreSQL repositories and the real
// MongoDB metadata repository, so the create guard is checked against the
// collection itself.
func TestIntegration_MetadataPersistence_TransactionRouteCreate(t *testing.T) {
	env := setupMetadataPersistenceEnv(t)

	tests := []struct {
		name      string
		metadata  map[string]any
		wantCount int64
	}{
		{name: "create-com-metadata-null: nil persists no document", metadata: nil, wantCount: 0},
		{name: "create-com-metadata-vazio: empty map persists no document", metadata: map[string]any{}, wantCount: 0},
		{name: "create-com-metadata-preenchido: non-empty map persists one document", metadata: map[string]any{"k": "v"}, wantCount: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			organizationID := uuid.New()
			ledgerID := uuid.New()
			routeID := uuid.New()
			sourceID := uuid.New()
			destinationID := uuid.New()

			operationRouteRepo := operationroute.NewMockRepository(ctrl)
			transactionRouteRepo := transactionroute.NewMockRepository(ctrl)

			operationRouteRepo.EXPECT().
				FindByIDs(gomock.Any(), organizationID, gomock.Any()).
				Return([]*mmodel.OperationRoute{
					{ID: sourceID, OperationType: "source"},
					{ID: destinationID, OperationType: "destination"},
				}, nil).
				Times(1)

			transactionRouteRepo.EXPECT().
				Create(gomock.Any(), organizationID, &ledgerID, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, tr *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error) {
					tr.ID = routeID

					return tr, nil
				}).
				Times(1)

			uc := &UseCase{
				OperationRouteRepo:      operationRouteRepo,
				TransactionRouteRepo:    transactionRouteRepo,
				TransactionMetadataRepo: env.uc.TransactionMetadataRepo,
			}

			result, err := uc.CreateTransactionRoute(context.Background(), organizationID, &ledgerID, &mmodel.CreateTransactionRouteInput{
				Title:           "Metadata Persistence Route",
				OperationRoutes: []uuid.UUID{sourceID, destinationID},
				Metadata:        tt.metadata,
			})

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantCount, env.count(t, constant.EntityTransactionRoute, routeID.String()))
		})
	}
}
