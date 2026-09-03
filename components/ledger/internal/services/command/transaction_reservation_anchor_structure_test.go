// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"
)

// Gate 5 — the version split of the PENDING state transition is enforced by
// construction: the /v1 pipeline simply does not name the reservation seams, and the
// /v2 pipeline names them in the one order the contract allows. Nothing at runtime
// distinguishes "the seam ran and no-oped" from "the seam was never reached", so these
// gates read the source, mirroring the create-side gates in
// create_transaction_version_gates_test.go.
//
// The create-side reserve anchor is covered by
// TestCreateTransactionV1_NeverReferencesVersionedSeams over the same source.

// byTransactionSeams are the reservation seams the /v1 state transition excludes.
var byTransactionSeams = []string{
	"confirmReservationsByTransaction",
	"releaseReservationsByTransaction",
}

func TestTransitionPendingV1_NeverReferencesReservationSeams(t *testing.T) {
	names := calledNames(t, readTransportSource(t, pendingPipelineFile, "func (uc *UseCase) transitionPendingV1"), "transitionPendingV1")

	for _, seam := range byTransactionSeams {
		if containsName(names, seam) {
			t.Errorf("transitionPendingV1 references %s — the /v1 contract carries no tracer, so a /v1 commit or cancel must build no request and dial nothing", seam)
		}
	}
}

func TestTransitionPendingV2_DrivesTheReservationLifecycleAfterTheCommit(t *testing.T) {
	src := readTransportSource(t, pendingPipelineFile, "func (uc *UseCase) "+pendingTransitionV2Func)

	names := calledNames(t, src, pendingTransitionV2Func)

	commitAt := indexOfName(names, "commitPendingBalances")
	if commitAt == -1 {
		t.Fatal("transitionPendingV2 does not call commitPendingBalances — the pipeline shape changed")
	}

	for _, seam := range byTransactionSeams {
		at := indexOfName(names, seam)
		if at == -1 {
			t.Errorf("transitionPendingV2 does not call %s — the /v2 contract includes the PENDING reservation lifecycle", seam)

			continue
		}

		if at <= commitAt {
			t.Errorf("transitionPendingV2 calls %s (pos %d) before commitPendingBalances (pos %d) — the reservation is flipped only once the balances have moved", seam, at, commitAt)
		}
	}

	finalizeAt := indexOfName(names, pendingFinalizeFuncName)
	if finalizeAt == -1 {
		t.Fatal("transitionPendingV2 does not call finalizePendingTransition — the pipeline shape changed")
	}

	for _, seam := range byTransactionSeams {
		if at := indexOfName(names, seam); at != -1 && at > finalizeAt {
			t.Errorf("transitionPendingV2 calls %s (pos %d) after finalizePendingTransition (pos %d) — a failed write must not follow an already-flipped reservation", seam, at, finalizeAt)
		}
	}
}

// TestTransitionPendingGatesBite proves the two gates above read the seams by name and
// therefore fail on a pipeline that drops them, adds them to the wrong version, or
// orders them before the balance commit.
func TestTransitionPendingGatesBite(t *testing.T) {
	const leaked = `package command

func (uc *UseCase) transitionPendingV1() error {
	_ = uc.commitPendingBalances()
	uc.confirmReservationsByTransaction() // BUG: the /v1 contract carries no tracer
	return nil
}
`

	if names := calledNames(t, leaked, "transitionPendingV1"); !containsName(names, "confirmReservationsByTransaction") {
		t.Fatal("Gate 5 bite: a /v1 pipeline naming a reservation seam must be detected")
	}

	const reordered = `package command

func (uc *UseCase) transitionPendingV2() error {
	uc.confirmReservationsByTransaction() // BUG: flipped before the balances moved
	_ = uc.commitPendingBalances()
	_ = uc.finalizePendingTransition()
	return nil
}
`

	names := calledNames(t, reordered, "transitionPendingV2")

	confirmAt := indexOfName(names, "confirmReservationsByTransaction")
	commitAt := indexOfName(names, "commitPendingBalances")

	if confirmAt == -1 || commitAt == -1 {
		t.Fatalf("Gate 5 bite fixture sanity: missing positions confirm=%d commit=%d", confirmAt, commitAt)
	}

	if confirmAt > commitAt {
		t.Error("Gate 5 bite: a confirm placed before the balance commit must be detected as out of order")
	}

	const correct = `package command

func (uc *UseCase) transitionPendingV2() error {
	_ = uc.commitPendingBalances()
	uc.confirmReservationsByTransaction()
	uc.releaseReservationsByTransaction()
	return uc.finalizePendingTransition()
}
`

	ok := calledNames(t, correct, "transitionPendingV2")
	if indexOfName(ok, "commitPendingBalances") >= indexOfName(ok, "confirmReservationsByTransaction") {
		t.Error("Gate 5 bite: fixture sanity — the correct shape must place the commit before the confirm")
	}
}
