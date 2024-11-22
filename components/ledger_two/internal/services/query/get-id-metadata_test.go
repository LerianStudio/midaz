package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	meta "github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/interface/metadata"
	mock "github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/mock/metadata"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
)

// TestMetadataFindByEntitySuccess is responsible to test MetadataFindByEntity with success
func TestMetadataFindByEntitySuccess(t *testing.T) {
	id := common.GenerateUUIDv7().String()
	collection := reflect.TypeOf(mmodel.Organization{}).Name()
	metadata := &meta.Metadata{ID: primitive.NewObjectID()}
	uc := UseCase{
		MetadataRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mock.MockRepository).
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
	id := common.GenerateUUIDv7().String()
	collection := reflect.TypeOf(mmodel.Organization{}).Name()
	uc := UseCase{
		MetadataRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mock.MockRepository).
		EXPECT().
		FindByEntity(gomock.Any(), collection, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.MetadataRepo.FindByEntity(context.TODO(), collection, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
