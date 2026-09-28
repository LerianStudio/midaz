// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"fmt"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

const (
	// accountClosingMarkerNamespace and accountAdminOwnershipNamespace are the key
	// namespaces the reconciliation walks. They match the namespaces the key
	// builders use; the scan discovers what a restart left behind, so it cannot
	// depend on anything held in memory.
	accountClosingMarkerNamespace  = "account-closing"
	accountAdminOwnershipNamespace = "account-admin-ownership"

	// accountProtectionScopeSegments is how many trailing segments of a protection
	// key carry its scope: organization, ledger and account. Reading them from the
	// END is what keeps the parsing independent of the tenant prefix the adapter
	// puts in front.
	accountProtectionScopeSegments = 3

	// accountProtectionKeySeparator is the separator the protection key builders
	// use between segments.
	accountProtectionKeySeparator = ":"
)

// AccountProtectionScope names one account whose protection key was discovered.
// The scope is always complete, because a protection key can never be resolved
// from the account alone.
type AccountProtectionScope struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	AccountID      uuid.UUID
}

// AccountProtectionScanPage is one bounded page of a protection namespace.
//
// Cursor is where the walk continues; zero means the namespace returned its
// terminal cursor. An empty page is NOT terminal on its own — SCAN may return no
// key for a slot it passed — so only the terminal cursor completes a walk.
//
// Unreadable counts the keys of this page whose scope could not be parsed. They
// are reported rather than dropped silently: a key nobody can attribute is a
// protection nobody can resolve.
type AccountProtectionScanPage struct {
	Scopes     []AccountProtectionScope
	Cursor     uint64
	Unreadable int
}

// Complete reports whether this page closed the walk of its namespace.
func (page AccountProtectionScanPage) Complete() bool {
	return page.Cursor == 0
}

// ScanAccountClosingMarkers reads one bounded page of the closing-marker
// namespace, so reconciliation can find the attempts a restart interrupted
// without an unbounded key read. The cursor belongs to the caller.
func (rr *RedisConsumerRepository) ScanAccountClosingMarkers(ctx context.Context, cursor uint64, count int64) (AccountProtectionScanPage, error) {
	return rr.scanAccountProtection(ctx, "redis.scan_account_closing_markers", accountClosingMarkerNamespace, cursor, count)
}

// ScanAccountAdminOwnerships reads one bounded page of the administrative
// ownership namespace, which is how an ownership left behind by an operation
// other than a closing is discovered.
func (rr *RedisConsumerRepository) ScanAccountAdminOwnerships(ctx context.Context, cursor uint64, count int64) (AccountProtectionScanPage, error) {
	return rr.scanAccountProtection(ctx, "redis.scan_account_admin_ownerships", accountAdminOwnershipNamespace, cursor, count)
}

func (rr *RedisConsumerRepository) scanAccountProtection(ctx context.Context, spanName, namespace string, cursor uint64, count int64) (AccountProtectionScanPage, error) {
	if err := ctx.Err(); err != nil {
		return AccountProtectionScanPage{}, fmt.Errorf("scan account protection keys: %w", err)
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	pattern, err := tenantKeyFromContextOrError(ctx, namespace+accountProtectionKeySeparator+cachepolicy.HashTag+accountProtectionKeySeparator+"*")
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace the protection scan pattern", err)

		return AccountProtectionScanPage{}, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get Redis client", err)

		return AccountProtectionScanPage{}, fmt.Errorf("get account protection scan client: %w", err)
	}

	if count <= 0 {
		count = DefaultRecoveryScanCount
	}

	if count > MaxRecoveryScanCount {
		count = MaxRecoveryScanCount
	}

	keys, nextCursor, err := rds.Scan(ctx, cursor, pattern, count).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to scan the account protection keys", err)

		return AccountProtectionScanPage{}, fmt.Errorf("scan account protection keys: %w", err)
	}

	page := AccountProtectionScanPage{Cursor: nextCursor, Scopes: make([]AccountProtectionScope, 0, len(keys))}

	for _, key := range keys {
		scope, err := parseAccountProtectionScope(key)
		if err != nil {
			page.Unreadable++

			continue
		}

		page.Scopes = append(page.Scopes, scope)
	}

	span.SetAttributes(
		attribute.Int("db.rows_returned", len(page.Scopes)),
		attribute.Int("app.account_protection_scan.unreadable_keys", page.Unreadable),
	)

	return page, nil
}

// parseAccountProtectionScope reads the scope out of one protection key. The
// three identifiers are the last segments, so a tenant prefix of any shape in
// front of the namespace changes nothing here.
func parseAccountProtectionScope(key string) (AccountProtectionScope, error) {
	segments := strings.Split(key, accountProtectionKeySeparator)
	if len(segments) < accountProtectionScopeSegments {
		return AccountProtectionScope{}, fmt.Errorf("account protection key %q carries no complete scope", key)
	}

	tail := segments[len(segments)-accountProtectionScopeSegments:]

	organizationID, err := uuid.Parse(tail[0])
	if err != nil {
		return AccountProtectionScope{}, fmt.Errorf("account protection key %q has an invalid organization: %w", key, err)
	}

	ledgerID, err := uuid.Parse(tail[1])
	if err != nil {
		return AccountProtectionScope{}, fmt.Errorf("account protection key %q has an invalid ledger: %w", key, err)
	}

	accountID, err := uuid.Parse(tail[2])
	if err != nil {
		return AccountProtectionScope{}, fmt.Errorf("account protection key %q has an invalid account: %w", key, err)
	}

	return AccountProtectionScope{OrganizationID: organizationID, LedgerID: ledgerID, AccountID: accountID}, nil
}
