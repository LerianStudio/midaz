// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// AccountAdminHolder classifies what holds an account's administrative ownership
// when a seed admission is refused.
type AccountAdminHolder string

const (
	// AccountAdminHolderNone reports that nothing refused the admission: it was
	// admitted, or the call failed before a holder could be read.
	AccountAdminHolderNone AccountAdminHolder = ""
	// AccountAdminHolderExclusive reports an exclusive owner: a closing, a balance
	// creation or a balance deletion. Which one it is, the ownership key does not
	// record; a closing is told apart by its closing marker.
	AccountAdminHolderExclusive AccountAdminHolder = "exclusive"
)

// Verdicts shared by the conditional protection scripts.
const (
	accountProtectionScriptApplied    int64 = 1
	accountProtectionScriptRefused    int64 = 0
	accountProtectionScriptUnreadable int64 = -1
)

// The administrative ownership key has two modes, told apart by its Redis type. A
// string is one exclusive owner (a closing, a balance creation or deletion). A
// sorted set holds the shared seed admissions of cache-miss balance loads: member
// = token, score = acquisition instant in milliseconds on the server clock. Every
// script below checks the type and writes in the same atomic step, which is what
// keeps the two modes mutually exclusive.
//
// Both modes live on the same key on purpose. A pod that still takes ownership
// with a plain SET NX finds the key occupied while admissions are live, so it can
// never slip a closing past a seed in flight.

//go:embed scripts/acquire_account_seed_admission.lua
var acquireAccountSeedAdmissionLua string

//go:embed scripts/release_account_seed_admission.lua
var releaseAccountSeedAdmissionLua string

//go:embed scripts/acquire_account_admin_ownership.lua
var acquireAccountAdminOwnershipLua string

//go:embed scripts/release_account_admin_ownership.lua
var releaseAccountAdminOwnershipLua string

var (
	acquireAccountSeedAdmissionScript  = redis.NewScript(acquireAccountSeedAdmissionLua)
	releaseAccountSeedAdmissionScript  = redis.NewScript(releaseAccountSeedAdmissionLua)
	acquireAccountAdminOwnershipScript = redis.NewScript(acquireAccountAdminOwnershipLua)
	releaseAccountAdminOwnershipScript = redis.NewScript(releaseAccountAdminOwnershipLua)
)

func (rr *RedisConsumerRepository) AcquireAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return rr.runAccountAdminOwnershipScript(ctx, acquireAccountAdminOwnershipScript, "redis.acquire_account_admin_ownership", organizationID, ledgerID, accountID, token)
}

func (rr *RedisConsumerRepository) ReleaseAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return rr.runAccountAdminOwnershipScript(ctx, releaseAccountAdminOwnershipScript, "redis.release_account_admin_ownership", organizationID, ledgerID, accountID, token)
}

func (rr *RedisConsumerRepository) AcquireAccountSeedAdmission(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, AccountAdminHolder, error) {
	admitted, err := rr.runAccountAdminOwnershipScript(ctx, acquireAccountSeedAdmissionScript, "redis.acquire_account_seed_admission", organizationID, ledgerID, accountID, token)
	if err != nil || admitted {
		return admitted, AccountAdminHolderNone, err
	}

	// The script refuses an admission only for an exclusive owner.
	return false, AccountAdminHolderExclusive, nil
}

func (rr *RedisConsumerRepository) ReleaseAccountSeedAdmission(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return rr.runAccountAdminOwnershipScript(ctx, releaseAccountSeedAdmissionScript, "redis.release_account_seed_admission", organizationID, ledgerID, accountID, token)
}

// runAccountAdminOwnershipScript runs one ownership script over the account's key
// for the caller's token. An empty token is refused before Redis is reached: it
// would name no operation, so nothing could ever release it.
func (rr *RedisConsumerRepository) runAccountAdminOwnershipScript(ctx context.Context, script *redis.Script, spanName string, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, fmt.Errorf("%w: owner token is empty", ErrAccountProtectionMarkerUnreadable)
	}

	key := utils.AccountAdminOwnershipKey(organizationID, ledgerID, accountID)

	verdict, err := rr.runAccountProtectionScript(ctx, script, spanName, key, token)

	return verdict == accountProtectionScriptApplied, err
}
