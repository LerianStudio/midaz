package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/metadata"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestMetadataDeleteSuccess is responsible to test MetadataDelete with success
func TestMetadataDeleteSuccess(t *testing.T) {
	id := uuid.New().String()
	collection := reflect.TypeOf(o.Organization{}).Name()
	uc := UseCase{
		MetadataRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mock.MockRepository).
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
	id := uuid.New().String()
	collection := reflect.TypeOf(o.Organization{}).Name()
	uc := UseCase{
		MetadataRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), collection, id).
		Return(errors.New(errMSG)).
		Times(1)

	err := uc.MetadataRepo.Delete(context.TODO(), collection, id)
	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
