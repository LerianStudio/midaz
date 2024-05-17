package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	meta "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/metadata"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
)

// TestGetAllMetadataProducts is responsible to test TestGetAllMetadataProducts with success and error
func TestGetAllMetadataProducts(t *testing.T) {
	collection := reflect.TypeOf(p.Product{}).Name()
	filter := bson.M{"metadata": 1}

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mock.NewMockRepository(gomock.NewController(t))
	uc := UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	t.Run("Success", func(t *testing.T) {
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), collection, filter).
			Return([]*meta.Metadata{{ID: primitive.NewObjectID()}}, nil).
			Times(1)
		res, err := uc.MetadataRepo.FindList(context.TODO(), collection, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMSG := "errDatabaseItemNotFound"
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), collection, filter).
			Return(nil, errors.New(errMSG)).
			Times(1)
		res, err := uc.MetadataRepo.FindList(context.TODO(), collection, filter)

		assert.EqualError(t, err, errMSG)
		assert.Nil(t, res)
	})
}
