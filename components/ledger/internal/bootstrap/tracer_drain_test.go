// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDisabledTracerRequiresDrainProofForEveryTenant(t *testing.T) {
	for _, scenario := range []string{"empty", "unmigrated", "pending", "query failure", "catalog failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			catalog, resolver := NewMocktracerRecoveryCatalog(ctrl), NewMocktracerRecoveryPoolResolver(ctrl)
			deps := contextTracerDependencies{catalog: catalog, resolver: resolver, service: "ledger"}
			var catalogErr error
			if scenario == "catalog failure" {
				catalogErr = errors.New("catalog unavailable")
			}
			catalog.EXPECT().GetActiveTenantsByService(gomock.Any(), "ledger").Return([]*tmclient.TenantSummary{{ID: "tenant-a", Status: "active"}, {ID: "tenant-b", Status: "active"}}, catalogErr)
			if catalogErr == nil {
				for _, tenant := range []string{"tenant-a", "tenant-b"} {
					db, mock, err := sqlmock.New()
					require.NoError(t, err)
					t.Cleanup(func() { _ = db.Close() })
					resolver.EXPECT().GetDB(gomock.Any(), tenant).Return(dbresolver.New(dbresolver.WithPrimaryDBs(db)), nil)
					mock.ExpectBegin()
					exists := scenario != "unmigrated"
					mock.ExpectQuery("SELECT to_regclass").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
					if exists {
						query := mock.ExpectQuery("SELECT EXISTS")
						if tenant == "tenant-b" && scenario == "query failure" {
							query.WillReturnError(errors.New("database unavailable"))
						} else {
							query.WillReturnRows(sqlmock.NewRows([]string{"pending"}).AddRow(tenant == "tenant-b" && scenario == "pending"))
						}
					}
					mock.ExpectRollback()
					t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
				}
			}
			runtime, err := buildContextTracer(&Config{MultiTenantEnabled: true, TracerIntegrationID: "producer"}, deps)
			require.Nil(t, runtime)
			if scenario == "empty" || scenario == "unmigrated" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestDisabledTracerWithoutSharedIdentityDoesNotInspectTenants(t *testing.T) {
	for _, cfg := range []*Config{
		{MultiTenantEnabled: true},
		{MultiTenantEnabled: true, TracerBaseURL: "http://legacy-tracer:4020", TracerTransport: "rest"},
	} {
		runtime, err := buildContextTracer(cfg, contextTracerDependencies{})
		require.NoError(t, err)
		require.Nil(t, runtime)
	}
}

func TestDisabledTracerDrainMatchesWorkerTenantEligibility(t *testing.T) {
	ctrl := gomock.NewController(t)
	catalog, resolver := NewMocktracerRecoveryCatalog(ctrl), NewMocktracerRecoveryPoolResolver(ctrl)
	deps := contextTracerDependencies{catalog: catalog, resolver: resolver, service: "ledger"}
	catalog.EXPECT().GetActiveTenantsByService(gomock.Any(), "ledger").Return([]*tmclient.TenantSummary{
		{ID: "tenant-upper", Status: "ACTIVE"},
		{ID: "invalid tenant", Status: "active"},
		{ID: "tenant-inactive", Status: "suspended"},
	}, nil)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	resolver.EXPECT().GetDB(gomock.Any(), "tenant-upper").Return(dbresolver.New(dbresolver.WithPrimaryDBs(db)), nil)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT to_regclass").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"pending"}).AddRow(false))
	mock.ExpectRollback()
	runtime, err := buildContextTracer(&Config{MultiTenantEnabled: true, TracerIntegrationID: "producer"}, deps)
	require.NoError(t, err)
	require.Nil(t, runtime)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDisabledTracerDrainSkipsTenantWithoutTransactionService(t *testing.T) {
	ctrl := gomock.NewController(t)
	catalog, resolver := NewMocktracerRecoveryCatalog(ctrl), NewMocktracerRecoveryPoolResolver(ctrl)
	deps := contextTracerDependencies{catalog: catalog, resolver: resolver, service: "ledger"}
	catalog.EXPECT().GetActiveTenantsByService(gomock.Any(), "ledger").Return([]*tmclient.TenantSummary{{ID: "tenant-a", Status: "active"}}, nil)
	resolver.EXPECT().GetDB(gomock.Any(), "tenant-a").Return(nil, tmcore.ErrServiceNotConfigured)
	runtime, err := buildContextTracer(&Config{MultiTenantEnabled: true, TracerAssetNamespace: "origin-a"}, deps)
	require.NoError(t, err)
	require.Nil(t, runtime)
}
