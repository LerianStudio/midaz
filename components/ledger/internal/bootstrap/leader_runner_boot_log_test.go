// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/redaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The logger masks every field whose name matches a sensitive token ("pass",
// "expiry", ...). A boot log that names its settings that way reaches the
// operator as [REDACTED], so every field of these lines must stay clear of
// the redaction list and still carry its value.
func requireBootLogFieldsReadable(t *testing.T, fields map[string]any, want map[string]any) {
	t.Helper()

	for key := range fields {
		assert.Falsef(t, redaction.IsSensitiveField(key), "boot log field %q is masked by the logger", key)
	}

	for key, value := range want {
		require.Containsf(t, fields, key, "boot log misses %q", key)
		assert.EqualValuesf(t, value, fields[key], "boot log field %q", key)
	}
}

func TestTenantHealthProberBootLogFieldsAreNotRedacted(t *testing.T) {
	w := newTenantHealthWiring(t)
	logger := &proberCapturingLogger{Logger: libLog.NewNop()}

	prober := initTenantHealthProber(enabledTenantHealthProbeConfig(), logger, w.redis, true,
		w.cache, w.client, "ledger", w.onbPG, w.txnPG, w.onbMgo, w.txnMgo, w.crmMgo, nil)
	require.NotNil(t, prober)

	configured := logger.withMessage("Tenant health prober configured")
	require.Len(t, configured, 1)

	requireBootLogFieldsReadable(t, configured[0].fields, map[string]any{
		"budget_ms":   int64(30000),
		"lock_ttl_ms": prober.lease.(*redisReconcilerLease).expiry.Milliseconds(),
	})
}

func TestBalanceReconcilerBootLogFieldsAreNotRedacted(t *testing.T) {
	w := newTenantHealthWiring(t)
	logger := &proberCapturingLogger{Logger: libLog.NewNop()}

	runner := initBalanceReconcilerRunner(&Config{BalanceReconcilerEnabled: true}, logger, nil, w.redis,
		false, nil, nil, nil, "", nil)
	require.NotNil(t, runner)

	configured := logger.withMessage("Balance reconciler configured")
	require.Len(t, configured, 1)

	requireBootLogFieldsReadable(t, configured[0].fields, map[string]any{
		"budget_ms":   runner.cfg.PassBudgetMs,
		"lock_ttl_ms": balanceReconcilerLeaseExpiry(runner.cfg).Milliseconds(),
	})
}
