// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type revertUpdateFreezeStub struct {
	active           bool
	ready            bool
	err              error
	releaseErr       error
	policyRead       int
	readyRead        int
	approvedReleases int
	revertReleases   int
	revertTokens     []string
	revertAttempts   []string
	completeErr      error
	completedModes   []string
	completedTokens  []string
	terminalComplete bool
	terminalErr      error
}

func TestActiveRevertIdempotencyMode_ZeroValuePreservesReleasedAlgorithm(t *testing.T) {
	t.Parallel()

	handler := &TransactionHandler{}
	assert.Equal(t, revertIdempotencyModeLegacy, handler.activeRevertIdempotencyMode())
}

func (s *revertUpdateFreezeStub) ApprovedUpdatePolicy(context.Context, string) (bool, bool, error) {
	s.policyRead++

	return s.active, s.ready, s.err
}

func (s *revertUpdateFreezeStub) ReadyForMode(context.Context, string) (bool, error) {
	s.readyRead++

	return s.ready, s.err
}

func (s *revertUpdateFreezeStub) AcquireApprovedUpdate(context.Context, string, string) (bool, bool, bool, error) {
	if s.err != nil {
		return false, false, false, s.err
	}
	if s.active {
		return false, true, false, nil
	}

	return s.ready, false, s.ready, nil
}

func (s *revertUpdateFreezeStub) ReleaseApprovedUpdate(context.Context, string) error {
	s.approvedReleases++

	return s.releaseErr
}

func (s *revertUpdateFreezeStub) AcquireRevert(_ context.Context, mode, token, attemptID string) (bool, bool, string, error) {
	s.revertTokens = append(s.revertTokens, token)
	s.revertAttempts = append(s.revertAttempts, attemptID)
	if mode == revertIdempotencyModeFinal && s.ready {
		return true, false, "finalized", s.err
	}
	phase := ""
	if s.active {
		phase = "active"
	}

	return s.ready, s.ready, phase, s.err
}

func TestAcquireRevertRolloutRequest_ReusesOriginTokenAfterCrash(t *testing.T) {
	t.Parallel()

	freeze := &revertUpdateFreezeStub{ready: true}
	handler := &TransactionHandler{RevertIdempotencyMode: revertIdempotencyModeLegacy, RevertUpdateFreeze: freeze}
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()

	_, firstToken, _, err := handler.acquireRevertRolloutRequest(context.Background(), organizationID, ledgerID, originID)
	require.NoError(t, err)
	_, retryToken, _, err := handler.acquireRevertRolloutRequest(context.Background(), organizationID, ledgerID, originID)
	require.NoError(t, err)
	_, otherToken, _, err := handler.acquireRevertRolloutRequest(context.Background(), organizationID, ledgerID, uuid.New())
	require.NoError(t, err)

	assert.Equal(t, firstToken, retryToken,
		"a same-origin retry must own and release the exact admission stranded by a crashed request")
	assert.NotEqual(t, firstToken, otherToken, "distinct economic origins must remain independent rollout blockers")
	assert.Equal(t, []string{firstToken, retryToken, otherToken}, freeze.revertTokens)
	assert.NotEqual(t, freeze.revertAttempts[0], freeze.revertAttempts[1],
		"distinct HTTP attempts for one origin must not share release ownership")
}

func (s *revertUpdateFreezeStub) ReleaseRevert(context.Context, string, string, string) error {
	s.revertReleases++

	return s.releaseErr
}

func (s *revertUpdateFreezeStub) CompleteRevert(_ context.Context, mode, token string) error {
	s.completedModes = append(s.completedModes, mode)
	s.completedTokens = append(s.completedTokens, token)

	return s.completeErr
}

func (s *revertUpdateFreezeStub) RevertTerminalHandoffComplete(context.Context, string, string) (bool, error) {
	return s.terminalComplete, s.terminalErr
}

func TestAcquireRolloutRequest_AmbiguousAdmissionPreservesSharedRevertOrigin(t *testing.T) {
	t.Parallel()

	acquireErr := errors.New("redis response lost")
	releaseErr := errors.New("redis cleanup unavailable")

	tests := []struct {
		name           string
		invoke         func(*TransactionHandler) error
		wantApproved   int
		wantRevert     int
		wantReleaseErr bool
	}{
		{
			name: "approved update",
			invoke: func(handler *TransactionHandler) error {
				_, err := handler.acquireApprovedUpdateRolloutRequest(context.Background(), constant.APPROVED)

				return err
			},
			wantApproved: 1,
		},
		{
			name: "revert",
			invoke: func(handler *TransactionHandler) error {
				_, _, _, err := handler.acquireRevertRolloutRequest(context.Background(), uuid.New(), uuid.New(), uuid.New())

				return err
			},
			wantRevert: 0,
		},
		{
			name: "revert admission remains durable when its response is ambiguous",
			invoke: func(handler *TransactionHandler) error {
				_, _, _, err := handler.acquireRevertRolloutRequest(context.Background(), uuid.New(), uuid.New(), uuid.New())

				return err
			},
			wantRevert: 0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			freeze := &revertUpdateFreezeStub{err: acquireErr}
			if tc.wantReleaseErr {
				freeze.releaseErr = releaseErr
			}
			handler := &TransactionHandler{RevertIdempotencyMode: revertIdempotencyModeBridge, RevertUpdateFreeze: freeze}

			err := tc.invoke(handler)
			require.ErrorIs(t, err, acquireErr)
			assert.Equal(t, tc.wantApproved, freeze.approvedReleases)
			assert.Equal(t, tc.wantRevert, freeze.revertReleases)
			assert.Equal(t, tc.wantReleaseErr, errors.Is(err, releaseErr))
		})
	}
}

