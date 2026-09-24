// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/google/uuid"
)

//go:generate mockgen -source=limit_asset_repository.go -destination=mocks/limit_asset_repository_mock.go -package=mocks

// LimitAssetRepository locks the current definition before association. A
// caller's tenant transaction owns that lock, the immutable write and audit.
type LimitAssetRepository interface {
	GetForAssetBindingWithTx(context.Context, pgdb.Tx, uuid.UUID) (*model.ContextAccountLimit, error)
	BindAssetWithTx(context.Context, pgdb.Tx, uuid.UUID, tracercontract.AssetRef) error
}
