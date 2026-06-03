// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"reflect"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/billing_package"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newFeeMongoSlice builds a feesMongoComponents with mock repos (no real Mongo).
// initFees only holds the repos behind use cases — it does not touch them — so
// bare mocks are sufficient to prove the wiring is sound.
func newFeeMongoSlice(t *testing.T) *feesMongoComponents {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	return &feesMongoComponents{
		packageRepo:        pack.NewMockRepository(ctrl),
		billingPackageRepo: billing_package.NewMockRepository(ctrl),
	}
}

// TestInitFees_ConstructsReachableUseCases proves the composition root can build
// the fee use cases from the fee Mongo slice + the ledger query.UseCase, and
// that the resulting fees.UseCase / billing services are reachable and non-nil.
// This is the P4-T09 bootstrap wiring acceptance: a non-nil fees.UseCase the
// transaction handler and fee CRUD handler can later consume.
func TestInitFees_ConstructsReachableUseCases(t *testing.T) {
	t.Parallel()

	logger := &libLog.GoLogger{}
	cfg := &Config{FeesDefaultCurrency: "USD"}

	fees, err := initFees(newFeeMongoSlice(t), &query.UseCase{}, cfg, logger)
	require.NoError(t, err, "initFees must succeed with valid dependencies")
	require.NotNil(t, fees, "fees components must be non-nil")

	require.NotNil(t, fees.useCase, "fee package use case must be reachable")
	assert.Equal(t, "USD", fees.useCase.DefaultCurrency(),
		"fee use case must carry the configured default currency")
	require.NotNil(t, fees.useCase.PackageRepo(), "fee use case must hold the package repo")
	require.NotNil(t, fees.useCase.Resolver(), "fee use case must hold the in-process resolver")

	require.NotNil(t, fees.billingPackageService, "billing package service must be reachable")
	require.NotNil(t, fees.billingCalculateService, "billing calculate service must be reachable")
}

// TestInitFees_RejectsNilDependencies asserts initFees fails fast on missing
// dependencies rather than constructing a half-wired fee slice.
func TestInitFees_RejectsNilDependencies(t *testing.T) {
	t.Parallel()

	logger := &libLog.GoLogger{}
	cfg := &Config{FeesDefaultCurrency: "USD"}

	t.Run("nil_fee_mongo", func(t *testing.T) {
		t.Parallel()

		_, err := initFees(nil, &query.UseCase{}, cfg, logger)
		require.Error(t, err, "initFees must reject a nil fee Mongo slice")
	})

	t.Run("nil_query_use_case", func(t *testing.T) {
		t.Parallel()

		_, err := initFees(newFeeMongoSlice(t), nil, cfg, logger)
		require.Error(t, err, "initFees must reject a nil query use case")
	})
}

// TestInitFees_EmptyDefaultCurrencyRejected proves the fee use case construction
// enforces a non-empty currency. applyConfigDefaults guarantees "USD" at boot,
// but initFees must not silently accept an empty value if a caller bypasses it.
func TestInitFees_EmptyDefaultCurrencyRejected(t *testing.T) {
	t.Parallel()

	logger := &libLog.GoLogger{}
	cfg := &Config{FeesDefaultCurrency: ""}

	_, err := initFees(newFeeMongoSlice(t), &query.UseCase{}, cfg, logger)
	require.Error(t, err, "initFees must reject an empty default currency")
}

// TestFeesConfigFields_PresentWithCorrectTags locks the merged fee config
// surface (P4-T17/T20): the FeesPrefixed* Mongo block + DEFAULT_CURRENCY must
// exist with the exact env tags, or the merged binary silently fails to load
// fee config at runtime (R17).
func TestFeesConfigFields_PresentWithCorrectTags(t *testing.T) {
	t.Parallel()

	expectedFields := map[string]string{
		"FeesPrefixedMongoURI":          "MONGO_FEES_URI",
		"FeesPrefixedMongoDBHost":       "MONGO_FEES_HOST",
		"FeesPrefixedMongoDBName":       "MONGO_FEES_NAME",
		"FeesPrefixedMongoDBUser":       "MONGO_FEES_USER",
		"FeesPrefixedMongoDBPassword":   "MONGO_FEES_PASSWORD",
		"FeesPrefixedMongoDBPort":       "MONGO_FEES_PORT",
		"FeesPrefixedMongoDBParameters": "MONGO_FEES_PARAMETERS",
		"FeesPrefixedMaxPoolSize":       "MONGO_FEES_MAX_POOL_SIZE",
		"FeesPrefixedMongoTLSCACert":    "MONGO_FEES_TLS_CA_CERT",
		"FeesDefaultCurrency":           "DEFAULT_CURRENCY",
	}

	for fieldName, expectedTag := range expectedFields {
		field, found := reflect.TypeOf(Config{}).FieldByName(fieldName)
		require.True(t, found, "Config must have fee field %s", fieldName)
		assert.Equal(t, expectedTag, field.Tag.Get("env"),
			"field %s must have env tag %q", fieldName, expectedTag)
	}
}

// TestFeesDefaultCurrencyDefault asserts applyConfigDefaults fills DEFAULT_CURRENCY
// with USD when unset — the standalone fees service required the env, the merged
// binary must not fail fee construction on an empty value.
func TestFeesDefaultCurrencyDefault(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	applyConfigDefaults(cfg)
	assert.Equal(t, "USD", cfg.FeesDefaultCurrency,
		"DEFAULT_CURRENCY must default to USD when unset")

	explicit := &Config{FeesDefaultCurrency: "BRL"}
	applyConfigDefaults(explicit)
	assert.Equal(t, "BRL", explicit.FeesDefaultCurrency,
		"an explicit DEFAULT_CURRENCY must not be overridden")
}

// TestModuleFeesProvisioningName locks the tenant-manager / auth namespace value.
// The standalone fees service registered under "plugin-fees" (auth namespace +
// single-module tenant service name). Renaming this breaks RBAC and tenant DB
// resolution for already-provisioned tenants (the CRM crm->crm-api footgun).
func TestModuleFeesProvisioningName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "plugin-fees", constant.ModuleFees,
		"ModuleFees MUST equal 'plugin-fees' to match tenant-manager provisioning + RBAC (R9, P4-T22)")
}