func TestEnforceRevertUpdateFreeze_RolloutPhases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mode       string
		status     string
		freeze     *revertUpdateFreezeStub
		wantCode   string
		policyRead int
	}{
		{name: "phase zero marker off preserves approved updates", mode: revertIdempotencyModeLegacy, status: constant.APPROVED, freeze: &revertUpdateFreezeStub{ready: true}, policyRead: 1},
		{name: "phase zero marker on freezes approved updates", mode: revertIdempotencyModeLegacy, status: constant.APPROVED, freeze: &revertUpdateFreezeStub{active: true, ready: true}, wantCode: constant.ErrActionNotPermitted.Error(), policyRead: 1},
		{name: "finalized marker fences a surviving phase zero pod", mode: revertIdempotencyModeLegacy, status: constant.APPROVED, freeze: &revertUpdateFreezeStub{}, wantCode: constant.ErrRevertRolloutFreezeRequired.Error(), policyRead: 1},
		{name: "bridge marker off blocks approved updates", mode: revertIdempotencyModeBridge, status: constant.APPROVED, freeze: &revertUpdateFreezeStub{}, wantCode: constant.ErrRevertRolloutFreezeRequired.Error(), policyRead: 1},
		{name: "bridge marker on freezes approved updates", mode: revertIdempotencyModeBridge, status: constant.APPROVED, freeze: &revertUpdateFreezeStub{active: true, ready: true}, wantCode: constant.ErrActionNotPermitted.Error(), policyRead: 1},
		{name: "final remains frozen before rollout finalization", mode: revertIdempotencyModeFinal, status: constant.APPROVED, freeze: &revertUpdateFreezeStub{active: true, ready: true}, wantCode: constant.ErrActionNotPermitted.Error(), policyRead: 1},
		{name: "finalized marker restores approved updates", mode: revertIdempotencyModeFinal, status: constant.APPROVED, freeze: &revertUpdateFreezeStub{ready: true}, policyRead: 1},
		{name: "pending updates remain allowed while frozen", mode: revertIdempotencyModeBridge, status: constant.PENDING, freeze: &revertUpdateFreezeStub{active: true, ready: true}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := &TransactionHandler{RevertIdempotencyMode: tc.mode, RevertUpdateFreeze: tc.freeze}
			err := handler.enforceRevertUpdateFreeze(context.Background(), tc.status)
			assert.Equal(t, tc.policyRead, tc.freeze.policyRead)
			if tc.wantCode == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			switch tc.wantCode {
			case constant.ErrActionNotPermitted.Error():
				var businessErr pkg.UnprocessableOperationError
				require.ErrorAs(t, err, &businessErr)
				assert.Equal(t, tc.wantCode, businessErr.Code)
			case constant.ErrRevertRolloutFreezeRequired.Error():
				var businessErr pkg.ServiceUnavailableError
				require.ErrorAs(t, err, &businessErr)
				assert.Equal(t, tc.wantCode, businessErr.Code)
			}
		})
	}
}

func TestRequireRevertRolloutBarrier_RejectsMissingOrUnreadableMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     string
		freeze   *revertUpdateFreezeStub
		wantCode string
	}{
		{name: "phase zero permits old algorithm while marker is absent", mode: revertIdempotencyModeLegacy, freeze: &revertUpdateFreezeStub{ready: true}},
		{name: "finalized marker fences a surviving phase zero revert", mode: revertIdempotencyModeLegacy, freeze: &revertUpdateFreezeStub{}, wantCode: constant.ErrRevertRolloutFreezeRequired.Error()},
		{name: "bridge requires active marker", mode: revertIdempotencyModeBridge, freeze: &revertUpdateFreezeStub{}, wantCode: constant.ErrRevertRolloutFreezeRequired.Error()},
		{name: "final requires active or finalized marker", mode: revertIdempotencyModeFinal, freeze: &revertUpdateFreezeStub{}, wantCode: constant.ErrRevertRolloutFreezeRequired.Error()},
		{name: "finalized marker permits final requests", mode: revertIdempotencyModeFinal, freeze: &revertUpdateFreezeStub{ready: true}},
		{name: "marker read failure is technical", mode: revertIdempotencyModeBridge, freeze: &revertUpdateFreezeStub{err: errors.New("redis unavailable")}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := &TransactionHandler{RevertIdempotencyMode: tc.mode, RevertUpdateFreeze: tc.freeze}
			err := handler.requireRevertRolloutBarrier(context.Background())
			if tc.wantCode == "" && (tc.freeze == nil || tc.freeze.err == nil) {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			if tc.wantCode != "" {
				var businessErr pkg.ServiceUnavailableError
				require.ErrorAs(t, err, &businessErr)
				assert.Equal(t, tc.wantCode, businessErr.Code)
			}
		})
	}
}
