// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Tests for InvalidateLedgerSettingsCache: no-op when RedisRepo is nil,
// Del called with correct key on success, and Del error does not panic.
package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/redis"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestInvalidateLedgerSettingsCache_NilRedisRepo_NoOp(t *testing.T) {
	t.Parallel()

	uc := &UseCase{RedisRepo: nil}
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	uc.InvalidateLedgerSettingsCache(ctx, orgID, ledgerID)
}

func TestInvalidateLedgerSettingsCache_WithRedisRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		orgID    uuid.UUID
		ledgerID uuid.UUID
		delErr   error
	}{
		{
			name:     "calls_del_with_correct_key_success",
			orgID:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			ledgerID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			delErr:   nil,
		},
		{
			name:     "del_error_does_not_panic",
			orgID:    uuid.New(),
			ledgerID: uuid.New(),
			delErr:   errors.New("redis error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			uc := &UseCase{RedisRepo: mockRedisRepo}
			ctx := context.Background()
			cacheKey := BuildLedgerSettingsCacheKey(tt.orgID, tt.ledgerID)

			mockRedisRepo.EXPECT().
				Del(gomock.Any(), cacheKey).
				Return(tt.delErr)

			uc.InvalidateLedgerSettingsCache(ctx, tt.orgID, tt.ledgerID)
		})
	}
}
