// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"context"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/log"
	mmongoDB "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Repository provides an interface for operations related to billing package MongoDB entities.
//
//go:generate mockgen --destination=./billing_package_mock.go --package=billing_package . Repository
type Repository interface {
	Create(ctx context.Context, bp *model.BillingPackage) (*model.BillingPackage, error)
	FindByID(ctx context.Context, id string, organizationID string) (*model.BillingPackage, error)
	FindAll(ctx context.Context, organizationID, ledgerID, billingType string, limit, page int) ([]*model.BillingPackage, int64, error)
	Update(ctx context.Context, id string, organizationID string, updateFields *bson.M) error
	SoftDelete(ctx context.Context, id string, organizationID string) error
	FindMatchingPackages(ctx context.Context, orgID, ledgerID, transactionRouteID string) ([]*model.BillingPackage, error)
	FindActiveByType(ctx context.Context, orgID, ledgerID string, billingType string) ([]*model.BillingPackage, error)
}

// BillingPackageMongoDBRepository is a MongoDB-specific implementation of the Repository.
type BillingPackageMongoDBRepository struct {
	connection *mmongoDB.MongoConnection
	Database   string
}

// getDatabase resolves the MongoDB database for the current request.
// Multi-tenant: returns tenant-specific database from context.
// Single-tenant: falls back to the static connection.
func (r *BillingPackageMongoDBRepository) getDatabase(ctx context.Context) (*mongo.Database, error) {
	if db := tmcore.GetMBContext(ctx); db != nil {
		return db, nil
	}

	client, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	return client.Database(strings.ToLower(r.Database)), nil
}

// NewBillingPackageMongoDBRepository returns a new instance of BillingPackageMongoDBRepository using the given MongoDB connection.
func NewBillingPackageMongoDBRepository(mc *mmongoDB.MongoConnection, logger libLog.Logger) (*BillingPackageMongoDBRepository, error) {
	r := &BillingPackageMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}

	ctx := context.Background()

	if _, err := r.connection.GetDB(ctx); err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to connect mongo, Err: %v", err))
		return nil, err
	}

	if err := EnsureIndexes(ctx, mc); err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to ensure mongo indexes for billing_package, Err: %v", err))
		return nil, err
	}

	return r, nil
}

// NewBillingPackageMongoDBRepositoryFromConnection creates a BillingPackageMongoDBRepository
// directly from an already-connected MongoConnection, without calling GetDB or EnsureIndexes.
// This is intended for integration tests where the caller manages connection and index setup.
func NewBillingPackageMongoDBRepositoryFromConnection(mc *mmongoDB.MongoConnection) *BillingPackageMongoDBRepository {
	return &BillingPackageMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}
}
