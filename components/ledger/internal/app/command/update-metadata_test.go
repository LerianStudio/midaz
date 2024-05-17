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

// TestMetadataUpdateSuccess is responsible to test MetadataUpdate with success
func TestMetadataUpdateSuccess(t *testing.T) {
	id := uuid.New().String()
	metadata := map[string]any{}
	collection := reflect.TypeOf(o.Organization{}).Name()
	uc := UseCase{
		MetadataRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mock.MockRepository).
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
	id := uuid.New().String()
	metadata := map[string]any{}
	collection := reflect.TypeOf(o.Organization{}).Name()
	uc := UseCase{
		MetadataRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.MetadataRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), collection, id, metadata).
		Return(errors.New(errMSG)).
		Times(1)

	err := uc.MetadataRepo.Update(context.TODO(), collection, id, metadata)
	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
