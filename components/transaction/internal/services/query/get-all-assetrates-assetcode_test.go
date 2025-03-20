package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libPointers "github.com/LerianStudio/lib-commons/commons/pointers"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

// GetAllAssetRatesByAssetCode is responsible to test GetAllAssetRatesByAssetCode
func GetAllAssetRatesByAssetCode(t *testing.T) {
	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	fromAssetCode := "USD"
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := libHTTP.CursorPagination{
		Next: "next",
		Prev: "prev",
	}

	assetRate := &assetrate.AssetRate{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     libCommons.GenerateUUIDv7().String(),
		From:           fromAssetCode,
		To:             filter.ToAssetCodes[0],
		Rate:           100,
		Scale:          libPointers.Float64(2),
		Source:         libPointers.String("External System"),
		TTL:            3600,
	}

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination()).
		Return(assetRate, mockCur, nil).
		Times(1)
	res, cur, err := uc.AssetRateRepo.FindAllByAssetCodes(context.TODO(), orgID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination())

	assert.Equal(t, assetRate, res)
	assert.NotNil(t, cur)
	assert.Nil(t, err)
}

// GetAllAssetRatesByAssetCodeError is responsible to test GetAllAssetRatesByAssetCode with error
func GetAllAssetRatesByAssetCodeError(t *testing.T) {
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	fromAssetCode := "USD"
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination()).
		Return(nil, libHTTP.CursorPagination{}, errors.New(errMSG)).
		Times(1)
	res, cur, err := uc.AssetRateRepo.FindAllByAssetCodes(context.TODO(), orgID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination())

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
	assert.Equal(t, cur, libHTTP.CursorPagination{})
}
