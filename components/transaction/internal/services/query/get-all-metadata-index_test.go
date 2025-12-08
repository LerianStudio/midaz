package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllMetadataIndexes tests the GetAllMetadataIndexes method with success and error scenarios
func TestGetAllMetadataIndexes(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	t.Run("Success_WithoutEntityFilter", func(t *testing.T) {
		filter := http.QueryHeader{
			Limit: 10,
			Page:  1,
		}

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "transaction").
			Return([]*mongodb.MetadataIndex{
				{MetadataKey: "tier", Unique: false, Sparse: true},
			}, nil).
			Times(1)

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "operation").
			Return([]*mongodb.MetadataIndex{
				{MetadataKey: "category", Unique: true, Sparse: false},
			}, nil).
			Times(1)

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "operation_route").
			Return([]*mongodb.MetadataIndex{}, nil).
			Times(1)

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), "transaction_route").
			Return([]*mongodb.MetadataIndex{}, nil).
			Times(1)

		result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 2)
	})

	t.Run("Success_WithEntityFilter", func(t *testing.T) {
		entityName := "transaction"
		filter := http.QueryHeader{
			Limit:      10,
			Page:       1,
			EntityName: &entityName,
		}

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), entityName).
			Return([]*mongodb.MetadataIndex{
				{MetadataKey: "tier", Unique: false, Sparse: true},
				{MetadataKey: "priority", Unique: false, Sparse: true},
			}, nil).
			Times(1)

		result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 2)
		assert.Equal(t, "metadata.tier_1", result[0].IndexName)
		assert.Equal(t, "tier", result[0].MetadataKey)
		assert.Equal(t, "transaction", result[0].EntityName)
		assert.Equal(t, "metadata.priority_1", result[1].IndexName)
		assert.Equal(t, "priority", result[1].MetadataKey)
	})

	t.Run("Success_EmptyResult", func(t *testing.T) {
		entityName := "operation"
		filter := http.QueryHeader{
			Limit:      10,
			Page:       1,
			EntityName: &entityName,
		}

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), entityName).
			Return([]*mongodb.MetadataIndex{}, nil).
			Times(1)

		result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("Success_SkipsMetadataPrefixedKeys", func(t *testing.T) {
		entityName := "transaction"
		filter := http.QueryHeader{
			Limit:      10,
			Page:       1,
			EntityName: &entityName,
		}

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), entityName).
			Return([]*mongodb.MetadataIndex{
				{MetadataKey: "metadata.existing_index", Unique: false, Sparse: true},
				{MetadataKey: "tier", Unique: false, Sparse: true},
			}, nil).
			Times(1)

		result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Equal(t, "tier", result[0].MetadataKey)
	})

	t.Run("Success_SkipsEmptyMetadataKey", func(t *testing.T) {
		entityName := "transaction"
		filter := http.QueryHeader{
			Limit:      10,
			Page:       1,
			EntityName: &entityName,
		}

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), entityName).
			Return([]*mongodb.MetadataIndex{
				{MetadataKey: "", Unique: false, Sparse: true},
				{MetadataKey: "tier", Unique: false, Sparse: true},
			}, nil).
			Times(1)

		result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Equal(t, "tier", result[0].MetadataKey)
	})

	t.Run("Error_RepositoryError", func(t *testing.T) {
		entityName := "transaction"
		filter := http.QueryHeader{
			Limit:      10,
			Page:       1,
			EntityName: &entityName,
		}

		repoError := errors.New("database connection error")

		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), entityName).
			Return(nil, repoError).
			Times(1)

		result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, repoError, err)
	})

	t.Run("Error_RepositoryErrorOnSecondEntity", func(t *testing.T) {
		filter := http.QueryHeader{
			Limit: 10,
			Page:  1,
		}

		repoError := errors.New("database error")

		// First call succeeds
		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), gomock.Any()).
			Return([]*mongodb.MetadataIndex{}, nil).
			Times(1)

		// Second call fails
		mockMetadataRepo.EXPECT().
			FindAllIndexes(gomock.Any(), gomock.Any()).
			Return(nil, repoError).
			Times(1)

		result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, repoError, err)
	})
}

