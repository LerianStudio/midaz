// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"

	authdecl "github.com/LerianStudio/lib-auth/v3/auth/declaration"
	tmmongo "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/mongo"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libStreaming "github.com/LerianStudio/lib-streaming/v2"

	feesservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
	feesdecl "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees/declaration"
	feesmidaz "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees/midaz"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
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
func initFees(feeMongo *feesMongoComponents, queryUC *query.UseCase, cfg *Config, logger libLog.Logger, streamingEmitter libStreaming.Emitter) (*feesComponents, error) {
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

	// Share the ledger emitter so fee services emit past-tense events.
	useCase.Streaming = streamingEmitter
	billingPackageService.Streaming = streamingEmitter

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

// initFeesDeclaration wires the fees component's D7 permissions declaration
// publisher via lib-auth WireFromEnv (the SLIM single-call pattern). Fees is the
// SOLE declarant inside the unified ledger binary — the ledger core does not
// declare — so the FIXED, un-prefixed env contract WireFromEnv reads internally
// (DECLARATION_ENABLED / PLUGIN_IDENTITY_HOST / M2M_CLIENT_ID / M2M_CLIENT_SECRET)
// unambiguously belongs to fees, with no collision against any ledger env tag.
//
// The caller passes ONLY the fee slug + embedded manifest (+ logger); everything
// operational (identity host, M2M creds, auth host) comes from the shared,
// deployment-owned environment. The slug is constant.ModuleFees so it stays
// locked to both the manifest's `service:` value (lib-auth's New enforces
// slug==service via BOLA) and the tenant-manager provisioning name.
//
// Default OFF and fail-open: with DECLARATION_ENABLED unset this returns a
// non-nil no-op stop and starts no goroutine, leaving the boot path unchanged.
func initFeesDeclaration(ctx context.Context, logger libLog.Logger) (func(), error) {
	return authdecl.WireFromEnv(ctx, authdecl.WireInput{
		Slug:     constant.ModuleFees,
		Manifest: feesdecl.Manifest,
		Logger:   logger,
	})
}
