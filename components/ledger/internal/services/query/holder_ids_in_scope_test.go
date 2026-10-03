// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestHolderIDsInScope(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := account.NewMockRepository(ctrl)
	uc := &UseCase{AccountRepo: repo}

	org, holder := uuid.New(), uuid.New()
	scope := http.ScopeConfinement{"ledgerId": {uuid.New()}}

	repo.EXPECT().ListHolderIDs(gomock.Any(), org, scope).Return([]uuid.UUID{holder}, nil)

	ids, err := uc.HolderIDsInScope(context.Background(), org, scope)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{holder}, ids)

	boom := errors.New("replica down")
	repo.EXPECT().ListHolderIDs(gomock.Any(), org, scope).Return(nil, boom)

	_, err = uc.HolderIDsInScope(context.Background(), org, scope)
	require.ErrorIs(t, err, boom)
}
