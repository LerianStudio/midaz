// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package producerauth_test

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

const authorizerTenantID = "tenant-007"

type lookupCall struct {
	tenantID string
	service  string
}

func fakeLookup(result error, calls *[]lookupCall) producerauth.TenantLookup {
	return func(_ context.Context, tenantID, service string) error {
		*calls = append(*calls, lookupCall{tenantID: tenantID, service: service})

		return result
	}
}

var ledgerProducer = producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken}

func TestTenantAuthorizerSingleTenantNeverLooksUp(t *testing.T) {
	t.Parallel()

	var calls []lookupCall

	authorizer := producerauth.NewTenantAuthorizer(fakeLookup(tmcore.ErrTenantNotFound, &calls), false)
	require.False(t, authorizer.Active())

	for _, tenantID := range []string{"", authorizerTenantID, "bad tenant id!"} {
		require.NoError(t, authorizer.Authorize(context.Background(), tenantID, ledgerProducer))
	}

	require.Empty(t, calls)

	var absent *producerauth.TenantAuthorizer
	require.False(t, absent.Active())
	require.NoError(t, absent.Authorize(context.Background(), authorizerTenantID, ledgerProducer))
}

func TestTenantAuthorizerRejectsMissingOrInvalidTenant(t *testing.T) {
	t.Parallel()

	for _, tenantID := range []string{"", "bad tenant id!", "../tenant"} {
		var calls []lookupCall

		authorizer := producerauth.NewTenantAuthorizer(fakeLookup(nil, &calls), true)

		err := authorizer.Authorize(context.Background(), tenantID, ledgerProducer)
		require.ErrorIs(t, err, constant.ErrReservationTenantRequired, tenantID)
		require.Empty(t, calls, "an invalid tenant must never reach the tenant-manager")
	}
}

func TestTenantAuthorizerClassifiesLookupOutcome(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		result error
		want   error
	}{
		{"associated", nil, nil},
		{"tenant not found", fmt.Errorf("tenant manager: %w", tmcore.ErrTenantNotFound), constant.ErrInsufficientPrivileges},
		{"association denied", fmt.Errorf("tenant %s: %w", authorizerTenantID, tmcore.ErrTenantServiceAccessDenied), constant.ErrInsufficientPrivileges},
		{"association suspended", fmt.Errorf("%w: %w", tmcore.ErrTenantServiceAccessDenied, &tmcore.TenantSuspendedError{TenantID: authorizerTenantID, Status: "suspended"}), constant.ErrInsufficientPrivileges},
		{"network failure", errors.New("dial tcp: connection refused"), constant.ErrTenantServiceUnavailable},
		{"tenant-manager 5xx", errors.New("tenant manager returned status 502"), constant.ErrTenantServiceUnavailable},
		{"breaker open", tmcore.ErrCircuitBreakerOpen, constant.ErrTenantServiceUnavailable},
		{"service not configured", tmcore.ErrServiceNotConfigured, constant.ErrTenantServiceUnavailable},
		{"cancelled", context.Canceled, constant.ErrTenantServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var calls []lookupCall

			authorizer := producerauth.NewTenantAuthorizer(fakeLookup(tc.result, &calls), true)
			require.True(t, authorizer.Active())

			err := authorizer.Authorize(context.Background(), authorizerTenantID, ledgerProducer)
			require.Equal(t, []lookupCall{{tenantID: authorizerTenantID, service: producerauth.ServiceLedger}}, calls,
				"the association is checked against the producer's service")

			if tc.want == nil {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, tc.want)
			require.ErrorIs(t, err, tc.result, "the cause stays in the chain for span attribution")

			if !errors.Is(tc.want, constant.ErrContextPolicyUnavailable) {
				require.NotErrorIs(t, err, constant.ErrContextPolicyUnavailable, "a tenant-manager outcome is never a policy configuration defect")
			}
		})
	}
}

func TestTenantAuthorizerRejectsProducerOutsideRoster(t *testing.T) {
	t.Parallel()

	var calls []lookupCall

	authorizer := producerauth.NewTenantAuthorizer(fakeLookup(nil, &calls), true)

	for _, producer := range []producerauth.Producer{{}, {Service: "tracer", Via: producerauth.ViaCert}} {
		require.ErrorIs(t, authorizer.Authorize(context.Background(), authorizerTenantID, producer), constant.ErrInsufficientPrivileges)
	}

	require.Empty(t, calls)
}

func TestTenantAuthorizerWithoutLookupFailsClosed(t *testing.T) {
	t.Parallel()

	authorizer := producerauth.NewTenantAuthorizer(nil, true)

	require.ErrorIs(t, authorizer.Authorize(context.Background(), authorizerTenantID, ledgerProducer), constant.ErrContextPolicyUnavailable)
	require.ErrorIs(t, authorizer.Authorize(context.Background(), "", ledgerProducer), constant.ErrReservationTenantRequired)
}
