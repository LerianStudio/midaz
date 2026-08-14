//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// createServiceWithRealRepo wires a CRM UseCase over a real MongoDB-backed holder repository.
func createServiceWithRealRepo(t *testing.T, container *mongotestutil.ContainerResult) *UseCase {
	t.Helper()

	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)
	crypto := testutils.SetupCrypto(t)
	resolver := encryption.NewProtectionStateResolver(nil, encryption.NewProtectionMetrics(nil))
	svc := encryption.NewEncryptionService(resolver, nil, nil, crypto, encryption.NewProtectionMetrics(nil))
	fe := encryption.NewFieldEncryptorAdapter(svc)

	repo, err := holder.NewMongoDBRepository(conn, fe)
	require.NoError(t, err)

	return &UseCase{HolderRepo: repo}
}

func TestIntegration_CreateHolderWithID_Idempotent(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	uc := createServiceWithRealRepo(t, container)
	ctx := context.Background()

	organizationID := "org-selfholder-" + uuid.New().String()[:8]
	holderID := uuid.New()
	holderType := "LEGAL_PERSON"

	input := &mmodel.CreateHolderInput{
		Type:     &holderType,
		Name:     "Self Holder Corp",
		Document: "12345678000199",
	}

	// Act - provision twice with the same deterministic id.
	first, err := uc.CreateHolderWithID(ctx, organizationID, holderID, input)
	require.NoError(t, err, "first provisioning should succeed")
	require.NotNil(t, first)
	assert.Equal(t, holderID, *first.ID, "id must be the caller-supplied one")

	second, err := uc.CreateHolderWithID(ctx, organizationID, holderID, input)

	// Assert - second call is idempotent success returning the existing holder.
	require.NoError(t, err, "second provisioning with same id must be a no-op success")
	require.NotNil(t, second)
	assert.Equal(t, holderID, *second.ID)

	// Exactly one document exists for the id.
	collName := strings.ToLower("holders_" + organizationID)
	count := mongotestutil.CountDocuments(t, container.Database, collName, bson.M{"_id": holderID})
	assert.Equal(t, int64(1), count, "exactly one holder document should exist after two provisioning calls")
}
