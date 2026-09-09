// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// nestedMetadataRequest returns a request whose nested contexts are all present
// with empty metadata, ready for a single map to be overfilled per subtest.
func nestedMetadataRequest() *ValidationRequest {
	return &ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1),
		TransactionType:      TransactionTypeCard,
		Amount:               decimal.RequireFromString("10"),
		Asset:                "USD",
		TransactionTimestamp: testutil.FixedTime(),
		Account: AccountContext{
			ID:     testutil.MustDeterministicUUID(2),
			Type:   "checking",
			Status: "active",
		},
		Segment:   &SegmentContext{ID: testutil.MustDeterministicUUID(3)},
		Portfolio: &PortfolioContext{ID: testutil.MustDeterministicUUID(4)},
		Merchant: &MerchantContext{
			ID:       testutil.MustDeterministicUUID(5),
			Name:     "Acme",
			Category: "5411",
			Country:  "US",
		},
	}
}

// nestedMetadataSetters names each nested metadata map the request carries, so
// every ceiling is asserted against all four rather than only the one a single
// example happens to use.
func nestedMetadataSetters() map[string]func(*ValidationRequest, map[string]any) {
	return map[string]func(*ValidationRequest, map[string]any){
		"account":   func(r *ValidationRequest, m map[string]any) { r.Account.Metadata = m },
		"segment":   func(r *ValidationRequest, m map[string]any) { r.Segment.Metadata = m },
		"portfolio": func(r *ValidationRequest, m map[string]any) { r.Portfolio.Metadata = m },
		"merchant":  func(r *ValidationRequest, m map[string]any) { r.Merchant.Metadata = m },
	}
}

// TestValidateMetadataCoversNestedContexts holds the nested context metadata to
// the same ceilings the top-level map answers to. MaxMetadataEntries and
// MaxMetadataKeyLength are declared for the context objects, but only the
// top-level map was ever measured against them, so a client could push
// unbounded metadata through any nested object and past the advertised limits.
func TestValidateMetadataCoversNestedContexts(t *testing.T) {
	t.Parallel()

	tooManyEntries := func() map[string]any {
		metadata := make(map[string]any, trcConstant.MaxMetadataEntries+1)
		for i := 0; i <= trcConstant.MaxMetadataEntries; i++ {
			metadata[fmt.Sprintf("key%d", i)] = i
		}

		return metadata
	}

	cases := map[string]struct {
		metadata func() map[string]any
		wantErr  error
	}{
		"entry count": {metadata: tooManyEntries, wantErr: constant.ErrMetadataEntriesExceeded},
		"key length": {
			metadata: func() map[string]any {
				return map[string]any{strings.Repeat("a", trcConstant.MaxMetadataKeyLength+1): "value"}
			},
			wantErr: constant.ErrMetadataKeyLengthExceeded,
		},
		"key characters": {
			metadata: func() map[string]any { return map[string]any{"invalid-key": "value"} },
			wantErr:  constant.ErrMetadataKeyInvalidChars,
		},
	}

	for ceiling, testCase := range cases {
		for context, setMetadata := range nestedMetadataSetters() {
			t.Run(ceiling+"/"+context, func(t *testing.T) {
				t.Parallel()

				req := nestedMetadataRequest()
				setMetadata(req, testCase.metadata())

				err := req.Validate(testutil.FixedTime())
				require.Error(t, err, "%s metadata must answer to the %s ceiling", context, ceiling)
				assert.ErrorIs(t, err, testCase.wantErr)

				reserveErr := req.ValidateForReserve(testutil.FixedTime())
				require.Error(t, reserveErr, "the reserve path shares the same ceilings")
				assert.ErrorIs(t, reserveErr, testCase.wantErr)
			})
		}
	}
}

// TestValidateMetadataAcceptsNestedContextsAtTheBoundary keeps the ceilings
// inclusive: a nested map exactly at the maximum entry count and key length
// still validates, so the limits reject only what exceeds them.
func TestValidateMetadataAcceptsNestedContextsAtTheBoundary(t *testing.T) {
	t.Parallel()

	atLimit := func() map[string]any {
		metadata := make(map[string]any, trcConstant.MaxMetadataEntries)
		for i := 0; i < trcConstant.MaxMetadataEntries-1; i++ {
			metadata[fmt.Sprintf("key%d", i)] = i
		}

		metadata[strings.Repeat("a", trcConstant.MaxMetadataKeyLength)] = "value"

		return metadata
	}

	for context, setMetadata := range nestedMetadataSetters() {
		t.Run(context, func(t *testing.T) {
			t.Parallel()

			req := nestedMetadataRequest()
			setMetadata(req, atLimit())

			require.NoError(t, req.Validate(testutil.FixedTime()))
			require.NoError(t, req.ValidateForReserve(testutil.FixedTime()))
		})
	}
}

// TestValidateMetadataAcceptsAbsentNestedContexts guards the common shape the
// ledger sends over the reserve seam: nested contexts carrying only an id, with
// no metadata at all.
func TestValidateMetadataAcceptsAbsentNestedContexts(t *testing.T) {
	t.Parallel()

	req := nestedMetadataRequest()

	require.NoError(t, req.Validate(testutil.FixedTime()))
	require.NoError(t, req.ValidateForReserve(testutil.FixedTime()))

	bare := nestedMetadataRequest()
	bare.Segment = nil
	bare.Portfolio = nil
	bare.Merchant = nil

	require.NoError(t, bare.Validate(testutil.FixedTime()))
	require.NoError(t, bare.ValidateForReserve(testutil.FixedTime()))
}
