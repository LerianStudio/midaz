package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/mongodb"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestMetadataUpdateSuccess is responsible to test MetadataUpdate with success
func TestMetadataUpdateSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7().String()
	metadata := map[string]any{}
	collection := reflect.TypeOf(mmodel.Organization{}).Name()
	uc := UseCase{
		MetadataRepo: mongodb.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		Update(gomock.Any(), collection, id, metadata).
		Return(nil).
		Times(1)

	err := uc.MetadataRepo.Update(context.TODO(), collection, id, metadata)
	assert.Nil(t, err)
}

// TestMetadataUpdateError is responsible to test MetadataUpdate with error
func TestMetadataUpdateError(t *testing.T) {
	errMSG := "err to update metadata on mongodb"
	id := pkg.GenerateUUIDv7().String()
	metadata := map[string]any{}
	collection := reflect.TypeOf(mmodel.Organization{}).Name()
	uc := UseCase{
		MetadataRepo: mongodb.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		Update(gomock.Any(), collection, id, metadata).
		Return(errors.New(errMSG)).
		Times(1)

	err := uc.MetadataRepo.Update(context.TODO(), collection, id, metadata)
	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
