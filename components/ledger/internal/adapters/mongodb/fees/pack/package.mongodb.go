// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	http "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	mmongoDB "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
)

// Repository provides an interface for operations related to mongo metadata entities.
//
// FindByID, Update and SoftDelete take the ledger the caller is acting within.
// uuid.Nil means organization scope and matches the package on whichever ledger
// owns it; any other value matches only a package owned by that ledger, and a
// package owned by another ledger of the same organization is reported as absent.
//
//go:generate mockgen --destination=./package_mongodb_mock.go --package=pack . Repository
type Repository interface {
	Create(ctx context.Context, pack *Package, organizationID uuid.UUID) (*Package, error)
	FindList(ctx context.Context, filters http.QueryHeader) ([]*Package, error)
	FindByID(ctx context.Context, id, organizationID, ledgerID uuid.UUID) (*Package, error)
	Update(ctx context.Context, id, organizationID, ledgerID uuid.UUID, updateFields *bson.M) (*Package, error)
	SoftDelete(ctx context.Context, id, organizationID, ledgerID uuid.UUID) error
	FindByOrganizationIDAndLedgerID(ctx context.Context, organizationID, ledgerID uuid.UUID) ([]*Package, error)
	FindFeesAndAmountDataByPackageID(ctx context.Context, organizationID, packageID uuid.UUID) (*model.AmountData, error)
}

// PackageMongoDBRepository is a MongoDD-specific implementation of the PackageRepository.
type PackageMongoDBRepository struct {
	connection *mmongoDB.MongoConnection
}

// NewPackageMongoDBRepository returns a new instance of PackageMongoDBRepository using the given MongoDB connection.
func NewPackageMongoDBRepository(mc *mmongoDB.MongoConnection, logger libLog.Logger) (*PackageMongoDBRepository, error) {
	r := &PackageMongoDBRepository{
		connection: mc,
	}
	ctx := context.Background()

	if _, err := r.connection.GetDB(ctx); err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to connect mongo", libLog.Err(err))
		return nil, err
	}

	if err := EnsureIndexes(ctx, mc); err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to ensure mongo indexes", libLog.Err(err))
		return nil, err
	}

	return r, nil
}
