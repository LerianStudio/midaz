// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// recordingMongoResolver returns a fixed database or error and records the tenant it was
// asked for, so a test can tell whether resolution ran at all.
type recordingMongoResolver struct {
	db       *mongo.Database
	err      error
	called   bool
	tenantID string
}

func (r *recordingMongoResolver) GetDatabaseForTenant(_ context.Context, tenantID string) (*mongo.Database, error) {
	r.called = true
	r.tenantID = tenantID

	return r.db, r.err
}

func TestResolveTenantMongoContext(t *testing.T) {
	t.Parallel()

	tenantDB := (&mongo.Client{}).Database("crm_tenant_a")
	otherDB := (&mongo.Client{}).Database("fees_tenant_a")

	canceled, cancel := context.WithCancel(tmcore.ContextWithTenantID(context.Background(), "tenant-a"))
	cancel()

	tests := []struct {
		name          string
		ctx           context.Context
		resolver      *recordingMongoResolver
		wantDB        *mongo.Database
		wantErrIs     error
		wantErrAs     bool
		wantErrCode   string
		wantNoResolve bool
	}{
		{
			name:     "binds the tenant database on the generic key",
			ctx:      tmcore.ContextWithTenantID(context.Background(), "tenant-a"),
			resolver: &recordingMongoResolver{db: tenantDB},
			wantDB:   tenantDB,
		},
		{
			name:     "replaces a generic database already on the context",
			ctx:      tmcore.ContextWithMB(tmcore.ContextWithTenantID(context.Background(), "tenant-a"), otherDB),
			resolver: &recordingMongoResolver{db: tenantDB},
			wantDB:   tenantDB,
		},
		{
			name:          "missing tenant id fails without resolving",
			ctx:           context.Background(),
			resolver:      &recordingMongoResolver{db: tenantDB},
			wantErrIs:     tmcore.ErrTenantNotFound,
			wantNoResolve: true,
		},
		{
			name:          "canceled context fails without resolving",
			ctx:           canceled,
			resolver:      &recordingMongoResolver{db: tenantDB},
			wantErrIs:     context.Canceled,
			wantNoResolve: true,
		},
		{
			name:        "resolution failure is mapped to the tenant service error",
			ctx:         tmcore.ContextWithTenantID(context.Background(), "tenant-a"),
			resolver:    &recordingMongoResolver{err: errors.New("tenant-manager down")},
			wantErrAs:   true,
			wantErrCode: constant.ErrTenantServiceUnavailable.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			derived, err := ResolveTenantMongoContext(tt.ctx, tt.resolver, "test seam")

			if tt.wantNoResolve {
				assert.False(t, tt.resolver.called, "resolution must not run")
			}

			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.Nil(t, derived)

				return
			}

			if tt.wantErrAs {
				var unavailable pkg.ServiceUnavailableError
				require.ErrorAs(t, err, &unavailable)
				assert.Equal(t, tt.wantErrCode, unavailable.Code)
				assert.Nil(t, derived)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "tenant-a", tt.resolver.tenantID, "resolution must use the caller's tenant")
			assert.Same(t, tt.wantDB, tmcore.GetMBContext(derived), "the tenant database must be on the generic key")
			assert.NotSame(t, tt.wantDB, tmcore.GetMBContext(tt.ctx), "the caller's context must stay untouched")
		})
	}
}

func TestMapTenantError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		assertFn func(t *testing.T, mapped error)
	}{
		{
			name: "suspended tenant is forbidden",
			err:  &tmcore.TenantSuspendedError{TenantID: "tenant-a", Status: "suspended"},
			assertFn: func(t *testing.T, mapped error) {
				var forbidden pkg.ForbiddenError
				require.ErrorAs(t, mapped, &forbidden)
				assert.Equal(t, constant.ErrTenantServiceSuspended.Error(), forbidden.Code)
			},
		},
		{
			name: "unknown tenant is not found",
			err:  tmcore.ErrTenantNotFound,
			assertFn: func(t *testing.T, mapped error) {
				var notFound pkg.EntityNotFoundError
				require.ErrorAs(t, mapped, &notFound)
				assert.Equal(t, constant.ErrTenantNotFound.Error(), notFound.Code)
			},
		},
		{
			name: "unprovisioned tenant is unprocessable",
			err:  tmcore.ErrTenantNotProvisioned,
			assertFn: func(t *testing.T, mapped error) {
				var unprocessable pkg.UnprocessableOperationError
				require.ErrorAs(t, mapped, &unprocessable)
				assert.Equal(t, constant.ErrTenantNotProvisioned.Error(), unprocessable.Code)
			},
		},
		{
			name: "any other failure is service unavailable without the cause",
			err:  errors.New("dial tcp 10.0.0.1:27017: connection refused"),
			assertFn: func(t *testing.T, mapped error) {
				var unavailable pkg.ServiceUnavailableError
				require.ErrorAs(t, mapped, &unavailable)
				assert.Equal(t, constant.ErrTenantServiceUnavailable.Error(), unavailable.Code)
				assert.NotContains(t, unavailable.Message, "10.0.0.1", "a 5xx message must not carry the internal cause")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.assertFn(t, MapTenantError(context.Background(), tt.err, "tenant-a"))
		})
	}
}
