package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllSettingsSuccess tests successful retrieval of all settings
func TestGetAllSettingsSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingID1 := libCommons.GenerateUUIDv7()
	settingID2 := libCommons.GenerateUUIDv7()

	mockRepo := settings.NewMockRepository(ctrl)
	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	active1 := true
	active2 := false
	expectedSettings := []*mmodel.Settings{
		{
			ID:             settingID1,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Key:            "setting1",
			Active:         &active1,
			Description:    "Description 1",
		},
		{
			ID:             settingID2,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Key:            "setting2",
			Active:         &active2,
			Description:    "Description 2",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedSettings, expectedCursor, nil)

	result, cursor, err := uc.GetAllSettings(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedSettings, result)
	assert.Equal(t, expectedCursor, cursor)
}

// TestGetAllSettingsNotFound tests when no settings are found
func TestGetAllSettingsNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	mockRepo := settings.NewMockRepository(ctrl)
	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound)

	result, cursor, err := uc.GetAllSettings(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cursor)
	businessError := pkg.ValidateBusinessError(constant.ErrSettingsNotFound, reflect.TypeOf(mmodel.Settings{}).Name())
	assert.Equal(t, businessError, err)
}

// TestGetAllSettingsRepositoryError tests repository error handling
func TestGetAllSettingsRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	mockRepo := settings.NewMockRepository(ctrl)
	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	expectedError := errors.New("database connection error")

	mockRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, expectedError)

	result, cursor, err := uc.GetAllSettings(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cursor)
	assert.Equal(t, expectedError, err)
}
