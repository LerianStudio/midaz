package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestMetadataIndexAdapter_CreateMetadataIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	cmdUseCase := &command.UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	qryUseCase := &query.UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	adapter := NewMetadataIndexAdapter(cmdUseCase, qryUseCase)

	t.Run("success", func(t *testing.T) {
		ctx := context.Background()
		input := &mmodel.CreateMetadataIndexInput{
			MetadataKey: "tier",
			Unique:      false,
			Sparse:      nil,
		}

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "transaction").
			Return([]*mongodb.MetadataIndex{}, nil)

		mockMetadataRepo.EXPECT().
			CreateIndex(gomock.Any(), "transaction", gomock.Any()).
			Return(&mongodb.MetadataIndex{
				EntityName:  "transaction",
				MetadataKey: "tier",
				Unique:      false,
				Sparse:      true,
			}, nil)

		result, err := adapter.CreateMetadataIndex(ctx, "transaction", input)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "transaction", result.EntityName)
		assert.Equal(t, "tier", result.MetadataKey)
	})

	t.Run("error - index already exists", func(t *testing.T) {
		ctx := context.Background()
		input := &mmodel.CreateMetadataIndexInput{
			MetadataKey: "tier",
			Unique:      false,
		}

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "transaction").
			Return([]*mongodb.MetadataIndex{
				{
					MetadataKey: "metadata.tier",
				},
			}, nil)

		result, err := adapter.CreateMetadataIndex(ctx, "transaction", input)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestMetadataIndexAdapter_GetAllMetadataIndexes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	cmdUseCase := &command.UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	qryUseCase := &query.UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	adapter := NewMetadataIndexAdapter(cmdUseCase, qryUseCase)

	t.Run("success - get all indexes", func(t *testing.T) {
		ctx := context.Background()
		filter := http.QueryHeader{}

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "transaction").
			Return([]*mongodb.MetadataIndex{
				{
					MetadataKey: "metadata.tier",
					Unique:      false,
					Sparse:      true,
				},
			}, nil)

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "operation").
			Return([]*mongodb.MetadataIndex{}, nil)

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "operation_route").
			Return([]*mongodb.MetadataIndex{}, nil)

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "transaction_route").
			Return([]*mongodb.MetadataIndex{}, nil)

		result, err := adapter.GetAllMetadataIndexes(ctx, filter)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Equal(t, "tier", result[0].MetadataKey)
	})

	t.Run("success - filter by entity name", func(t *testing.T) {
		ctx := context.Background()
		entityName := "transaction"
		filter := http.QueryHeader{
			EntityName: &entityName,
		}

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "transaction").
			Return([]*mongodb.MetadataIndex{
				{
					MetadataKey: "metadata.tier",
					Unique:      false,
					Sparse:      true,
					CreatedAt:   time.Now(),
				},
			}, nil)

		result, err := adapter.GetAllMetadataIndexes(ctx, filter)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 1)
	})

	t.Run("error - repository failure", func(t *testing.T) {
		ctx := context.Background()
		entityName := "transaction"
		filter := http.QueryHeader{
			EntityName: &entityName,
		}

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "transaction").
			Return(nil, errors.New("database error"))

		result, err := adapter.GetAllMetadataIndexes(ctx, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestMetadataIndexAdapter_DeleteMetadataIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	cmdUseCase := &command.UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	qryUseCase := &query.UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	adapter := NewMetadataIndexAdapter(cmdUseCase, qryUseCase)

	t.Run("success", func(t *testing.T) {
		ctx := context.Background()
		entityName := "transaction"
		indexName := "metadata.tier_1"

		mockMetadataRepo.EXPECT().
			DeleteIndex(gomock.Any(), entityName, indexName).
			Return(nil)

		err := adapter.DeleteMetadataIndex(ctx, entityName, indexName)

		assert.NoError(t, err)
	})

	t.Run("error - invalid index name format", func(t *testing.T) {
		ctx := context.Background()
		entityName := "transaction"
		indexName := "invalid_index"

		err := adapter.DeleteMetadataIndex(ctx, entityName, indexName)

		assert.Error(t, err)
	})

	t.Run("error - repository failure", func(t *testing.T) {
		ctx := context.Background()
		entityName := "transaction"
		indexName := "metadata.tier_1"

		mockMetadataRepo.EXPECT().
			DeleteIndex(gomock.Any(), entityName, indexName).
			Return(errors.New("index not found"))

		err := adapter.DeleteMetadataIndex(ctx, entityName, indexName)

		assert.Error(t, err)
	})
}

func TestNewMetadataIndexAdapter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	cmdUseCase := &command.UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	qryUseCase := &query.UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	adapter := NewMetadataIndexAdapter(cmdUseCase, qryUseCase)

	assert.NotNil(t, adapter)
	assert.Equal(t, cmdUseCase, adapter.Command)
	assert.Equal(t, qryUseCase, adapter.Query)
}
