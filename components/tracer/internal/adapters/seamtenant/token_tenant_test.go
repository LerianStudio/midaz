// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seamtenant

import (
	"context"
	"errors"
	"fmt"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestClassifyTenantDBRefusal(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"missing token":         {err: tmcore.ErrAuthorizationTokenRequired, want: constant.ErrInsufficientPrivileges},
		"invalid token":         {err: tmcore.ErrInvalidAuthorizationToken, want: constant.ErrInsufficientPrivileges},
		"invalid tenant claims": {err: tmcore.ErrInvalidTenantClaims, want: constant.ErrInsufficientPrivileges},
		"missing tenant claim":  {err: tmcore.ErrMissingTenantIDClaim, want: constant.ErrInsufficientPrivileges},
		"tenant not found":      {err: fmt.Errorf("pg: %w", tmcore.ErrTenantNotFound), want: constant.ErrInsufficientPrivileges},
		"access denied":         {err: fmt.Errorf("pg: %w", tmcore.ErrTenantServiceAccessDenied), want: constant.ErrInsufficientPrivileges},
		"suspended":             {err: &tmcore.TenantSuspendedError{TenantID: "t", Status: "suspended"}, want: constant.ErrInsufficientPrivileges},
		"circuit breaker":       {err: tmcore.ErrCircuitBreakerOpen, want: constant.ErrTenantServiceUnavailable},
		"connection failed":     {err: tmcore.ErrConnectionFailed, want: constant.ErrTenantServiceUnavailable},
		"unknown failure":       {err: errors.New("boom"), want: constant.ErrTenantServiceUnavailable},
		"wrapped cancellation":  {err: fmt.Errorf("pg: %w", context.Canceled), want: context.Canceled},
		"wrapped deadline":      {err: fmt.Errorf("pg: %w", context.DeadlineExceeded), want: context.DeadlineExceeded},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, ClassifyTenantDBRefusal(t.Context(), tc.err))
		})
	}

	t.Run("a caller that went away is reported whatever the refusal", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		require.Equal(t, context.Canceled, ClassifyTenantDBRefusal(ctx, errors.New("resolve failed")))
	})
}

func TestAuthorizeAssociation(t *testing.T) {
	t.Parallel()

	producer := producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken}
	withProducer := producerauth.WithProducer(t.Context(), producer)

	var asked []string

	authz := producerauth.NewTenantAuthorizer(func(_ context.Context, tenantID, service string) error {
		asked = append(asked, service+":"+tenantID)

		if tenantID == "tenant-a" {
			return nil
		}

		return fmt.Errorf("not active: %w", tmcore.ErrTenantNotFound)
	}, true)

	require.NoError(t, AuthorizeAssociation(withProducer, authz, "tenant-a"))
	require.Equal(t, constant.ErrInsufficientPrivileges, AuthorizeAssociation(withProducer, authz, "tenant-b"))
	require.Equal(t, []string{"ledger:tenant-a", "ledger:tenant-b"}, asked)

	require.Equal(t, constant.ErrContextPolicyUnavailable, AuthorizeAssociation(t.Context(), authz, "tenant-a"), "no producer")
	require.Equal(t, constant.ErrContextPolicyUnavailable, AuthorizeAssociation(withProducer, producerauth.NewTenantAuthorizer(nil, false), "tenant-a"), "inactive authorizer")
	require.Equal(t, constant.ErrReservationTenantRequired, AuthorizeAssociation(withProducer, authz, ""))
	require.Len(t, asked, 2, "no lookup for a refused request")
}
