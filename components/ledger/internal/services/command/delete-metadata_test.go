package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestMetadataDeleteSuccess is responsible to test MetadataDelete with success
func TestMetadataDeleteSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7().String()
	collection := reflect.TypeOf(mmodel.Organization{}).Name()
	uc := UseCase{
		MetadataRepo: mongodb.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		Delete(gomock.Any(), collection, id).
		Return(nil).
		Times(1)

	err := uc.MetadataRepo.Delete(context.TODO(), collection, id)
	assert.Nil(t, err)
}

// TestMetadataDeleteError is responsible to test MetadataDelete with error
func TestMetadataDeleteError(t *testing.T) {
	errMSG := "err to delete metadata on mongodb"
	id := pkg.GenerateUUIDv7().String()
	collection := reflect.TypeOf(mmodel.Organization{}).Name()
	uc := UseCase{
		MetadataRepo: mongodb.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		Delete(gomock.Any(), collection, id).
		Return(errors.New(errMSG)).
		Times(1)

	err := uc.MetadataRepo.Delete(context.TODO(), collection, id)
	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
