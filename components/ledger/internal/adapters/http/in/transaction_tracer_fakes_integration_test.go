// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
)

// stubReserver records what the reservation lifecycle asked of the transport and
// answers with the configured result or error.
type stubReserver struct {
	reserveCalls int
	confirmedIDs []uuid.UUID
	releasedIDs  []uuid.UUID

	confirmedTxns []uuid.UUID
	releasedTxns  []uuid.UUID

	result     *tracer.ReserveResult
	reserveErr error

	confirmErr error
	releaseErr error

	confirmByTxnErr error
	releaseByTxnErr error
}

func (s *stubReserver) Reserve(_ context.Context, _ tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	s.reserveCalls++

	if s.reserveErr != nil {
		return nil, s.reserveErr
	}

	return s.result, nil
}

func (s *stubReserver) Confirm(_ context.Context, id uuid.UUID) error {
	s.confirmedIDs = append(s.confirmedIDs, id)
	return s.confirmErr
}

func (s *stubReserver) Release(_ context.Context, id uuid.UUID) error {
	s.releasedIDs = append(s.releasedIDs, id)
	return s.releaseErr
}

func (s *stubReserver) ConfirmByTransaction(_ context.Context, transactionID uuid.UUID) error {
	s.confirmedTxns = append(s.confirmedTxns, transactionID)
	return s.confirmByTxnErr
}

func (s *stubReserver) ReleaseByTransaction(_ context.Context, transactionID uuid.UUID) error {
	s.releasedTxns = append(s.releasedTxns, transactionID)
	return s.releaseByTxnErr
}

// forbiddenReserver fails the test on ANY call. It is the direct proof the route gate
// is asked for: asserting a zero call count only shows the stub was not invoked, while
// this shows the seam could not have reached a transport at all.
type forbiddenReserver struct {
	t *testing.T
}

func (f *forbiddenReserver) fail(method string) {
	f.t.Helper()
	f.t.Fatalf("a /v1 route reached the tracer via %s — the route gate must return before any transport call", method)
}

func (f *forbiddenReserver) Reserve(_ context.Context, _ tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	f.fail("Reserve")

	return nil, nil
}

func (f *forbiddenReserver) Confirm(_ context.Context, _ uuid.UUID) error {
	f.fail("Confirm")

	return nil
}

func (f *forbiddenReserver) Release(_ context.Context, _ uuid.UUID) error {
	f.fail("Release")

	return nil
}

func (f *forbiddenReserver) ConfirmByTransaction(_ context.Context, _ uuid.UUID) error {
	f.fail("ConfirmByTransaction")

	return nil
}

func (f *forbiddenReserver) ReleaseByTransaction(_ context.Context, _ uuid.UUID) error {
	f.fail("ReleaseByTransaction")

	return nil
}
