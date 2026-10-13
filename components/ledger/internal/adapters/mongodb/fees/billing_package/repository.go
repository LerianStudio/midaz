// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"context"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"go.mongodb.org/mongo-driver/v2/bson"

	mmongoDB "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
)

// Repository provides an interface for operations related to billing package MongoDB entities.
//
// FindByID, Update and SoftDelete take the ledger the caller is acting within.
// AnyLedger means organization scope and matches the package on whichever ledger
// owns it; any other value matches only a package owned by that ledger, and a
// package owned by another ledger of the same organization is reported as absent.
// FindAll reads AnyLedger the same way, listing every ledger of the organization.
//
//go:generate mockgen --destination=./billing_package_mock.go --package=billing_package . Repository
type Repository interface {
	Create(ctx context.Context, bp *model.BillingPackage) (*model.BillingPackage, error)
	FindByID(ctx context.Context, id, organizationID, ledgerID string) (*model.BillingPackage, error)
	FindAll(ctx context.Context, organizationID, ledgerID, billingType string, limit, page int) ([]*model.BillingPackage, int64, error)
	Update(ctx context.Context, id, organizationID, ledgerID string, updateFields *bson.M) (*model.BillingPackage, error)
	SoftDelete(ctx context.Context, id, organizationID, ledgerID string) error
	FindMatchingPackages(ctx context.Context, orgID, ledgerID, transactionRouteID string) ([]*model.BillingPackage, error)
	FindActiveByType(ctx context.Context, orgID, ledgerID string, billingType string) ([]*model.BillingPackage, error)
	// FindNotDeletedByLedger returns every non-deleted billing package of the ledger,
	// enabled or not, without pagination. It is ledger-scoped only: AnyLedger
	// matches no package.
	FindNotDeletedByLedger(ctx context.Context, organizationID, ledgerID string) ([]*model.BillingPackage, error)
}

// BillingPackageMongoDBRepository is a MongoDB-specific implementation of the Repository.
type BillingPackageMongoDBRepository struct {
	connection *mmongoDB.MongoConnection
}

// NewBillingPackageMongoDBRepository returns a new instance of BillingPackageMongoDBRepository using the given MongoDB connection.
func NewBillingPackageMongoDBRepository(mc *mmongoDB.MongoConnection, logger libLog.Logger) (*BillingPackageMongoDBRepository, error) {
	r := &BillingPackageMongoDBRepository{
		connection: mc,
	}

	ctx := context.Background()

	if _, err := r.connection.GetDB(ctx); err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to connect mongo", libLog.Err(err))
		return nil, err
	}

	if err := EnsureIndexes(ctx, mc); err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to ensure mongo indexes for billing_package", libLog.Err(err))
		return nil, err
	}

	return r, nil
}
