//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	crmholder "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	crmservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

func TestIntegration_CreateHolder_IdempotencySlotHoldsNoPersonalData(t *testing.T) {
	mongoContainer := mongotestutil.SetupReusableContainer(t)
	redisContainer := redistestutil.SetupReusableContainer(t)

	fieldEncryptor := newTestFieldEncryptor(t)

	holderRepo, err := crmholder.NewMongoDBRepository(mongotestutil.CreateConnection(t, mongoContainer.URI, mongoContainer.DBName), fieldEncryptor)
	require.NoError(t, err)

	redisRepo, err := txRedis.NewConsumerRedis(redistestutil.CreateConnectionWithDB(t, redisContainer.Addr, redisContainer.DB))
	require.NoError(t, err)

	handler := &HolderHandler{Service: &crmservices.UseCase{
		HolderRepo:  holderRepo,
		Idempotency: redisRepo,
		Encryptor:   fieldEncryptor,
	}}

	holderType := "NATURAL_PERSON"
	motherName := "Ana Beatriz Figueiredo"
	fatherName := "Carlos Eduardo Figueiredo"
	email := "maria.figueiredo@example.com"
	phone := "+5511987654321"
	payload := &mmodel.CreateHolderInput{
		Type:          &holderType,
		Name:          "Maria Clara Figueiredo",
		Document:      "91315026015",
		Contact:       &mmodel.Contact{PrimaryEmail: &email, MobilePhone: &phone},
		NaturalPerson: &mmodel.NaturalPerson{MotherName: &motherName, FatherName: &fatherName},
	}

	ctx := t.Context()
	orgID := uuid.New()
	ttl := time.Duration(300) // seconds, as ParseIdempotencyTTL resolves X-TTL

	created, replayed, err := handler.createHolder(ctx, orgID, payload, "holder-key-1", ttl)
	require.NoError(t, err)
	require.False(t, replayed)

	raw, err := redisContainer.Client.Get(ctx, crmservices.HolderIdempotencyKey(orgID.String(), "holder-key-1")).Result()
	require.NoError(t, err, "the create must leave its entity in the idempotency slot")
	require.NotEmpty(t, raw)

	assert.False(t, json.Valid([]byte(raw)), "the slot must not hold the entity JSON")

	for _, personal := range []string{payload.Name, payload.Document, motherName, fatherName, email, phone} {
		assert.NotContains(t, raw, personal, "the slot must not hold personal data in clear")
	}

	replay, replayed, err := handler.createHolder(ctx, orgID, payload, "holder-key-1", ttl)
	require.NoError(t, err)
	require.True(t, replayed, "an identical retry must be served from the slot")

	want, err := json.Marshal(created)
	require.NoError(t, err)

	got, err := json.Marshal(replay)
	require.NoError(t, err)

	assert.JSONEq(t, string(want), string(got), "the replay must return the original response")
}
