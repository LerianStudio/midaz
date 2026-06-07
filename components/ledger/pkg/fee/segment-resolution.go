// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package fee provides utilities for calculating transaction fees based on various rules and package configurations.
package fee

import (
	"context"
	"fmt"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/google/uuid"
)

// resolveSegmentWaivedAccounts splits a waivedAccounts slice into two groups:
//   - directAliases: entries without the "segment:" prefix, preserved as-is for exact matching
//   - segmentIDs: parsed UUIDs from entries with the "segment:" prefix, used for segment-based exemption
//
// Returns an error if any entry has the "segment:" prefix but contains an invalid UUID,
// since this indicates a configuration error that could silently disable fee exemptions.
// This function makes no external calls and is safe to call without a resolver.
func resolveSegmentWaivedAccounts(waivedAccounts []string) (directAliases []string, segmentIDs []uuid.UUID, err error) {
	for _, entry := range waivedAccounts {
		isSegment, segID, parseErr := isSegmentReference(entry)
		if parseErr != nil {
			return nil, nil, parseErr
		}

		if isSegment {
			segmentIDs = append(segmentIDs, segID)
		} else {
			directAliases = append(directAliases, entry)
		}
	}

	return directAliases, segmentIDs, nil
}

// isAccountExemptWithSegments checks whether an account is exempt from fee distribution.
// It first performs a fast-path exact alias match against directAliases.
// If no direct match is found and segmentIDs is non-empty, it resolves the account's
// segment via the in-process resolver and checks whether the account belongs to any
// of the waived segments.
//
// Returns an error if segment resolution fails, so the caller can decide to fail the
// billing rather than silently charging exempt accounts.
//
// Non-error false cases:
//   - If segmentIDs is empty, no resolution is performed and (false, nil) is returned.
//   - If resolver is nil, segment resolution is skipped and (false, nil) is returned.
//   - If the account has a nil SegmentID, (false, nil) is returned.
func isAccountExemptWithSegments(
	ctx context.Context,
	account string,
	directAliases *[]string,
	segmentIDs []uuid.UUID,
	resolver feeshared.MidazResolver,
	organizationID, ledgerID uuid.UUID,
) (bool, error) {
	// Fast path: exact alias match avoids any resolution call.
	if isAccountExempt(account, directAliases) {
		return true, nil
	}

	// External accounts (e.g. @external/BRL) are virtual accounts with no
	// real entity — they cannot belong to a segment and are never exempt.
	if strings.HasPrefix(account, "@external/") {
		return false, nil
	}

	// No segment IDs configured — nothing to resolve.
	if len(segmentIDs) == 0 {
		return false, nil
	}

	// Cannot resolve segment without a resolver.
	if resolver == nil {
		return false, nil
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "fee.segment_resolution.check_account_segment")
	defer span.End()

	accountDetails, err := resolver.GetAccountByAlias(ctx, organizationID, ledgerID, account)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve account segment for exemption check", err)

		return false, fmt.Errorf("segment resolution failed for account %s: %w", account, err)
	}

	if accountDetails == nil {
		return false, nil
	}

	if accountDetails.SegmentID == nil {
		return false, nil
	}

	for _, segID := range segmentIDs {
		if *accountDetails.SegmentID == segID {
			return true, nil
		}
	}

	return false, nil
}
