// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	billing "github.com/LerianStudio/lib-streaming/v2/billing"
)

// TestBillingSerializerSeam_ConcreteSatisfies locks that the concrete
// *billing.Serializer satisfies the unexported billingSerializer seam the
// UseCase field is typed on, so bootstrap can inject it and Phase 2 can fake it.
func TestBillingSerializerSeam_ConcreteSatisfies(t *testing.T) {
	t.Parallel()

	var _ billingSerializer = (*billing.Serializer)(nil)
}

// TestBillingSerializer_NilGuardKeepsFieldNil is the typed-nil trap guard. A
// disabled build yields a nil *billing.Serializer; the nil-guarded injection
// (mirroring bootstrap config.go) must leave UseCase.BillingSerializer == nil,
// and the test also documents the trap the guard exists to prevent.
func TestBillingSerializer_NilGuardKeepsFieldNil(t *testing.T) {
	t.Parallel()

	var disabled *billing.Serializer // nil, as returned on the disabled branch

	uc := &UseCase{}

	// Correct guarded injection: only assign when non-nil.
	if disabled != nil {
		uc.BillingSerializer = disabled
	}

	if uc.BillingSerializer != nil {
		t.Fatalf("expected BillingSerializer == nil when disabled, got a non-nil typed-nil interface")
	}

	// Document the trap the guard protects against: a typed-nil pointer assigned
	// straight to the interface compares NON-nil.
	var trap billingSerializer = disabled
	if trap == nil {
		t.Fatalf("expected typed-nil *billing.Serializer assigned to interface to be non-nil")
	}
}
