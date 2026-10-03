//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestTransactionValidationRepository_ListConfinedToTheScope_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := NewTransactionValidationRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})

	accountA, accountB := uuid.New(), uuid.New()
	segment, merchant := uuid.New(), uuid.New()
	at := time.Now().UTC().Add(-time.Hour)

	seed := func(account uuid.UUID, segmentJSON, merchantJSON any) uuid.UUID {
		t.Helper()

		id := uuid.New()
		_, err := db.Exec(`
			INSERT INTO transaction_validations
				(id, request_id, transaction_type, amount, asset, transaction_timestamp,
				 account, segment, merchant, decision, matched_rule_ids, evaluated_rule_ids,
				 processing_time_ms, reason, created_at)
			VALUES ($1,$2,'CARD',10,'USD',$3,$4,$5,$6,'ALLOW','{}','{}',1,'allowed',$3)`,
			id, uuid.New(), at, fmt.Sprintf(`{"accountId":%q,"type":"CHECKING"}`, account.String()), segmentJSON, merchantJSON)
		require.NoError(t, err)

		return id
	}

	a := seed(accountA, fmt.Sprintf(`{"segmentId":%q}`, segment.String()), nil)
	b := seed(accountB, nil, fmt.Sprintf(`{"merchantId":%q}`, merchant.String()))

	tests := []struct {
		name  string
		scope http.ScopeConfinement
		want  []uuid.UUID
	}{
		{name: "allowed account", scope: http.ScopeConfinement{"accountId": {accountA}}, want: []uuid.UUID{a}},
		{name: "allowed segment", scope: http.ScopeConfinement{"segmentId": {segment}}, want: []uuid.UUID{a}},
		{name: "allowed merchant", scope: http.ScopeConfinement{"merchantId": {merchant}}, want: []uuid.UUID{b}},
		{name: "an empty allowed list lists nothing", scope: http.ScopeConfinement{"accountId": {}}, want: []uuid.UUID{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.List(context.Background(), &model.TransactionValidationFilters{
				StartDate: at.Add(-time.Minute), EndDate: at.Add(time.Minute), Limit: 100, Scope: tt.scope,
			})
			require.NoError(t, err)

			got := make([]uuid.UUID, 0, len(result.TransactionValidations))
			for _, v := range result.TransactionValidations {
				got = append(got, v.ID)
			}

			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
