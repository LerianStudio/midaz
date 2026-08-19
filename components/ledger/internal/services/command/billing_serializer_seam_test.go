// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	billing "github.com/LerianStudio/lib-streaming/v3/billing"
)

// TestBillingSerializerSeam_ConcreteSatisfies locks that the concrete
// *billing.Serializer satisfies the unexported billingSerializer seam the
// UseCase field is typed on, so bootstrap can inject it and tests can fake it.
func TestBillingSerializerSeam_ConcreteSatisfies(t *testing.T) {
	t.Parallel()

	var _ billingSerializer = (*billing.Serializer)(nil)
}

// TestUseCase_SetBillingSerializer exercises the REAL production guard
// (UseCase.SetBillingSerializer), the single place the typed-nil-interface trap
// is defended. A nil *billing.Serializer must leave BillingSerializer == nil
// (dropping the guard would assign a typed-nil pointer that compares NON-nil, so
// this case FAILS on a regression); a non-nil pointer must be assigned.
func TestUseCase_SetBillingSerializer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serializer *billing.Serializer
		wantNil    bool
	}{
		{
			name:       "typed-nil pointer leaves field nil",
			serializer: (*billing.Serializer)(nil),
			wantNil:    true,
		},
		{
			name:       "non-nil pointer is assigned",
			serializer: &billing.Serializer{},
			wantNil:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := &UseCase{}
			uc.SetBillingSerializer(tt.serializer)

			if tt.wantNil {
				if uc.BillingSerializer != nil {
					t.Fatalf("expected BillingSerializer == nil, got a non-nil typed-nil interface")
				}

				return
			}

			if uc.BillingSerializer == nil {
				t.Fatalf("expected BillingSerializer to be assigned, got nil")
			}
		})
	}
}
