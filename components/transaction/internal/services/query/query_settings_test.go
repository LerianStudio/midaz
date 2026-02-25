// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestUseCase_HasSettingsPort(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSettingsPort := mbootstrap.NewMockSettingsPort(ctrl)

	uc := &UseCase{
		SettingsPort: mockSettingsPort,
	}

	assert.NotNil(t, uc.SettingsPort)
}
