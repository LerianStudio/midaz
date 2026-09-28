// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package producerauth_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	ledgerClientID = "ledger-m2m-client"
	ledgerCertURI  = "spiffe://example.test/service/ledger"
)

func TestParsePlatformProducersResolvesByClientID(t *testing.T) {
	t.Parallel()

	registry, err := producerauth.ParsePlatformProducers(
		`[{"service":"ledger","clientId":"` + ledgerClientID + `","certUri":"` + ledgerCertURI + `"},` +
			`{"service":"ledger","clientId":"ledger-rotated"}]`,
	)
	require.NoError(t, err)

	for _, clientID := range []string{ledgerClientID, "ledger-rotated"} {
		producer, ok := registry.ByClientID(clientID)
		require.True(t, ok)
		require.Equal(t, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken}, producer)
	}

	for _, clientID := range []string{"", "unknown", ledgerCertURI, strings.ToUpper(ledgerClientID)} {
		producer, ok := registry.ByClientID(clientID)
		require.False(t, ok, clientID)
		require.Zero(t, producer)
	}

	var absent *producerauth.Registry

	producer, ok := absent.ByClientID(ledgerClientID)
	require.False(t, ok)
	require.Zero(t, producer)
}

func TestParsePlatformProducersAcceptsSingleCredentialEntries(t *testing.T) {
	t.Parallel()

	tokenOnly, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + ledgerClientID + `"}]`)
	require.NoError(t, err)

	_, ok := tokenOnly.ByClientID(ledgerClientID)
	require.True(t, ok)

	certOnly, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","certUri":"` + ledgerCertURI + `"}]`)
	require.NoError(t, err)

	_, ok = certOnly.ByClientID(ledgerClientID)
	require.False(t, ok)
}

func TestParsePlatformProducersRefusesInvalidConfiguration(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"empty input", ""},
		{"oversized input", `[` + strings.Repeat(" ", 65536) + `]`},
		{"not JSON", `ledger`},
		{"object instead of array", `{"service":"ledger","clientId":"a"}`},
		{"trailing document", `[{"service":"ledger","clientId":"a"}] []`},
		{"empty array", `[]`},
		{"null", `null`},
		{"unknown field", `[{"service":"ledger","clientId":"a","integrationId":"x"}]`},
		{"service outside roster", `[{"service":"crm","clientId":"a"}]`},
		{"empty service", `[{"clientId":"a"}]`},
		{"noncanonical service", `[{"service":"Ledger","clientId":"a"}]`},
		{"no credential", `[{"service":"ledger"}]`},
		{"empty credentials", `[{"service":"ledger","clientId":"","certUri":""}]`},
		{"duplicate clientId", `[{"service":"ledger","clientId":"a"},{"service":"ledger","clientId":"a"}]`},
		{"padded clientId", `[{"service":"ledger","clientId":" a"}]`},
		{"control char clientId", `[{"service":"ledger","clientId":"a\u0007"}]`},
		{"oversized clientId", `[{"service":"ledger","clientId":"` + strings.Repeat("a", 257) + `"}]`},
		{"duplicate certUri", `[{"service":"ledger","certUri":"` + ledgerCertURI + `"},{"service":"ledger","certUri":"` + ledgerCertURI + `"}]`},
		{"relative certUri", `[{"service":"ledger","certUri":"ledger"}]`},
		{"certUri without host", `[{"service":"ledger","certUri":"spiffe:///ledger"}]`},
		{"certUri with user", `[{"service":"ledger","certUri":"spiffe://user@example.test/ledger"}]`},
		{"certUri with query", `[{"service":"ledger","certUri":"spiffe://example.test/ledger?x=1"}]`},
		{"certUri with empty query", `[{"service":"ledger","certUri":"spiffe://example.test/ledger?"}]`},
		{"certUri with fragment", `[{"service":"ledger","certUri":"spiffe://example.test/ledger#x"}]`},
		{"wildcard certUri", `[{"service":"ledger","certUri":"spiffe://*.test/ledger"}]`},
		{"certUri with space", `[{"service":"ledger","certUri":"spiffe://example.test/led ger"}]`},
		{"noncanonical certUri", `[{"service":"ledger","certUri":"SPIFFE://example.test/ledger"}]`},
		{"valid entry beside invalid one", `[{"service":"ledger","clientId":"a"},{"service":"other","clientId":"b"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			registry, err := producerauth.ParsePlatformProducers(tc.raw)
			require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
			require.Nil(t, registry)
		})
	}
}

func TestRegistryCredentialsListsTheServiceMappings(t *testing.T) {
	t.Parallel()

	registry, err := producerauth.ParsePlatformProducers(
		`[{"service":"ledger","clientId":"ledger-rotated","certUri":"` + ledgerCertURI + `"},` +
			`{"service":"ledger","clientId":"` + ledgerClientID + `"}]`,
	)
	require.NoError(t, err)

	clientIDs, certURIs := registry.Credentials(producerauth.ServiceLedger)
	require.Equal(t, []string{ledgerClientID, "ledger-rotated"}, clientIDs)
	require.Equal(t, []string{ledgerCertURI}, certURIs)

	clientIDs, certURIs = registry.Credentials("admin")
	require.Empty(t, clientIDs)
	require.Empty(t, certURIs)

	var absent *producerauth.Registry

	clientIDs, certURIs = absent.Credentials(producerauth.ServiceLedger)
	require.Nil(t, clientIDs)
	require.Nil(t, certURIs)
}

func TestInRosterAcceptsOnlyPlatformProducers(t *testing.T) {
	t.Parallel()

	require.True(t, producerauth.InRoster(producerauth.ServiceLedger))

	for _, service := range []string{"", "admin", "Ledger", " ledger"} {
		require.False(t, producerauth.InRoster(service), service)
	}
}
