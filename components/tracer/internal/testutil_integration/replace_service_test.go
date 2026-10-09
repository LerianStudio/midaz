// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package testutil_integration

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/bootstrap"
)

func TestReplaceService(t *testing.T) {
	errStop := errors.New("stop failed")
	errStart := errors.New("start failed")

	tests := []struct {
		name       string
		nilStop    bool
		stopErr    error
		startErr   error
		wantCalls  []string
		wantSvc    bool
		wantErrors []error
	}{
		{
			name:      "stop and start succeed",
			wantCalls: []string{"stop", "start"},
			wantSvc:   true,
		},
		{
			name:       "stop fails and start still runs",
			stopErr:    errStop,
			wantCalls:  []string{"stop", "start"},
			wantSvc:    true,
			wantErrors: []error{errStop},
		},
		{
			name:       "start fails after a clean stop",
			startErr:   errStart,
			wantCalls:  []string{"stop", "start"},
			wantErrors: []error{errStart},
		},
		{
			name:       "both fail and both causes are reported",
			stopErr:    errStop,
			startErr:   errStart,
			wantCalls:  []string{"stop", "start"},
			wantErrors: []error{errStop, errStart},
		},
		{
			name:      "nil stop is treated as success",
			nilStop:   true,
			wantCalls: []string{"start"},
			wantSvc:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string

			started := &bootstrap.Service{}

			var stop func() error
			if !tt.nilStop {
				stop = func() error {
					calls = append(calls, "stop")

					return tt.stopErr
				}
			}

			start := func() (*bootstrap.Service, error) {
				calls = append(calls, "start")

				if tt.startErr != nil {
					return nil, tt.startErr
				}

				return started, nil
			}

			svc, err := replaceService(stop, start)

			require.Equal(t, tt.wantCalls, calls)

			if tt.wantSvc {
				require.Same(t, started, svc)
			} else {
				require.Nil(t, svc)
			}

			if len(tt.wantErrors) == 0 {
				require.NoError(t, err)

				return
			}

			for _, want := range tt.wantErrors {
				require.ErrorIs(t, err, want)
			}
		})
	}
}
