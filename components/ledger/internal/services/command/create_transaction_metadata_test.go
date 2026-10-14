// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestCreateTransactionMetadata pins the transaction-side metadata write: a filled
// map writes exactly one document keyed by the given entity, a write failure is
// returned unchanged, and nil or empty metadata never reaches the repository.
func TestCreateTransactionMetadata(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("mongo unavailable")

	tests := []struct {
		name         string
		metadata     map[string]any
		wantCreate   bool
		createErr    error
		expectedMeta map[string]any
	}{
		{
			name:         "filled metadata writes one document",
			metadata:     map[string]any{"k": "v", "n": float64(1)},
			wantCreate:   true,
			expectedMeta: map[string]any{"k": "v", "n": float64(1)},
		},
		{
			name:       "write failure is propagated",
			metadata:   map[string]any{"k": "v"},
			wantCreate: true,
			createErr:  writeErr,
		},
		{
			name:     "nil metadata writes no document",
			metadata: nil,
		},
		{
			name:     "empty metadata writes no document",
			metadata: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			// Without a registered Create expectation, any write attempt fails the test.
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)

			if tt.wantCreate {
				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), constant.EntityOperationRoute, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
						assert.Equal(t, "entity-id", meta.EntityID)
						assert.Equal(t, constant.EntityOperationRoute, meta.EntityName)
						assert.Equal(t, mongodb.JSON(tt.metadata), meta.Data)
						assert.False(t, meta.CreatedAt.IsZero(), "CreatedAt must be stamped")
						assert.Equal(t, meta.CreatedAt, meta.UpdatedAt, "a new document has UpdatedAt == CreatedAt")

						return tt.createErr
					}).
					Times(1)
			}

			uc := &UseCase{TransactionMetadataRepo: mockMetadataRepo}

			result, err := uc.CreateTransactionMetadata(context.Background(), constant.EntityOperationRoute, "entity-id", tt.metadata)

			if tt.createErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.createErr, err, "the repository error must be returned unchanged")
				assert.Nil(t, result)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedMeta, result)
		})
	}
}

// TestCreateTransactionMetadata_WriteFailureMarksSpan checks that a technical write
// failure flips the helper's own span to error and is not logged: the calling use case
// owns the single log line.
func TestCreateTransactionMetadata_WriteFailureMarksSpan(t *testing.T) {
	ctrl := gomock.NewController(t)

	writeErr := errors.New("mongo unavailable")

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), constant.EntityTransactionRoute, gomock.Any()).
		Return(writeErr).
		Times(1)

	uc := &UseCase{TransactionMetadataRepo: mockMetadataRepo}

	ctx, recorder, logger := accountFindErrorContext()

	result, err := uc.CreateTransactionMetadata(ctx, constant.EntityTransactionRoute, "entity-id", map[string]any{"k": "v"})

	require.ErrorIs(t, err, writeErr)
	assert.Nil(t, result)

	span := findSpan(t, recorder, "command.create_transaction_metadata")
	assert.Equal(t, codes.Error, span.Status().Code)

	assert.Empty(t, logger.snapshot(), "the helper records on its span only; the use case logs")
}
