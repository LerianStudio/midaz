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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

func TestIntegration_HolderRepo_FinancialFigures(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	organizationID := "org-figures-" + uuid.New().String()[:8]
	repo := createRepository(t, container, organizationID)
	ctx := context.Background()
	updatedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	// mergePatch runs an RFC 7396 body through the handler's decode and null-path derivation.
	mergePatch := func(t *testing.T, id uuid.UUID, body string) *mmodel.Holder {
		t.Helper()

		var input mmodel.UpdateHolderInput

		originalMap, err := http.DecodeAndValidate([]byte(body), &input)
		require.NoError(t, err)

		_, err = repo.Update(ctx, organizationID, id, &mmodel.Holder{
			NaturalPerson: input.NaturalPerson,
			LegalPerson:   input.LegalPerson,
			UpdatedAt:     updatedAt,
		}, http.FindNilFields(originalMap, ""))
		require.NoError(t, err)

		reloaded, err := repo.Find(ctx, organizationID, id, false)
		require.NoError(t, err)

		return reloaded
	}

	t.Run("natural person income is encrypted at rest, updates and is removed with null", func(t *testing.T) {
		person := mongotestutil.CreateTestHolderWithNaturalPerson(t, "Jane Doe", "11122233344")
		person.NaturalPerson.MonthlyGrossIncome = monetaryAmount("12500.50", "BRL", "2026-06-30")

		_, err := repo.Create(ctx, organizationID, person)
		require.NoError(t, err)

		reloaded, err := repo.Find(ctx, organizationID, *person.ID, false)
		require.NoError(t, err)
		assert.Equal(t, person.NaturalPerson.MonthlyGrossIncome, reloaded.NaturalPerson.MonthlyGrossIncome)

		var raw struct {
			NaturalPerson struct {
				MonthlyGrossIncome MonetaryAmountMongoDBModel `bson:"monthly_gross_income"`
			} `bson:"natural_person"`
		}

		collection := container.Database.Collection(strings.ToLower("holders_" + organizationID))
		require.NoError(t, collection.FindOne(ctx, bson.M{"_id": person.ID}).Decode(&raw))

		stored := raw.NaturalPerson.MonthlyGrossIncome
		require.NotEmpty(t, stored.Value)
		assert.NotContains(t, stored.Value, "12500", "the income value must not be stored in plaintext")
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
		assert.Equal(t, raised, reloaded.NaturalPerson.MonthlyGrossIncome)
		assert.Equal(t, *person.NaturalPerson.MotherName, *reloaded.NaturalPerson.MotherName)

		reloaded = mergePatch(t, *person.ID, `{"naturalPerson":{"monthlyGrossIncome":null}}`)
		assert.Nil(t, reloaded.NaturalPerson.MonthlyGrossIncome)
		assert.Equal(t, *person.NaturalPerson.MotherName, *reloaded.NaturalPerson.MotherName)
	})

	t.Run("legal person figures round-trip verbatim and one-figure changes keep the other", func(t *testing.T) {
		company := mongotestutil.CreateTestHolderWithLegalPerson(t, "Acme SA", "12345678000199")
		company.LegalPerson.AnnualGrossRevenue = monetaryAmount("4800000.00", "BRL", "2025-12-31")
		company.LegalPerson.TotalAssets = monetaryAmount("0", "BRL", "2025-12-31")

		_, err := repo.Create(ctx, organizationID, company)
		require.NoError(t, err)

		reloaded, err := repo.Find(ctx, organizationID, *company.ID, false)
		require.NoError(t, err)
		assert.Equal(t, company.LegalPerson.AnnualGrossRevenue, reloaded.LegalPerson.AnnualGrossRevenue)
		assert.Equal(t, company.LegalPerson.TotalAssets, reloaded.LegalPerson.TotalAssets)

		revenue := monetaryAmount("5100000.75", "BRL", "2026-06-30")
		_, err = repo.Update(ctx, organizationID, *company.ID, &mmodel.Holder{
			LegalPerson: &mmodel.LegalPerson{AnnualGrossRevenue: revenue},
			UpdatedAt:   updatedAt,
		}, nil)
		require.NoError(t, err)

		reloaded, err = repo.Find(ctx, organizationID, *company.ID, false)
		require.NoError(t, err)
		assert.Equal(t, revenue, reloaded.LegalPerson.AnnualGrossRevenue)
		assert.Equal(t, company.LegalPerson.TotalAssets, reloaded.LegalPerson.TotalAssets)
		assert.Equal(t, company.LegalPerson.TradeName, reloaded.LegalPerson.TradeName)

		reloaded = mergePatch(t, *company.ID, `{"legalPerson":{"totalAssets":null}}`)
		assert.Nil(t, reloaded.LegalPerson.TotalAssets)
		assert.Equal(t, revenue, reloaded.LegalPerson.AnnualGrossRevenue)
		assert.Equal(t, company.LegalPerson.TradeName, reloaded.LegalPerson.TradeName)
	})
}

func monetaryAmount(value, currency, referenceDate string) *mmodel.MonetaryAmount {
	return &mmodel.MonetaryAmount{Value: value, Currency: currency, ReferenceDate: referenceDate}
}
