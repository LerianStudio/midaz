// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-observability/log"
	libZap "github.com/LerianStudio/lib-observability/zap"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/bootstrap"
)

// @title			Midaz Ledger API
// @version		4.0.0
// @description	This is a swagger documentation for the Midaz Ledger API. This unified service combines Onboarding endpoints (organizations, ledgers, accounts, assets, portfolios, segments), Transaction endpoints (transactions, balances, operations, asset-rates), Holders and Instruments endpoints (holder and instrument account management), Fees endpoints (packages, estimates, billing), the Holder-Account composition endpoint, and Metadata Index endpoints in a single service.
// @tag.name			Organizations
// @tag.description		Top-level tenant entities that own ledgers and all nested resources.
// @tag.name			Ledgers
// @tag.description		Bookkeeping containers scoping assets, accounts, and transactions.
// @tag.name			Accounts
// @tag.description		Balance-holding entries that transactions debit and credit.
// @tag.name			Assets
// @tag.description		Currencies and instruments tracked within a ledger.
// @tag.name			Portfolios
// @tag.description		Logical groupings of accounts under a holder.
// @tag.name			Segments
// @tag.description		Sub-classifications for organizing accounts.
// @tag.name			Account Types
// @tag.description		Reusable account classification definitions.
// @tag.name			Transactions
// @tag.description		Double-entry postings that move value between accounts.
// @tag.name			Balances
// @tag.description		Per-account available, on-hold, and scale state.
// @tag.name			Operations
// @tag.description		The individual debit and credit legs of a transaction.
// @tag.name			Asset Rates
// @tag.description		Conversion rates between assets.
// @tag.name			Operation Routes
// @tag.description		Templates constraining which accounts an operation leg may touch.
// @tag.name			Transaction Routes
// @tag.description		Ordered sets of operation routes describing a transaction shape.
// @tag.name			Metadata Indexes
// @tag.description		Index definitions over entity metadata for queryable lookups.
// @tag.name			Holders
// @tag.description		Holder identity records (the party behind accounts).
// @tag.name			Instruments
// @tag.description		Financial instrument records bound to holders.
// @tag.name			Composition
// @tag.description		Holder + account creation in a single call.
// @tag.name			Packages
// @tag.description		Fee package definitions applied during transaction processing.
// @tag.name			Fees
// @tag.description		Fee estimation and application for transactions.
// @tag.name			Billing Packages
// @tag.description		Billing package definitions for charge aggregation.
// @tag.name			Billing Calculate
// @tag.description		On-demand billing charge calculation.
// @termsOfService	https://www.elastic.co/licensing/elastic-license
// @contact.name	Discord community
// @contact.url	https://discord.gg/DnhqKwkGv3
// @license.name	Elastic License 2.0
// @license.url	https://www.elastic.co/licensing/elastic-license
// @host			localhost:3002
// @BasePath		/
// @schemes		http https
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Bearer token authentication. Format: 'Bearer {access_token}'. Only required when auth plugin is enabled.
func main() {
	libCommons.InitLocalEnvConfig()

	logLevel := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if logLevel == "" {
		logLevel = "info"
	}

	envName := strings.ToLower(strings.TrimSpace(os.Getenv("ENV_NAME")))
	if envName == "" {
		envName = "development"
	}

	otelServiceName := os.Getenv("OTEL_RESOURCE_SERVICE_NAME")
	if otelServiceName == "" {
		otelServiceName = "ledger"
	}

	logger, err := libZap.New(libZap.Config{
		Environment:     libZap.Environment(envName),
		Level:           logLevel,
		OTelLibraryName: otelServiceName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)

		os.Exit(1)
	}

	service, err := bootstrap.InitServersWithOptions(&bootstrap.Options{
		Logger: logger,
	})
	if err != nil {
		logger.Log(context.Background(), libLog.LevelError, "Failed to initialize ledger service", libLog.Err(err))
		_ = logger.Sync(context.Background())

		os.Exit(1)
	}

	service.Run()

	_ = logger.Sync(context.Background())
}