// TestGetAllMetadataIndexesIndexNameFormat tests that the index name is formatted correctly
func TestGetAllMetadataIndexesIndexNameFormat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	entityName := "transaction"
	filter := http.QueryHeader{
		Limit:      10,
		Page:       1,
		EntityName: &entityName,
	}

	mockMetadataRepo.EXPECT().
		FindAllIndexes(gomock.Any(), entityName).
		Return([]*mongodb.MetadataIndex{
			{MetadataKey: "custom_field", Unique: true, Sparse: false},
		}, nil).
		Times(1)

	result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 1)
	assert.Equal(t, "metadata.custom_field_1", result[0].IndexName)
	assert.Equal(t, "custom_field", result[0].MetadataKey)
	assert.Equal(t, "transaction", result[0].EntityName)
	assert.True(t, result[0].Unique)
	assert.False(t, result[0].Sparse)
}

// TestGetAllMetadataIndexesPreservesIndexProperties tests that index properties are preserved
func TestGetAllMetadataIndexesPreservesIndexProperties(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	entityName := "operation"
	filter := http.QueryHeader{
		Limit:      10,
		Page:       1,
		EntityName: &entityName,
	}

	mockMetadataRepo.EXPECT().
		FindAllIndexes(gomock.Any(), entityName).
		Return([]*mongodb.MetadataIndex{
			{MetadataKey: "unique_key", Unique: true, Sparse: true},
			{MetadataKey: "non_unique_key", Unique: false, Sparse: false},
		}, nil).
		Times(1)

	result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 2)

	assert.Equal(t, "unique_key", result[0].MetadataKey)
	assert.True(t, result[0].Unique)
	assert.True(t, result[0].Sparse)

	assert.Equal(t, "non_unique_key", result[1].MetadataKey)
	assert.False(t, result[1].Unique)
	assert.False(t, result[1].Sparse)
}

// TestGetAllMetadataIndexesWithEmptyEntityName tests behavior when entity name is empty string
func TestGetAllMetadataIndexesWithEmptyEntityName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	emptyEntityName := ""
	filter := http.QueryHeader{
		Limit:      10,
		Page:       1,
		EntityName: &emptyEntityName,
	}

	mockMetadataRepo.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		Return([]*mongodb.MetadataIndex{}, nil).
		Times(4)

	result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

	assert.NoError(t, err)
	assert.Empty(t, result)
}

// TestGetAllMetadataIndexesMultipleEntitiesAggregation tests aggregation of indexes from multiple entities
func TestGetAllMetadataIndexesMultipleEntitiesAggregation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit: 10,
		Page:  1,
	}

	mockMetadataRepo.EXPECT().
		FindAllIndexes(gomock.Any(), "transaction").
		Return([]*mongodb.MetadataIndex{
			{MetadataKey: "tx_field", Unique: false, Sparse: true},
		}, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindAllIndexes(gomock.Any(), "operation").
		Return([]*mongodb.MetadataIndex{
			{MetadataKey: "op_field", Unique: true, Sparse: false},
		}, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindAllIndexes(gomock.Any(), "operation_route").
		Return([]*mongodb.MetadataIndex{
			{MetadataKey: "or_field", Unique: false, Sparse: true},
		}, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindAllIndexes(gomock.Any(), "transaction_route").
		Return([]*mongodb.MetadataIndex{
			{MetadataKey: "tr_field", Unique: true, Sparse: true},
		}, nil).
		Times(1)

	result, err := uc.GetAllMetadataIndexes(context.Background(), filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 4)

	entityNames := make(map[string]bool)
	for _, idx := range result {
		entityNames[idx.EntityName] = true
	}

	assert.True(t, entityNames["transaction"])
	assert.True(t, entityNames["operation"])
	assert.True(t, entityNames["operation_route"])
	assert.True(t, entityNames["transaction_route"])
}
