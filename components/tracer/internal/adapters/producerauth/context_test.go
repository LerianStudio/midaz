// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package producerauth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
)

func TestProducerContextRoundTrips(t *testing.T) {
	t.Parallel()

	want := producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaCert}

	got, ok := producerauth.ProducerFromContext(producerauth.WithProducer(context.Background(), want))
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestProducerContextReportsAbsence(t *testing.T) {
	t.Parallel()

	for name, ctx := range map[string]context.Context{
		"no producer":     context.Background(),
		"empty service":   producerauth.WithProducer(context.Background(), producerauth.Producer{Via: producerauth.ViaToken}),
		"foreign value":   context.WithValue(context.Background(), struct{}{}, producerauth.Producer{Service: producerauth.ServiceLedger}),
		"nil context arg": nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, ok := producerauth.ProducerFromContext(ctx) //nolint:staticcheck // nil context is part of the contract under test
			require.False(t, ok)
			require.Zero(t, got)
		})
	}
}
