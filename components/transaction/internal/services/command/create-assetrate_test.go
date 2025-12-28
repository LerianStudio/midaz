package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateOrUpdateAssetRate(t *testing.T) {
	ctx := context.Background()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	t.Run("success - create new asset rate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 3600
		input := &mmodel.CreateAssetRateInput{
			From:   "USD",
			To:     "BRL",
			Rate:   500,
			Scale:  2,
			Source: libPointers.String("External System"),
			TTL:    &ttl,
		}

		mockAssetRateRepo.EXPECT().
			FindByCurrencyPair(gomock.Any(), organizationID, ledgerID, "USD", "BRL").
			Return(nil, nil).
			Times(1)

		mockAssetRateRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, ar *mmodel.AssetRate) (*mmodel.AssetRate, error) {
				assert.Equal(t, organizationID.String(), ar.OrganizationID)
				assert.Equal(t, ledgerID.String(), ar.LedgerID)
				assert.Equal(t, "USD", ar.From)
				assert.Equal(t, "BRL", ar.To)
				assert.Equal(t, float64(500), ar.Rate)
				return ar, nil
			}).
			Times(1)

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "USD", result.From)
		assert.Equal(t, "BRL", result.To)
	})

	t.Run("success - create new asset rate with metadata", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 3600
		metadata := map[string]any{"provider": "Central Bank"}
		input := &mmodel.CreateAssetRateInput{
			From:     "EUR",
			To:       "USD",
			Rate:     110,
			Scale:    2,
			Source:   libPointers.String("External System"),
			TTL:      &ttl,
			Metadata: metadata,
		}

		mockAssetRateRepo.EXPECT().
			FindByCurrencyPair(gomock.Any(), organizationID, ledgerID, "EUR", "USD").
			Return(nil, nil).
			Times(1)

		mockAssetRateRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, ar *mmodel.AssetRate) (*mmodel.AssetRate, error) {
				return ar, nil
			}).
			Times(1)

		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), "AssetRate", gomock.Any()).
			Return(nil).
			Times(1)

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, metadata, result.Metadata)
	})

	t.Run("success - update existing asset rate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 7200
		existingID := libCommons.GenerateUUIDv7().String()
		existingRate := &mmodel.AssetRate{
			ID:             existingID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			From:           "USD",
			To:             "BRL",
			Rate:           500,
			Scale:          libPointers.Float64(2),
			TTL:            3600,
		}

		input := &mmodel.CreateAssetRateInput{
			From:   "USD",
			To:     "BRL",
			Rate:   550,
			Scale:  2,
			Source: libPointers.String("Updated System"),
			TTL:    &ttl,
		}

		mockAssetRateRepo.EXPECT().
			FindByCurrencyPair(gomock.Any(), organizationID, ledgerID, "USD", "BRL").
			Return(existingRate, nil).
			Times(1)

		mockAssetRateRepo.EXPECT().
			Update(gomock.Any(), organizationID, ledgerID, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ any, id any, ar *mmodel.AssetRate) (*mmodel.AssetRate, error) {
				assert.Equal(t, float64(550), ar.Rate)
				assert.Equal(t, 7200, ar.TTL)
				return ar, nil
			}).
			Times(1)

		mockMetadataRepo.EXPECT().
			FindByEntity(gomock.Any(), "AssetRate", existingID).
			Return(nil, nil).
			Times(1)

		mockMetadataRepo.EXPECT().
			Update(gomock.Any(), "AssetRate", existingID, gomock.Any()).
			Return(nil).
			Times(1)

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("error - invalid from asset code (lowercase)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 3600
		input := &mmodel.CreateAssetRateInput{
			From:   "usd",
			To:     "BRL",
			Rate:   500,
			Scale:  2,
			Source: libPointers.String("External System"),
			TTL:    &ttl,
		}

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.Error(t, err)
		assert.Nil(t, result)
		// Error should indicate validation failure for asset code
		assert.Contains(t, err.Error(), "0004")
	})

	t.Run("error - invalid to asset code (contains number)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 3600
		input := &mmodel.CreateAssetRateInput{
			From:   "USD",
			To:     "BRL123",
			Rate:   500,
			Scale:  2,
			Source: libPointers.String("External System"),
			TTL:    &ttl,
		}

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.Error(t, err)
		assert.Nil(t, result)
		// Error should indicate validation failure for asset code
		assert.Contains(t, err.Error(), "0033")
	})

	t.Run("error - find currency pair fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 3600
		input := &mmodel.CreateAssetRateInput{
			From:   "USD",
			To:     "BRL",
			Rate:   500,
			Scale:  2,
			Source: libPointers.String("External System"),
			TTL:    &ttl,
		}

		mockAssetRateRepo.EXPECT().
			FindByCurrencyPair(gomock.Any(), organizationID, ledgerID, "USD", "BRL").
			Return(nil, errors.New("database error")).
			Times(1)

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.Error(t, err)
		assert.Nil(t, result)
		var internalErr pkg.InternalServerError
		assert.True(t, errors.As(err, &internalErr))
	})

	t.Run("error - create asset rate fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 3600
		input := &mmodel.CreateAssetRateInput{
			From:   "GBP",
			To:     "EUR",
			Rate:   115,
			Scale:  2,
			Source: libPointers.String("External System"),
			TTL:    &ttl,
		}

		mockAssetRateRepo.EXPECT().
			FindByCurrencyPair(gomock.Any(), organizationID, ledgerID, "GBP", "EUR").
			Return(nil, nil).
			Times(1)

		mockAssetRateRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("create failed")).
			Times(1)

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.Error(t, err)
		assert.Nil(t, result)
		var internalErr pkg.InternalServerError
		assert.True(t, errors.As(err, &internalErr))
	})

	t.Run("error - update asset rate fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 7200
		existingID := libCommons.GenerateUUIDv7().String()
		existingRate := &mmodel.AssetRate{
			ID:             existingID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			From:           "JPY",
			To:             "USD",
			Rate:           100,
			Scale:          libPointers.Float64(4),
			TTL:            3600,
		}

		input := &mmodel.CreateAssetRateInput{
			From:   "JPY",
			To:     "USD",
			Rate:   110,
			Scale:  4,
			Source: libPointers.String("Updated System"),
			TTL:    &ttl,
		}

		mockAssetRateRepo.EXPECT().
			FindByCurrencyPair(gomock.Any(), organizationID, ledgerID, "JPY", "USD").
			Return(existingRate, nil).
			Times(1)

		mockAssetRateRepo.EXPECT().
			Update(gomock.Any(), organizationID, ledgerID, gomock.Any(), gomock.Any()).
			Return(nil, errors.New("update failed")).
			Times(1)

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.Error(t, err)
		assert.Nil(t, result)
		var internalErr pkg.InternalServerError
		assert.True(t, errors.As(err, &internalErr))
	})

	t.Run("error - create metadata fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 3600
		metadata := map[string]any{"provider": "Central Bank"}
		input := &mmodel.CreateAssetRateInput{
			From:     "CHF",
			To:       "EUR",
			Rate:     107,
			Scale:    2,
			Source:   libPointers.String("External System"),
			TTL:      &ttl,
			Metadata: metadata,
		}

		mockAssetRateRepo.EXPECT().
			FindByCurrencyPair(gomock.Any(), organizationID, ledgerID, "CHF", "EUR").
			Return(nil, nil).
			Times(1)

		mockAssetRateRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, ar *mmodel.AssetRate) (*mmodel.AssetRate, error) {
				return ar, nil
			}).
			Times(1)

		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), "AssetRate", gomock.Any()).
			Return(errors.New("metadata creation failed")).
			Times(1)

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.Error(t, err)
		assert.Nil(t, result)
		var internalErr pkg.InternalServerError
		assert.True(t, errors.As(err, &internalErr))
	})

	t.Run("success - create with external ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 3600
		externalID := libCommons.GenerateUUIDv7().String()
		input := &mmodel.CreateAssetRateInput{
			From:       "AUD",
			To:         "NZD",
			Rate:       108,
			Scale:      2,
			Source:     libPointers.String("External System"),
			TTL:        &ttl,
			ExternalID: &externalID,
		}

		mockAssetRateRepo.EXPECT().
			FindByCurrencyPair(gomock.Any(), organizationID, ledgerID, "AUD", "NZD").
			Return(nil, nil).
			Times(1)

		mockAssetRateRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, ar *mmodel.AssetRate) (*mmodel.AssetRate, error) {
				assert.Equal(t, externalID, ar.ExternalID)
				return ar, nil
			}).
			Times(1)

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, externalID, result.ExternalID)
	})

	t.Run("success - update existing with external ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 7200
		existingID := libCommons.GenerateUUIDv7().String()
		newExternalID := libCommons.GenerateUUIDv7().String()
		existingRate := &mmodel.AssetRate{
			ID:             existingID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			From:           "CAD",
			To:             "USD",
			Rate:           75,
			Scale:          libPointers.Float64(2),
			TTL:            3600,
		}

		input := &mmodel.CreateAssetRateInput{
			From:       "CAD",
			To:         "USD",
			Rate:       76,
			Scale:      2,
			Source:     libPointers.String("Updated System"),
			TTL:        &ttl,
			ExternalID: &newExternalID,
		}

		mockAssetRateRepo.EXPECT().
			FindByCurrencyPair(gomock.Any(), organizationID, ledgerID, "CAD", "USD").
			Return(existingRate, nil).
			Times(1)

		mockAssetRateRepo.EXPECT().
			Update(gomock.Any(), organizationID, ledgerID, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ any, id any, ar *mmodel.AssetRate) (*mmodel.AssetRate, error) {
				assert.Equal(t, newExternalID, ar.ExternalID)
				return ar, nil
			}).
			Times(1)

		mockMetadataRepo.EXPECT().
			FindByEntity(gomock.Any(), "AssetRate", existingID).
			Return(nil, nil).
			Times(1)

		mockMetadataRepo.EXPECT().
			Update(gomock.Any(), "AssetRate", existingID, gomock.Any()).
			Return(nil).
			Times(1)

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("error - update metadata find fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		uc := &UseCase{
			AssetRateRepo: mockAssetRateRepo,
			MetadataRepo:  mockMetadataRepo,
		}

		ttl := 7200
		existingID := libCommons.GenerateUUIDv7().String()
		existingRate := &mmodel.AssetRate{
			ID:             existingID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			From:           "SGD",
			To:             "USD",
			Rate:           74,
			Scale:          libPointers.Float64(2),
			TTL:            3600,
		}

		input := &mmodel.CreateAssetRateInput{
			From:     "SGD",
			To:       "USD",
			Rate:     75,
			Scale:    2,
			Source:   libPointers.String("Updated System"),
			TTL:      &ttl,
			Metadata: map[string]any{"key": "value"},
		}

		mockAssetRateRepo.EXPECT().
			FindByCurrencyPair(gomock.Any(), organizationID, ledgerID, "SGD", "USD").
			Return(existingRate, nil).
			Times(1)

		mockAssetRateRepo.EXPECT().
			Update(gomock.Any(), organizationID, ledgerID, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ any, id any, ar *mmodel.AssetRate) (*mmodel.AssetRate, error) {
				return ar, nil
			}).
			Times(1)

		mockMetadataRepo.EXPECT().
			FindByEntity(gomock.Any(), "AssetRate", existingID).
			Return(nil, errors.New("metadata find failed")).
			Times(1)

		result, err := uc.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, input)

		require.Error(t, err)
		assert.Nil(t, result)
		var internalErr pkg.InternalServerError
		assert.True(t, errors.As(err, &internalErr))
	})
}

