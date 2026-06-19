// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"strings"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/log"
	mmongoDB "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Repository provides an interface for operations related to mongo metadata entities.
//
//go:generate mockgen --destination=./package_mongodb_mock.go --package=pack . Repository
type Repository interface {
	Create(ctx context.Context, pack *Package, organizationID uuid.UUID) (*Package, error)
	FindList(ctx context.Context, filters http.QueryHeader) ([]*Package, error)
	FindByID(ctx context.Context, id, organizationID uuid.UUID) (*Package, error)
	Update(ctx context.Context, id, organizationID uuid.UUID, updateFields *bson.M) error
	SoftDelete(ctx context.Context, id, organizationID uuid.UUID) error
	FindByOrganizationIDAndLedgerID(ctx context.Context, organizationID, ledgerID uuid.UUID) ([]*Package, error)
	FindFeesAndAmountDataByPackageID(ctx context.Context, organizationID, packageID uuid.UUID) (*model.AmountData, error)
}

// PackageMongoDBRepository is a MongoDD-specific implementation of the PackageRepository.
type PackageMongoDBRepository struct {
	connection *mmongoDB.MongoConnection
	Database   string
}

// getDatabase resolves the MongoDB database for the current request.
// Multi-tenant: returns tenant-specific database from context.
// Single-tenant: falls back to the static connection.
func (pm *PackageMongoDBRepository) getDatabase(ctx context.Context) (*mongo.Database, error) {
	if db := tmcore.GetMBContext(ctx); db != nil {
		return db, nil
	}

	client, err := pm.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	return client.Database(strings.ToLower(pm.Database)), nil
}

// NewPackageMongoDBRepository returns a new instance of PackageMongoDBRepository using the given MongoDB connection.
func NewPackageMongoDBRepository(mc *mmongoDB.MongoConnection, logger libLog.Logger) (*PackageMongoDBRepository, error) {
	r := &PackageMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
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
