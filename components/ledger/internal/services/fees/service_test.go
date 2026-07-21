// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"
)

func TestNewUseCase(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockResolver := feeshared.NewMockMidazResolver(ctrl)

	tests := []struct {
		name            string
		packageRepo     pack.Repository
		resolver        feeshared.MidazResolver
		defaultCurrency string
		wantErr         bool
		errContains     string
	}{
		{
			name:            "Success - all dependencies provided",
			packageRepo:     mockPackRepo,
			resolver:        mockResolver,
			defaultCurrency: "BRL",
			wantErr:         false,
		},
		{
			name:            "Error - nil PackageRepo",
			packageRepo:     nil,
			resolver:        mockResolver,
			defaultCurrency: "BRL",
			wantErr:         true,
			errContains:     "PackageRepo",
		},
		{
			name:            "Error - nil MidazResolver",
			packageRepo:     mockPackRepo,
			resolver:        nil,
			defaultCurrency: "BRL",
			wantErr:         true,
			errContains:     "MidazResolver",
		},
		{
			name:            "Error - empty DefaultCurrency",
			packageRepo:     mockPackRepo,
			resolver:        mockResolver,
			defaultCurrency: "",
			wantErr:         true,
			errContains:     "DefaultCurrency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, err := NewUseCase(tt.packageRepo, tt.resolver, tt.defaultCurrency)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, uc)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, uc)
				assert.Equal(t, tt.packageRepo, uc.PackageRepo())
				assert.Equal(t, tt.resolver, uc.Resolver())
				assert.Equal(t, tt.defaultCurrency, uc.DefaultCurrency())
			}
		})
	}
}

// TestFeesEmit_NilAndNoopEmitter_NoPanic asserts the mutation paths do not panic
// when Streaming is nil (disabled) or a Noop emitter.
func TestFeesEmit_NilAndNoopEmitter_NoPanic(t *testing.T) {
	emitters := map[string]libStreaming.Emitter{
		"nil":  nil,
		"noop": libStreaming.NewNoopEmitter(),
	}

	for name, emitter := range emitters {
		emitter := emitter
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPackRepo := pack.NewMockRepository(ctrl)

			orgID := uuid.New()
			packID := uuid.New()
			ledgerID := uuid.New()

			found := &pack.Package{ID: packID, LedgerID: ledgerID}

			mockPackRepo.EXPECT().
				FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(found, nil)
			mockPackRepo.EXPECT().
				SoftDelete(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil)

			svc := &UseCase{
				packageRepo: mockPackRepo,
				Streaming:   emitter,
			}

			var err error
			assert.NotPanics(t, func() {
				err = svc.DeletePackageByID(context.Background(), packID, orgID)
			})
			require.NoError(t, err, "disabled streaming must not break the delete mutation")
		})
	}
}

// TestPackageRepositoryUpdate_ReturnsPersistedEntity asserts the Repository
// interface Update returns the persisted entity (DECISION 1). This is a
// compile-and-shape lock on the mock; the real Mongo behavior is covered by
// integration tests.
func TestPackageRepositoryUpdate_ReturnsPersistedEntity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)

	want := &pack.Package{ID: uuid.New(), FeeGroupLabel: "persisted"}

	mockPackRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(want, nil)

	updateFields := bson.M{}

	got, err := mockPackRepo.Update(context.Background(), uuid.New(), uuid.New(), &updateFields)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// unmarshalPayload decodes a captured emit payload into a generic map so
// tests can assert individual wire fields without coupling to the payload
// struct type.
func unmarshalPayload(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	return payload
}
