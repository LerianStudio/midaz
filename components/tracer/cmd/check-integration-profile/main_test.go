// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	validLedgerActivation = "TRACER_CONTEXT_ENABLED=true\nTRACER_INTEGRATION_ID=ledger\nTRACER_ASSET_NAMESPACE=midaz\n"
	validTracerActivation = "CONTEXT_RESERVE_ENABLED=true\nCONTEXT_PRODUCER_BINDINGS='[{\"uri\":\"spiffe://example.test/ledger\",\"integrationId\":\"ledger\",\"assetNamespace\":\"midaz\",\"purposes\":[\"reserve\"]}]'\n"
)

func TestProfileCheckDetectsPrecisionAndEnvelopeDrift(t *testing.T) {
	dir := t.TempDir()
	ledger, tracer := filepath.Join(dir, "ledger.env"), filepath.Join(dir, "tracer.env")
	require.NoError(t, os.WriteFile(ledger, []byte(validLedgerActivation), 0o600))
	for _, test := range []struct {
		input string
		valid bool
	}{
		{validTracerActivation, true},
		{validTracerActivation + "CONTEXT_MAX_FRACTION_DIGITS=128\n", true},
		{validTracerActivation + "CONTEXT_MAX_FRACTION_DIGITS=8\n", false},
		{validTracerActivation + "CONTEXT_MAX_FRACTION_DIGITS=0\n", false},
		{validTracerActivation + "CONTEXT_RESERVE_MAX_BODY_BYTES=10\n", false},
		{validTracerActivation + "CONTEXT_MAX_ACCOUNTS=0\n", false},
		{validTracerActivation + "CONTEXT_MAX_FRACTION_DIGITS=\n", false},
	} {
		require.NoError(t, os.WriteFile(tracer, []byte(test.input), 0o600))
		err := checkProfiles(ledger, tracer)
		if test.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	require.NoError(t, os.WriteFile(tracer, []byte("SECRET='private-test-material"), 0o600))
	err := checkProfiles(ledger, tracer)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private-test-material")
}

func TestProfileCheckRejectsActivationDrift(t *testing.T) {
	dir := t.TempDir()
	ledgerPath, tracerPath := filepath.Join(dir, "ledger.env"), filepath.Join(dir, "tracer.env")

	tests := []struct {
		name   string
		ledger string
		tracer string
	}{
		{name: "invalid ledger boolean", ledger: "TRACER_CONTEXT_ENABLED=yes\nTRACER_INTEGRATION_ID=ledger\nTRACER_ASSET_NAMESPACE=midaz\n", tracer: validTracerActivation},
		{name: "tracer reserve disabled", ledger: validLedgerActivation, tracer: "CONTEXT_RESERVE_ENABLED=false\nCONTEXT_PRODUCER_BINDINGS='[{\"uri\":\"spiffe://example.test/ledger\",\"integrationId\":\"ledger\",\"assetNamespace\":\"midaz\",\"purposes\":[\"reserve\"]}]'\n"},
		{name: "identity differs", ledger: validLedgerActivation, tracer: "CONTEXT_RESERVE_ENABLED=true\nCONTEXT_PRODUCER_BINDINGS='[{\"uri\":\"spiffe://example.test/ledger\",\"integrationId\":\"another\",\"assetNamespace\":\"midaz\",\"purposes\":[\"reserve\"]}]'\n"},
		{name: "namespace differs", ledger: validLedgerActivation, tracer: "CONTEXT_RESERVE_ENABLED=true\nCONTEXT_PRODUCER_BINDINGS='[{\"uri\":\"spiffe://example.test/ledger\",\"integrationId\":\"ledger\",\"assetNamespace\":\"other\",\"purposes\":[\"reserve\"]}]'\n"},
		{name: "purpose differs", ledger: validLedgerActivation, tracer: "CONTEXT_RESERVE_ENABLED=true\nCONTEXT_PRODUCER_BINDINGS='[{\"uri\":\"spiffe://example.test/admin\",\"integrationId\":\"ledger\",\"assetNamespace\":\"midaz\",\"purposes\":[\"asset-admin\"]}]'\n"},
		{name: "recovery cannot cover batch", ledger: validLedgerActivation + "TRANSACTION_BATCH_MAX_SIZE=50\nTRACER_RECOVERY_BATCH_SIZE=10\n", tracer: validTracerActivation},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(ledgerPath, []byte(test.ledger), 0o600))
			require.NoError(t, os.WriteFile(tracerPath, []byte(test.tracer), 0o600))
			require.Error(t, checkProfiles(ledgerPath, tracerPath))
		})
	}
}

func TestProfileCheckAcceptsMatchingRotationBindingsAndBatch(t *testing.T) {
	dir := t.TempDir()
	ledgerPath, tracerPath := filepath.Join(dir, "ledger.env"), filepath.Join(dir, "tracer.env")
	ledger := validLedgerActivation + "TRANSACTION_BATCH_MAX_SIZE=50\nTRACER_RECOVERY_BATCH_SIZE=50\n"
	bindings := `[{"uri":"spiffe://example.test/old","integrationId":"ledger","assetNamespace":"midaz","purposes":["reserve"]},{"uri":"spiffe://example.test/new","integrationId":"ledger","assetNamespace":"midaz","purposes":["reserve"]}]`
	tracer := fmt.Sprintf("CONTEXT_RESERVE_ENABLED=true\nCONTEXT_PRODUCER_BINDINGS='%s'\n", bindings)
	require.NoError(t, os.WriteFile(ledgerPath, []byte(ledger), 0o600))
	require.NoError(t, os.WriteFile(tracerPath, []byte(tracer), 0o600))
	require.NoError(t, checkProfiles(ledgerPath, tracerPath))
}
