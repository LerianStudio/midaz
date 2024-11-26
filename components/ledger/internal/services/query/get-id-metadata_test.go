package query

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestMetadataFindByEntitySuccess is responsible to test MetadataFindByEntity with success
func TestMetadataFindByEntitySuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7().String()
	collection := reflect.TypeOf(mmodel.Organization{}).Name()
	metadata := &mongodb.Metadata{ID: primitive.NewObjectID()}
	uc := UseCase{
		MetadataRepo: mongodb.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		FindByEntity(gomock.Any(), collection, id).
		Return(metadata, nil).
		Times(1)

	res, err := uc.MetadataRepo.FindByEntity(context.TODO(), collection, id)

	assert.Equal(t, res, metadata)
	assert.Nil(t, err)
}

// TestMetadataFindByEntityError is responsible to test MetadataFindByEntity with error
func TestMetadataFindByEntityError(t *testing.T) {
	errMSG := "err to findByEntity metadata on mongodb"
	id := pkg.GenerateUUIDv7().String()
	collection := reflect.TypeOf(mmodel.Organization{}).Name()
	uc := UseCase{
		MetadataRepo: mongodb.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		FindByEntity(gomock.Any(), collection, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.MetadataRepo.FindByEntity(context.TODO(), collection, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
