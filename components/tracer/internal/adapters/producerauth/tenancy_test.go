// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package producerauth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestServiceTenancy_Applies(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		lookupErr error
		applies   bool
		want      error
	}{
		"associated tenant":          {applies: true},
		"tenant not associated":      {lookupErr: fmt.Errorf("not active: %w", tmcore.ErrTenantNotFound)},
		"association denied":         {lookupErr: fmt.Errorf("denied: %w", tmcore.ErrTenantServiceAccessDenied)},
		"tenant-manager unavailable": {lookupErr: errors.New("list unavailable"), want: constant.ErrTenantServiceUnavailable},
		"caller went away":           {lookupErr: context.Canceled, want: context.Canceled},
		"deadline passed":            {lookupErr: context.DeadlineExceeded, want: context.DeadlineExceeded},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var asked []string

			tenancy := NewServiceTenancy(func(_ context.Context, tenantID, service string) error {
				asked = append(asked, service+":"+tenantID)

				return tc.lookupErr
			}, ServiceLedger)

			applies, err := tenancy.Applies(tmcore.ContextWithTenantID(t.Context(), "tenant-a"))
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, tc.applies, applies)
			require.Equal(t, []string{"ledger:tenant-a"}, asked)
		})
	}
}

func TestServiceTenancy_EdgeCases(t *testing.T) {
	t.Parallel()

	called := false
	tenancy := NewServiceTenancy(func(context.Context, string, string) error { called = true; return nil }, ServiceLedger)

	applies, err := tenancy.Applies(t.Context())
	require.NoError(t, err)
	require.True(t, applies, "a request without a tenant gets the stricter rule")
	require.False(t, called)

	ctx := tmcore.ContextWithTenantID(t.Context(), "tenant-a")

	for name, unconfigured := range map[string]*ServiceTenancy{"nil tenancy": nil, "nil lookup": NewServiceTenancy(nil, ServiceLedger)} {
		applies, err := unconfigured.Applies(ctx)
		require.ErrorIs(t, err, constant.ErrTenantServiceUnavailable, name)
		require.False(t, applies, name)
	}
}

func TestRegistry_HasClientIDMappings(t *testing.T) {
	t.Parallel()

	var none *Registry
	require.False(t, none.HasClientIDMappings())

	certificates, err := ParsePlatformProducers(`[{"service":"ledger","certUri":"spiffe://example.test/service/ledger"}]`)
	require.NoError(t, err)
	require.False(t, certificates.HasClientIDMappings())

	tokens, err := ParsePlatformProducers(`[{"service":"ledger","clientId":"ledger-m2m-client"}]`)
	require.NoError(t, err)
	require.True(t, tokens.HasClientIDMappings())
}
