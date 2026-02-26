// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// mockSettingsPort implements mbootstrap.SettingsPort for testing
type mockSettingsPort struct {
	settings map[string]any
	err      error
}

func (m *mockSettingsPort) GetLedgerSettings(_ context.Context, _, _ uuid.UUID) (map[string]any, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.settings, nil
}

func TestService_SetSettingsPort(t *testing.T) {
	t.Parallel()

	t.Run("sets SettingsPort on both UseCases", func(t *testing.T) {
		t.Parallel()

		commandUseCase := &command.UseCase{}
		queryUseCase := &query.UseCase{}

		service := &Service{
			commandUseCase: commandUseCase,
			queryUseCase:   queryUseCase,
		}

		mockPort := &mockSettingsPort{
			settings: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
		}

		// Act
		service.SetSettingsPort(mockPort)

		// Assert - both UseCases should have the port set
		assert.Equal(t, mockPort, commandUseCase.SettingsPort)
		assert.Equal(t, mockPort, queryUseCase.SettingsPort)

		// Verify the port works correctly
		ctx := context.Background()
		settings, err := queryUseCase.SettingsPort.GetLedgerSettings(ctx, uuid.New(), uuid.New())

		assert.NoError(t, err)
		assert.Equal(t, mockPort.settings, settings)
	})

	t.Run("handles nil commandUseCase gracefully", func(t *testing.T) {
		t.Parallel()

		queryUseCase := &query.UseCase{}

		service := &Service{
			commandUseCase: nil,
			queryUseCase:   queryUseCase,
		}

		mockPort := &mockSettingsPort{}

		// Assert commandUseCase is nil before call
		assert.Nil(t, service.commandUseCase)

		// Must not panic
		service.SetSettingsPort(mockPort)

		// Assert commandUseCase remains nil
		assert.Nil(t, service.commandUseCase)

		// queryUseCase should still be updated
		assert.Equal(t, mockPort, queryUseCase.SettingsPort)
	})

	t.Run("handles nil queryUseCase gracefully", func(t *testing.T) {
		t.Parallel()

		commandUseCase := &command.UseCase{}

		service := &Service{
			commandUseCase: commandUseCase,
			queryUseCase:   nil,
		}

		mockPort := &mockSettingsPort{}

		// Assert queryUseCase is nil before call
		assert.Nil(t, service.queryUseCase)

		// Must not panic
		service.SetSettingsPort(mockPort)

		// Assert queryUseCase remains nil
		assert.Nil(t, service.queryUseCase)

		// commandUseCase should still be updated
		assert.Equal(t, mockPort, commandUseCase.SettingsPort)
	})

	t.Run("handles both UseCases nil gracefully", func(t *testing.T) {
		t.Parallel()

		service := &Service{
			commandUseCase: nil,
			queryUseCase:   nil,
		}

		mockPort := &mockSettingsPort{}

		// Assert both are nil before call
		assert.Nil(t, service.commandUseCase)
		assert.Nil(t, service.queryUseCase)

		// Must not panic
		service.SetSettingsPort(mockPort)

		// Assert both remain nil
		assert.Nil(t, service.commandUseCase)
		assert.Nil(t, service.queryUseCase)
	})

	t.Run("handles nil port gracefully", func(t *testing.T) {
		t.Parallel()

		commandUseCase := &command.UseCase{}
		queryUseCase := &query.UseCase{}

		service := &Service{
			commandUseCase: commandUseCase,
			queryUseCase:   queryUseCase,
		}

		// Pre-set a port
		initialPort := &mockSettingsPort{settings: map[string]any{"initial": true}}
		service.SetSettingsPort(initialPort)

		assert.Equal(t, initialPort, commandUseCase.SettingsPort)
		assert.Equal(t, initialPort, queryUseCase.SettingsPort)

		// Set to nil (e.g., cleanup or disable settings functionality)
		service.SetSettingsPort(nil)

		// Verify ports are now nil
		assert.Nil(t, commandUseCase.SettingsPort)
		assert.Nil(t, queryUseCase.SettingsPort)
	})

	t.Run("can replace SettingsPort", func(t *testing.T) {
		t.Parallel()

		commandUseCase := &command.UseCase{}
		queryUseCase := &query.UseCase{}

		service := &Service{
			commandUseCase: commandUseCase,
			queryUseCase:   queryUseCase,
		}

		port1 := &mockSettingsPort{settings: map[string]any{"version": "1"}}
		port2 := &mockSettingsPort{settings: map[string]any{"version": "2"}}

		// Set first port
		service.SetSettingsPort(port1)
		assert.Equal(t, port1, commandUseCase.SettingsPort)
		assert.Equal(t, port1, queryUseCase.SettingsPort)

		// Replace with second port
		service.SetSettingsPort(port2)
		assert.Equal(t, port2, commandUseCase.SettingsPort)
		assert.Equal(t, port2, queryUseCase.SettingsPort)
	})
}
