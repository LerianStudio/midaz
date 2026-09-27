//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

func TestIntegration_HolderRepo_FinancialFigures(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	organizationID := "org-figures-" + uuid.New().String()[:8]
	repo := createRepository(t, container, organizationID)
	ctx := context.Background()
	updatedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	t.Run("natural person income is encrypted at rest and survives an update", func(t *testing.T) {
		person := mongotestutil.CreateTestHolderWithNaturalPerson(t, "Jane Doe", "11122233344")
		person.NaturalPerson.MonthlyGrossIncome = monetaryAmount("12500.50", "BRL", "2026-06-30")

		_, err := repo.Create(ctx, organizationID, person)
		require.NoError(t, err)

		reloaded, err := repo.Find(ctx, organizationID, *person.ID, false)
		require.NoError(t, err)
		assertMonetaryAmountEqual(t, person.NaturalPerson.MonthlyGrossIncome, reloaded.NaturalPerson.MonthlyGrossIncome)

		var raw struct {
			NaturalPerson struct {
				MonthlyGrossIncome MonetaryAmountMongoDBModel `bson:"monthly_gross_income"`
			} `bson:"natural_person"`
		}

		collection := container.Database.Collection(strings.ToLower("holders_" + organizationID))
		require.NoError(t, collection.FindOne(ctx, bson.M{"_id": person.ID}).Decode(&raw))

		stored := raw.NaturalPerson.MonthlyGrossIncome
		_, parseErr := decimal.NewFromString(stored.Value)
		require.NotEmpty(t, stored.Value)
		assert.Error(t, parseErr, "the income value must not be stored as a plaintext decimal")
		assert.Equal(t, "BRL", stored.Currency)
		assert.Equal(t, "2026-06-30", stored.ReferenceDate)

		raised := monetaryAmount("13000", "BRL", "2026-07-31")
		_, err = repo.Update(ctx, organizationID, *person.ID, &mmodel.Holder{
			NaturalPerson: &mmodel.NaturalPerson{MonthlyGrossIncome: raised},
			UpdatedAt:     updatedAt,
		}, nil)
		require.NoError(t, err)

		reloaded, err = repo.Find(ctx, organizationID, *person.ID, false)
		require.NoError(t, err)
		assertMonetaryAmountEqual(t, raised, reloaded.NaturalPerson.MonthlyGrossIncome)
		assert.Equal(t, *person.NaturalPerson.MotherName, *reloaded.NaturalPerson.MotherName)
	})

	t.Run("legal person figures round-trip and one-figure update keeps the other", func(t *testing.T) {
		company := mongotestutil.CreateTestHolderWithLegalPerson(t, "Acme SA", "12345678000199")
		company.LegalPerson.AnnualGrossRevenue = monetaryAmount("4800000.00", "BRL", "2025-12-31")
		company.LegalPerson.TotalAssets = monetaryAmount("240000000", "BRL", "2025-12-31")

		_, err := repo.Create(ctx, organizationID, company)
		require.NoError(t, err)

		reloaded, err := repo.Find(ctx, organizationID, *company.ID, false)
		require.NoError(t, err)
		assertMonetaryAmountEqual(t, company.LegalPerson.AnnualGrossRevenue, reloaded.LegalPerson.AnnualGrossRevenue)
		assertMonetaryAmountEqual(t, company.LegalPerson.TotalAssets, reloaded.LegalPerson.TotalAssets)

		revenue := monetaryAmount("5100000.75", "BRL", "2026-06-30")
		_, err = repo.Update(ctx, organizationID, *company.ID, &mmodel.Holder{
			LegalPerson: &mmodel.LegalPerson{AnnualGrossRevenue: revenue},
			UpdatedAt:   updatedAt,
		}, nil)
		require.NoError(t, err)

		reloaded, err = repo.Find(ctx, organizationID, *company.ID, false)
		require.NoError(t, err)
		assertMonetaryAmountEqual(t, revenue, reloaded.LegalPerson.AnnualGrossRevenue)
		assertMonetaryAmountEqual(t, company.LegalPerson.TotalAssets, reloaded.LegalPerson.TotalAssets)
		assert.Equal(t, company.LegalPerson.TradeName, reloaded.LegalPerson.TradeName)
	})
}

func monetaryAmount(value, currency, referenceDate string) *mmodel.MonetaryAmount {
	amount := decimal.RequireFromString(value)

	return &mmodel.MonetaryAmount{Value: &amount, Currency: currency, ReferenceDate: referenceDate}
}

func assertMonetaryAmountEqual(t *testing.T, want, got *mmodel.MonetaryAmount) {
	t.Helper()

	require.NotNil(t, got)
	require.NotNil(t, got.Value)
	assert.True(t, want.Value.Equal(*got.Value), "value: want %s, got %s", want.Value, got.Value)
	assert.Equal(t, want.Currency, got.Currency)
	assert.Equal(t, want.ReferenceDate, got.ReferenceDate)
}
