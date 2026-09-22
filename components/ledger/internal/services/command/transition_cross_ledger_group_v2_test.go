// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestTransitionCrossLedgerGroupV2_RejectsTerminalGroupBeforeLocksOrEngine(t *testing.T) {
	groupID := uuid.MustParse("0199a500-0000-7000-8000-000000000001")
	target := pendingTransaction(false)
	groupText := groupID.String()
	target.GroupID = &groupText
	in := pendingTransitionInputFor(target)

	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	repo.EXPECT().FindByID(gomock.Any(), groupID).Return(&transactiongroup.TransactionGroup{
		ID:     groupID,
		Status: constant.APPROVED,
	}, nil)

	uc := &UseCase{TransactionGroupRepo: repo}
	result, err := uc.transitionCrossLedgerGroupV2(context.Background(), in, target, constant.APPROVED)
	require.Error(t, err)
	assert.Nil(t, result)

	var business pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &business))
	assert.Equal(t, constant.ErrCrossLedgerGroupNotPending.Error(), business.Code)
	assert.Contains(t, business.Message, constant.APPROVED)
}
