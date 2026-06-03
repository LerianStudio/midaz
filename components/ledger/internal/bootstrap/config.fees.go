// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"

	libLog "github.com/LerianStudio/lib-observability/log"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	feesservices "github.com/LerianStudio/midaz/v3/components/ledger/internal/services/fees"
	feesmidaz "github.com/LerianStudio/midaz/v3/components/ledger/internal/services/fees/midaz"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/query"
)

// feesComponents holds the fee/billing slice of the unified ledger binary: the
// fee package use case, the billing-package CRUD service, and the
// billing-calculate service. They share a single in-process MidazResolver backed
// by the ledger query.UseCase (Chunk B) so account/segment/count reads no longer
// cross the network. The Mongo manager is carried for route-scoped tenant
// middleware and eviction wiring (mirrors crmComponents.mongoManager).
type feesComponents struct {
	useCase                 *feesservices.UseCase
	billingPackageService   *feesservices.BillingPackageService
	billingCalculateService *feesservices.BillingCalculateService
	mongoManager            *tmmongo.Manager // nil in single-tenant mode
}

// initFees wires the fee/billing use cases from the already-initialized fee
// Mongo slice and ledger query.UseCase. It is the fee analogue of the
// initOnboardingMongo / initCRM extraction discipline: the composition root
// delegates fee construction here so InitServersWithOptions stays reviewable,
// and the command/query god-structs are NOT extended with fee fields.
//
// The resolver is constructed ONCE here and shared by every fee service so all
// fee reads route through the same in-process query.UseCase.
func initFees(feeMongo *feesMongoComponents, queryUC *query.UseCase, cfg *Config, logger libLog.Logger) (*feesComponents, error) {
	if feeMongo == nil {
		return nil, fmt.Errorf("fee Mongo components are required for fee initialization")
	}

	if queryUC == nil {
		return nil, fmt.Errorf("query use case is required for fee initialization")
	}

	resolver, err := feesservices.NewQueryResolver(queryUC)
	if err != nil {
		return nil, fmt.Errorf("failed to build fee Midaz resolver: %w", err)
	}

	useCase, err := feesservices.NewUseCase(feeMongo.packageRepo, resolver, cfg.FeesDefaultCurrency)
	if err != nil {
		return nil, fmt.Errorf("failed to build fee use case: %w", err)
	}

	billingPackageService, err := feesservices.NewBillingPackageService(feeMongo.billingPackageRepo, resolver)
	if err != nil {
		return nil, fmt.Errorf("failed to build billing package service: %w", err)
	}

	// The billing-calculate path consumes the narrower midaz.AccountResolver /
	// midaz.TransactionCounter ports; both adapt the same shared MidazResolver.
	accountResolver, err := feesmidaz.NewAccountResolver(resolver)
	if err != nil {
		return nil, fmt.Errorf("failed to build fee account resolver: %w", err)
	}

	transactionCounter, err := feesmidaz.NewTransactionCounter(resolver)
	if err != nil {
		return nil, fmt.Errorf("failed to build fee transaction counter: %w", err)
	}

	billingCalculateService, err := feesservices.NewBillingCalculateService(feeMongo.billingPackageRepo, transactionCounter, accountResolver)
	if err != nil {
		return nil, fmt.Errorf("failed to build billing calculate service: %w", err)
	}

	logger.Log(context.Background(), libLog.LevelInfo, "Fee use cases initialized")

	return &feesComponents{
		useCase:                 useCase,
		billingPackageService:   billingPackageService,
		billingCalculateService: billingCalculateService,
		mongoManager:            feeMongo.mongoManager,
	}, nil
}