// TestUpdateAssetRateSuccess is responsible to test TestUpdateAssetRateSuccess with success
func TestUpdateAssetRateSuccess(t *testing.T) {
	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	exID := libCommons.GenerateUUIDv7()

	assetRateEntity := &assetrate.AssetRate{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     exID.String(),
		From:           "USD",
		To:             "BRL",
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
		FindByCurrencyPair(gomock.Any(), orgID, ledgerID, assetRateEntity.From, assetRateEntity.To).
		Return(assetRateEntity, nil).
		Times(1)
	res, err := uc.AssetRateRepo.FindByCurrencyPair(context.TODO(), orgID, ledgerID, assetRateEntity.From, assetRateEntity.To)
	if err != nil {
		t.Errorf("Error finding asset rate by currency pair: %v", err)
	}

	assert.Equal(t, assetRateEntity.OrganizationID, res.OrganizationID)
	assert.Equal(t, assetRateEntity.LedgerID, res.LedgerID)
	assert.Equal(t, assetRateEntity.ExternalID, res.ExternalID)
	assert.Equal(t, assetRateEntity.From, res.From)
	assert.Equal(t, assetRateEntity.To, res.To)
	assert.Equal(t, assetRateEntity.Rate, res.Rate)
	assert.Equal(t, assetRateEntity.Scale, res.Scale)
	assert.Equal(t, assetRateEntity.Source, res.Source)
	assert.Equal(t, assetRateEntity.TTL, res.TTL)
	assert.Nil(t, err)

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		Update(gomock.Any(), orgID, ledgerID, id, assetRateEntity).
		Return(assetRateEntity, nil).
		Times(1)
	res, err = uc.AssetRateRepo.Update(context.TODO(), orgID, ledgerID, id, assetRateEntity)
	if err != nil {
		t.Errorf("Error creating asset rate: %v", err)
	}

	assert.Equal(t, assetRateEntity.OrganizationID, res.OrganizationID)
	assert.Equal(t, assetRateEntity.LedgerID, res.LedgerID)
	assert.Equal(t, assetRateEntity.ExternalID, res.ExternalID)
	assert.Equal(t, assetRateEntity.From, res.From)
	assert.Equal(t, assetRateEntity.To, res.To)
	assert.Equal(t, assetRateEntity.Rate, res.Rate)
	assert.Equal(t, assetRateEntity.Scale, res.Scale)
	assert.Equal(t, assetRateEntity.Source, res.Source)
	assert.Equal(t, assetRateEntity.TTL, res.TTL)
	assert.Nil(t, err)
}

// TestCreateAssetRateSuccess is responsible to test TestCreateAssetRateSuccess with success
func TestCreateAssetRateSuccess(t *testing.T) {
	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	exID := libCommons.GenerateUUIDv7()

	assetRateEntity := &assetrate.AssetRate{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     exID.String(),
		From:           "USD",
		To:             "BRL",
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
		FindByCurrencyPair(gomock.Any(), orgID, ledgerID, assetRateEntity.From, assetRateEntity.To).
		Return(nil, nil).
		Times(1)
	res, err := uc.AssetRateRepo.FindByCurrencyPair(context.TODO(), orgID, ledgerID, assetRateEntity.From, assetRateEntity.To)
	if err != nil {
		t.Errorf("Error finding asset rate by currency pair: %v", err)
	}

	assert.Nil(t, res)
	assert.Nil(t, err)

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		Create(gomock.Any(), assetRateEntity).
		Return(assetRateEntity, nil).
		Times(1)
	res, err = uc.AssetRateRepo.Create(context.TODO(), assetRateEntity)
	if err != nil {
		t.Errorf("Error creating asset rate: %v", err)
	}

	assert.Equal(t, assetRateEntity.OrganizationID, res.OrganizationID)
	assert.Equal(t, assetRateEntity.LedgerID, res.LedgerID)
	assert.Equal(t, assetRateEntity.ExternalID, res.ExternalID)
	assert.Equal(t, assetRateEntity.From, res.From)
	assert.Equal(t, assetRateEntity.To, res.To)
	assert.Equal(t, assetRateEntity.Rate, res.Rate)
	assert.Equal(t, assetRateEntity.Scale, res.Scale)
	assert.Equal(t, assetRateEntity.Source, res.Source)
	assert.Equal(t, assetRateEntity.TTL, res.TTL)
	assert.Nil(t, err)
}

// TestCreateAssetRateError is responsible to test CreateAssetRateError with error
func TestCreateAssetRateError(t *testing.T) {
	errMSG := "err to create asset rate on database"
	assetRateEntity := &assetrate.AssetRate{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: libCommons.GenerateUUIDv7().String(),
		LedgerID:       libCommons.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		Create(gomock.Any(), assetRateEntity).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRateRepo.Create(context.TODO(), assetRateEntity)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
