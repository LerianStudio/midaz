// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/sanitize"
)

// snapshotRequestWithSensitiveMetadata builds a validation request that carries
// the same sensitive keys at the top level and inside every nested context, so a
// single fixture exercises all five metadata maps the snapshot copies.
func snapshotRequestWithSensitiveMetadata() *model.ValidationRequest {
	sensitive := func() map[string]any {
		return map[string]any{
			"cpf":         "123.456.789-09",
			"email":       "holder@example.com",
			"card_number": "4111111111111111",
			"dob":         "1980-02-29",
			"tier":        "gold",
			"nested": map[string]any{
				"password": "hunter2",
				"channel":  "mobile",
			},
		}
	}

	return &model.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.NewFromInt(100),
		Asset:                "USD",
		TransactionTimestamp: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Account: model.AccountContext{
			ID:       testutil.MustDeterministicUUID(2),
			Type:     "checking",
			Status:   "active",
			Metadata: sensitive(),
		},
		Segment:   &model.SegmentContext{ID: testutil.MustDeterministicUUID(3), Name: "retail", Metadata: sensitive()},
		Portfolio: &model.PortfolioContext{ID: testutil.MustDeterministicUUID(4), Name: "growth", Metadata: sensitive()},
		Merchant:  &model.MerchantContext{ID: testutil.MustDeterministicUUID(5), Name: "Acme", Category: "5411", Country: "US", Metadata: sensitive()},
		Metadata:  sensitive(),
	}
}

// TestBuildRequestSnapshotRedactsEveryMetadataMap pins the data-minimization
// contract for the audit snapshot. The audit_events row is immutable by database
// rule (UPDATE and DELETE are discarded), so a value that lands here verbatim
// can never be scrubbed. The redaction the validation record already applies
// must therefore apply to the snapshot too, at the top level and inside the
// account, segment, portfolio and merchant contexts.
func TestBuildRequestSnapshotRedactsEveryMetadataMap(t *testing.T) {
	t.Parallel()

	req := snapshotRequestWithSensitiveMetadata()
	snapshot := buildRequestSnapshot(req)

	metadataOf := func(t *testing.T, path ...string) map[string]any {
		t.Helper()

		current := snapshot

		for _, key := range path {
			next, ok := current[key].(map[string]any)
			require.True(t, ok, "expected a map at %q", key)
			current = next
		}

		return current
	}

	cases := map[string][]string{
		"request":   {"metadata"},
		"account":   {"account", "metadata"},
		"segment":   {"segment", "metadata"},
		"portfolio": {"portfolio", "metadata"},
		"merchant":  {"merchant", "metadata"},
	}

	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			metadata := metadataOf(t, path...)

			for _, key := range []string{"cpf", "email", "card_number", "dob"} {
				require.Equal(t, sanitize.MaskedValue, metadata[key],
					"%s metadata key %q must be redacted in the immutable audit snapshot", name, key)
			}

			require.Equal(t, "gold", metadata["tier"],
				"%s metadata must keep the values that are not sensitive", name)

			nested, ok := metadata["nested"].(map[string]any)
			require.True(t, ok, "%s nested metadata must survive as a map", name)
			require.Equal(t, sanitize.MaskedValue, nested["password"],
				"%s metadata must be redacted at depth, not only at the first level", name)
			require.Equal(t, "mobile", nested["channel"])
		})
	}
}

// TestBuildRequestSnapshotLeavesTheRequestUntouched guards the caller's copy:
// redaction happens on the snapshot, so the decision path keeps evaluating rules
// against the values the client actually sent.
func TestBuildRequestSnapshotLeavesTheRequestUntouched(t *testing.T) {
	t.Parallel()

	req := snapshotRequestWithSensitiveMetadata()
	_ = buildRequestSnapshot(req)

	require.Equal(t, "123.456.789-09", req.Metadata["cpf"])
	require.Equal(t, "123.456.789-09", req.Account.Metadata["cpf"])
	require.Equal(t, "123.456.789-09", req.Segment.Metadata["cpf"])
	require.Equal(t, "123.456.789-09", req.Portfolio.Metadata["cpf"])
	require.Equal(t, "123.456.789-09", req.Merchant.Metadata["cpf"])
}

// TestBuildRequestSnapshotKeepsAbsentMetadataAbsent keeps the null-versus-empty
// distinction the sanitizer preserves, so an audit reader can still tell a
// request that sent no metadata from one that sent an empty map.
func TestBuildRequestSnapshotKeepsAbsentMetadataAbsent(t *testing.T) {
	t.Parallel()

	req := &model.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.NewFromInt(100),
		Asset:                "USD",
		TransactionTimestamp: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Account:              model.AccountContext{ID: testutil.MustDeterministicUUID(2)},
	}

	snapshot := buildRequestSnapshot(req)

	require.Nil(t, snapshot["metadata"])

	account, ok := snapshot["account"].(map[string]any)
	require.True(t, ok)
	require.Nil(t, account["metadata"])

	require.NotContains(t, snapshot, "segment")
	require.NotContains(t, snapshot, "portfolio")
	require.NotContains(t, snapshot, "merchant")
}
