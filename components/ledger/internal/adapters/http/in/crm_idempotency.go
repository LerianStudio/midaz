// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
)

// crmIdempotencyClaim is the result of a CRM idempotency claim at the handler
// layer. ReplayJSON is non-nil when the slot already held a serialized entity;
// the caller deserializes it and returns it as the cached response. InternalKey
// and TTL are always set so the caller can store the created entity on the
// fresh-claim path.
type crmIdempotencyClaim struct {
	InternalKey string
	ReplayJSON  *string
	TTL         time.Duration
}

// claimCRMIdempotency runs the handler-side idempotency dance shared by
// CreateHolder and CreateInstrument: it extracts the client key/TTL, computes
// the request-body hash (used as the key when the client sends none), builds
// the caller-supplied namespaced internal key, sets the X-Idempotency-Replayed
// header to "false", and claims the slot. On a replay hit it sets the header to
// "true" and returns the cached JSON.
//
// keyBuilder receives the resolved key (client key, or body hash on fallback)
// and produces the CRM-namespaced internal key.
func claimCRMIdempotency(ctx context.Context, c *fiber.Ctx, uc *services.UseCase, payload any, keyBuilder func(key string) string) (*crmIdempotencyClaim, error) {
	clientKey, ttl := http.GetIdempotencyKeyAndTTL(c)

	body, err := libCommons.StructToJSONString(payload)
	if err != nil {
		return nil, err
	}

	hash := libCommons.HashSHA256(body)

	key := clientKey
	if key == "" {
		key = hash
	}

	internalKey := keyBuilder(key)

	c.Set(libConstants.IdempotencyReplayed, "false")

	result, err := uc.CreateOrCheckCRMIdempotency(ctx, internalKey, hash, ttl)
	if err != nil {
		return nil, err
	}

	if result.Replay != nil {
		c.Set(libConstants.IdempotencyReplayed, "true")
	}

	return &crmIdempotencyClaim{
		InternalKey: internalKey,
		ReplayJSON:  result.Replay,
		TTL:         ttl,
	}, nil
}
