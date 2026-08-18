// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type revertRolloutTransitionStub struct {
	called string
	err    error
}

func (s *revertRolloutTransitionStub) Activate(context.Context) error {
	s.called = "active"

	return s.err
}

func (s *revertRolloutTransitionStub) MarkPhaseZeroDrained(context.Context) error {
	s.called = "phase-zero-drained"

	return s.err
}

func (s *revertRolloutTransitionStub) Finalize(context.Context) error {
	s.called = "finalized"

	return s.err
}

func TestApplyRevertRolloutTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		wantCalled string
		wantErr    string
	}{
		{name: "no target leaves marker unchanged"},
		{name: "activate", target: "active", wantCalled: "active"},
		{name: "drain phase zero", target: "phase-zero-drained", wantCalled: "phase-zero-drained"},
		{name: "finalize", target: "finalized", wantCalled: "finalized"},
		{name: "normalizes whitespace and case", target: " FINALIZED ", wantCalled: "finalized"},
		{name: "invalid target fails startup", target: "reopened", wantErr: "invalid REVERT_ROLLOUT_TARGET"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			guard := &revertRolloutTransitionStub{}
			err := applyRevertRolloutTarget(context.Background(), guard, tc.target)
			assert.Equal(t, tc.wantCalled, guard.called)
			if tc.wantErr == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestApplyRevertRolloutTarget_TransitionFailureFailsStartup(t *testing.T) {
	t.Parallel()

	transitionErr := errors.New("in-flight request remains")
	guard := &revertRolloutTransitionStub{err: transitionErr}
	err := applyRevertRolloutTarget(context.Background(), guard, "phase-zero-drained")
	require.ErrorIs(t, err, transitionErr)
	assert.Equal(t, "phase-zero-drained", guard.called)
}

func TestApplyConfigDefaults_RevertRolloutStartsInPhaseZero(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	applyConfigDefaults(cfg)

	assert.Equal(t, "legacy", cfg.RevertIdempotencyMode)
	assert.Empty(t, cfg.RevertRolloutTarget)
}

func TestValidateRevertRolloutConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    string
		target  string
		wantErr bool
	}{
		{name: "phase zero without transition", mode: "legacy"},
		{name: "phase zero activates freeze", mode: "legacy", target: "active"},
		{name: "bridge keeps active", mode: "bridge", target: "active"},
		{name: "bridge drains phase zero", mode: "bridge", target: "phase-zero-drained"},
		{name: "final observes drained", mode: "final", target: "phase-zero-drained"},
		{name: "final finalizes", mode: "final", target: "finalized"},
		{name: "invalid mode", mode: "bridg", wantErr: true},
		{name: "invalid target", mode: "legacy", target: "reopened", wantErr: true},
		{name: "phase zero cannot drain itself", mode: "legacy", target: "phase-zero-drained", wantErr: true},
		{name: "bridge cannot finalize", mode: "bridge", target: "finalized", wantErr: true},
		{name: "final cannot reopen active", mode: "final", target: "active", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateRevertRolloutConfiguration(tc.mode, tc.target)
			if tc.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
		})
	}
}
