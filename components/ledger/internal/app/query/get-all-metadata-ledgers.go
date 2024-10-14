package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"
	"reflect"

	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/google/uuid"
)

// GetAllMetadataLedgers fetch all Ledgers from the repository
func (uc *UseCase) GetAllMetadataLedgers(ctx context.Context, organizationID string, filter commonHTTP.QueryHeader) ([]*l.Ledger, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving ledgers")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(l.Ledger{}).Name(), filter)
	if err != nil || metadata == nil {
		return nil, common.ValidateBusinessError(cn.NoLedgersFoundBusinessError, reflect.TypeOf(l.Ledger{}).Name())
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	ledgers, err := uc.LedgerRepo.ListByIDs(ctx, uuid.MustParse(organizationID), uuids)
	if err != nil {
		logger.Errorf("Error getting ledgers on repo by query params: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.NoLedgersFoundBusinessError, reflect.TypeOf(l.Ledger{}).Name())
		}

		return nil, err
	}

	for i := range ledgers {
		if data, ok := metadataMap[ledgers[i].ID]; ok {
			ledgers[i].Metadata = data
		}
	}

	return ledgers, nil
}
