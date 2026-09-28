// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	ledgerClientID = "ledger-m2m"
	ledgerSecret   = "ledger-m2m-secret"
	ledgerCertURI  = "spiffe://example.test/ledger"

	grpcLedger       = "TRACER_BASE_URL=https://tracer.example.test:4021\nTRACER_TLS_MODE=mtls\n"
	pluginAuth       = "PLUGIN_AUTH_ENABLED=true\nPLUGIN_AUTH_HOST=http://plugin-auth:4000\n"
	clientCredential = "IDP_M2M_CLIENT_ID=" + ledgerClientID + "\nIDP_M2M_CLIENT_SECRET=" + ledgerSecret + "\n"
	restLedger       = "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\n" + clientCredential + pluginAuth

	bothProducers = "TRACER_PLATFORM_PRODUCERS='[{\"service\":\"ledger\",\"clientId\":\"" + ledgerClientID + "\",\"certUri\":\"" + ledgerCertURI + "\"}]'\n"
	tokenProducer = "TRACER_PLATFORM_PRODUCERS='[{\"service\":\"ledger\",\"clientId\":\"" + ledgerClientID + "\"}]'\n"
	certProducer  = "TRACER_PLATFORM_PRODUCERS='[{\"service\":\"ledger\",\"certUri\":\"" + ledgerCertURI + "\"}]'\n"

	grpcPort   = "TRACER_GRPC_PORT=:4021\n"
	tokenCheck = "CONTEXT_M2M_JWKS_URL=https://access-manager.example.test/.well-known/jwks\nCONTEXT_M2M_ISSUER=https://access-manager.example.test\n"
	grpcTracer = "TRACER_TLS_MODE=mtls\n" + grpcPort + tokenCheck + bothProducers
	restTracer = tokenCheck + tokenProducer

	multiTenant   = "MULTI_TENANT_ENABLED=true\n"
	tenantManager = "MULTI_TENANT_URL=https://tenant-manager.example.test\nMULTI_TENANT_SERVICE_API_KEY=svc-api-key\n"
	httpsREST     = "TRACER_BASE_URL=https://tracer:4020\nTRACER_TRANSPORT=rest\n" + clientCredential + "PLUGIN_AUTH_ENABLED=true\nPLUGIN_AUTH_HOST=https://plugin-auth:4000\n"
)

func writeProfiles(t *testing.T, ledger, tracer string) (string, string) {
	t.Helper()

	dir := t.TempDir()
	ledgerPath, tracerPath := filepath.Join(dir, "ledger.env"), filepath.Join(dir, "tracer.env")
	require.NoError(t, os.WriteFile(ledgerPath, []byte(ledger), 0o600))
	require.NoError(t, os.WriteFile(tracerPath, []byte(tracer), 0o600))

	return ledgerPath, tracerPath
}

func TestProfileCheckDetectsPrecisionAndEnvelopeDrift(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		input string
		valid bool
	}{
		{grpcTracer, true},
		{grpcTracer + "CONTEXT_MAX_FRACTION_DIGITS=128\n", true},
		{grpcTracer + "CONTEXT_MAX_FRACTION_DIGITS=8\n", false},
		{grpcTracer + "CONTEXT_MAX_FRACTION_DIGITS=0\n", false},
		{grpcTracer + "CONTEXT_RESERVE_MAX_BODY_BYTES=10\n", false},
		{grpcTracer + "CONTEXT_MAX_ACCOUNTS=0\n", false},
		{grpcTracer + "CONTEXT_MAX_FRACTION_DIGITS=\n", false},
	} {
		err := checkProfiles(writeProfiles(t, grpcLedger, test.input))
		if test.valid {
			require.NoError(t, err, test.input)
		} else {
			require.Error(t, err, test.input)
		}
	}

	err := checkProfiles(writeProfiles(t, grpcLedger, "SECRET='private-test-material"))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private-test-material")
}

func TestProfileCheckAcceptsBothTransports(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ledger string
		tracer string
	}{
		{name: "grpc by default", ledger: grpcLedger, tracer: grpcTracer},
		{name: "grpc explicit and case-insensitive", ledger: grpcLedger + "TRACER_TRANSPORT= GRPC \n", tracer: "TRACER_TLS_MODE=MTLS\n" + grpcPort + certProducer},
		{name: "grpc with rotating certificates", ledger: grpcLedger, tracer: "TRACER_TLS_MODE=mtls\n" + grpcPort + "TRACER_PLATFORM_PRODUCERS='[{\"service\":\"ledger\",\"certUri\":\"spiffe://example.test/old\"},{\"service\":\"ledger\",\"certUri\":\"spiffe://example.test/new\"}]'\n"},
		{name: "rest behind a mesh", ledger: restLedger, tracer: restTracer},
		{name: "rest over mtls", ledger: restLedger + "TRACER_TLS_MODE=mtls\n", tracer: grpcTracer},
		{name: "rest with a rotated client id", ledger: restLedger, tracer: tokenCheck + "TRACER_PLATFORM_PRODUCERS='[{\"service\":\"ledger\",\"clientId\":\"old\"},{\"service\":\"ledger\",\"clientId\":\"" + ledgerClientID + "\"}]'\n"},
		{name: "explicit ledger application name", ledger: restLedger + "APPLICATION_NAME=ledger\n", tracer: restTracer},
		{name: "rest to a local tracer without an issuer", ledger: restLedger, tracer: "DEPLOYMENT_MODE= Local \nCONTEXT_M2M_JWKS_URL=http://localhost:8000/.well-known/jwks\n" + tokenProducer},
		{name: "rest with plugin auth case-insensitive", ledger: "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\n" + clientCredential + "PLUGIN_AUTH_ENABLED= TRUE \nPLUGIN_AUTH_HOST=http://plugin-auth:4000\n", tracer: restTracer},
		{name: "rest to a local tracer without token verification settings", ledger: restLedger, tracer: "DEPLOYMENT_MODE=local\n" + tokenProducer},
		{name: "rest over https to an mtls tracer", ledger: "TRACER_BASE_URL=HTTPS://tracer:4020\nTRACER_TRANSPORT=rest\n" + clientCredential + pluginAuth, tracer: grpcTracer},
		{name: "rest through a mesh to an mtls tracer", ledger: restLedger + "TRACER_TLS_MODE=mesh\n", tracer: grpcTracer},
		{name: "rest resolving plugin auth through discovery", ledger: "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\n" + clientCredential + "PLUGIN_AUTH_ENABLED=true\nSD_ENABLED=true\n", tracer: restTracer},
		{name: "reaper explicitly on", ledger: grpcLedger, tracer: grpcTracer + "RESERVATION_REAPER_ENABLED=true\n"},
		{name: "unrelated ledger settings", ledger: grpcLedger + "TRANSACTION_BATCH_MAX_SIZE=50\n", tracer: grpcTracer},
		{name: "both multi-tenant", ledger: grpcLedger + multiTenant, tracer: grpcTracer + multiTenant + tenantManager},
		{name: "both multi-tenant as parsed at boot", ledger: grpcLedger + "MULTI_TENANT_ENABLED=1\n", tracer: grpcTracer + "MULTI_TENANT_ENABLED=TRUE\n" + tenantManager},
		{name: "both single-tenant as parsed at boot", ledger: grpcLedger + "MULTI_TENANT_ENABLED=yes\n", tracer: grpcTracer + "MULTI_TENANT_ENABLED=false\n"},
		{name: "saas rest over https", ledger: httpsREST + "DEPLOYMENT_MODE=saas\n", tracer: grpcTracer + "DEPLOYMENT_MODE=saas\n"},
		{name: "saas rest over http through an explicit mesh", ledger: restLedger + "DEPLOYMENT_MODE=SaaS\nTRACER_TLS_MODE=mesh\n", tracer: "TRACER_TLS_MODE=mesh\n" + restTracer},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.NoError(t, checkProfiles(writeProfiles(t, test.ledger, test.tracer)))
		})
	}
}

func TestProfileCheckRejectsIdentityDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ledger string
		tracer string
		// reason is a substring of the rejection, so a row cannot pass by
		// failing for an unrelated setting.
		reason string
	}{
		{name: "ledger integration off", ledger: "TRACER_TLS_MODE=mtls\n", tracer: grpcTracer, reason: "TRACER_BASE_URL must be set"},
		{name: "ledger base url blank", ledger: "TRACER_BASE_URL= \nTRACER_TLS_MODE=mtls\n", tracer: grpcTracer, reason: "TRACER_BASE_URL must be set"},
		{name: "ledger application outside the roster", ledger: grpcLedger + "APPLICATION_NAME=admin\n", tracer: grpcTracer, reason: "APPLICATION_NAME is not a platform producer"},
		{name: "producers missing", ledger: grpcLedger, tracer: "TRACER_TLS_MODE=mtls\n", reason: "invalid TRACER_PLATFORM_PRODUCERS"},
		{name: "producer outside the roster", ledger: grpcLedger, tracer: "TRACER_TLS_MODE=mtls\nTRACER_PLATFORM_PRODUCERS='[{\"service\":\"admin\",\"certUri\":\"" + ledgerCertURI + "\"}]'\n", reason: "invalid TRACER_PLATFORM_PRODUCERS"},
		{name: "producer carries an unknown field", ledger: grpcLedger, tracer: "TRACER_TLS_MODE=mtls\nTRACER_PLATFORM_PRODUCERS='[{\"service\":\"ledger\",\"certUri\":\"" + ledgerCertURI + "\",\"tenantId\":\"acme\"}]'\n", reason: "invalid TRACER_PLATFORM_PRODUCERS"},
		{name: "unknown ledger transport", ledger: grpcLedger + "TRACER_TRANSPORT=amqp\n", tracer: grpcTracer, reason: "TRACER_TRANSPORT must be"},
		{name: "grpc without ledger mtls", ledger: "TRACER_BASE_URL=http://tracer:4021\n", tracer: grpcTracer, reason: "requires ledger TRACER_TLS_MODE=mtls"},
		{name: "grpc with ledger mesh", ledger: "TRACER_BASE_URL=http://tracer:4021\nTRACER_TLS_MODE=mesh\n", tracer: grpcTracer, reason: "requires ledger TRACER_TLS_MODE=mtls"},
		{name: "grpc without tracer mtls", ledger: grpcLedger, tracer: grpcPort + bothProducers, reason: "requires tracer TRACER_TLS_MODE=mtls"},
		{name: "grpc with tracer mesh", ledger: grpcLedger, tracer: "TRACER_TLS_MODE=mesh\n" + grpcPort + bothProducers, reason: "requires tracer TRACER_TLS_MODE=mtls"},
		{name: "grpc without a tracer grpc port", ledger: grpcLedger, tracer: "TRACER_TLS_MODE=mtls\n" + bothProducers, reason: "tracer TRACER_GRPC_PORT"},
		{name: "grpc with a blank tracer grpc port", ledger: grpcLedger, tracer: "TRACER_TLS_MODE=mtls\nTRACER_GRPC_PORT= \n" + bothProducers, reason: "tracer TRACER_GRPC_PORT"},
		{name: "grpc without a ledger certificate", ledger: grpcLedger, tracer: "TRACER_TLS_MODE=mtls\n" + grpcPort + tokenProducer, reason: "requires a certUri"},
		{name: "rest without a client id", ledger: "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\n" + pluginAuth, tracer: restTracer, reason: "requires IDP_M2M_CLIENT_ID"},
		{name: "rest with an unmapped client id", ledger: "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\nIDP_M2M_CLIENT_ID=someone-else\nIDP_M2M_CLIENT_SECRET=" + ledgerSecret + "\n" + pluginAuth, tracer: restTracer, reason: "is not a clientId"},
		{name: "rest over ledger mtls to a plaintext tracer", ledger: restLedger + "TRACER_TLS_MODE=mtls\n", tracer: restTracer, reason: "requires tracer TRACER_TLS_MODE=mtls"},
		{name: "rest over ledger mtls to a mesh tracer", ledger: restLedger + "TRACER_TLS_MODE=mtls\n", tracer: "TRACER_TLS_MODE=mesh\n" + restTracer, reason: "requires tracer TRACER_TLS_MODE=mtls"},
		{name: "rest without plugin auth", ledger: "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\n" + clientCredential + "PLUGIN_AUTH_HOST=http://plugin-auth:4000\n", tracer: restTracer, reason: "PLUGIN_AUTH_ENABLED=true"},
		{name: "rest with plugin auth disabled", ledger: "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\n" + clientCredential + "PLUGIN_AUTH_ENABLED=false\nPLUGIN_AUTH_HOST=http://plugin-auth:4000\n", tracer: restTracer, reason: "PLUGIN_AUTH_ENABLED=true"},
		{name: "rest with plugin auth unparsable", ledger: "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\n" + clientCredential + "PLUGIN_AUTH_ENABLED=yes\nPLUGIN_AUTH_HOST=http://plugin-auth:4000\n", tracer: restTracer, reason: "PLUGIN_AUTH_ENABLED=true"},
		{name: "rest without a plugin auth host", ledger: "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\n" + clientCredential + "PLUGIN_AUTH_ENABLED=true\nPLUGIN_AUTH_HOST= \n", tracer: restTracer, reason: "PLUGIN_AUTH_HOST"},
		{name: "rest without a tracer jwks url", ledger: restLedger, tracer: "CONTEXT_M2M_ISSUER=https://access-manager.example.test\n" + tokenProducer, reason: "CONTEXT_M2M_JWKS_URL"},
		{name: "rest without a tracer issuer", ledger: restLedger, tracer: "CONTEXT_M2M_JWKS_URL=https://access-manager.example.test/.well-known/jwks\n" + tokenProducer, reason: "CONTEXT_M2M_ISSUER"},
		{name: "rest without a tracer issuer outside local", ledger: restLedger, tracer: "DEPLOYMENT_MODE=byoc\nCONTEXT_M2M_JWKS_URL=https://access-manager.example.test/.well-known/jwks\n" + tokenProducer, reason: "CONTEXT_M2M_ISSUER"},
		{name: "rest with only a certificate mapped", ledger: restLedger, tracer: certProducer, reason: "is not a clientId"},
		{name: "rest without a client secret", ledger: "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\nIDP_M2M_CLIENT_ID=" + ledgerClientID + "\nIDP_M2M_CLIENT_SECRET= \n" + pluginAuth, tracer: restTracer, reason: "requires IDP_M2M_CLIENT_SECRET"},
		{name: "rest without a plugin auth host and discovery off", ledger: "TRACER_BASE_URL=http://tracer:4020\nTRACER_TRANSPORT=rest\n" + clientCredential + "PLUGIN_AUTH_ENABLED=true\nSD_ENABLED=false\n", tracer: restTracer, reason: "PLUGIN_AUTH_HOST unless ledger SD_ENABLED=true"},
		{name: "rest over plaintext to an mtls tracer", ledger: restLedger, tracer: grpcTracer, reason: "tracer TRACER_TLS_MODE=mtls serves HTTPS only"},
		{name: "rest over plaintext to an mtls tracer with an explicit empty mode", ledger: restLedger + "TRACER_TLS_MODE=\n", tracer: grpcTracer, reason: "tracer TRACER_TLS_MODE=mtls serves HTTPS only"},
		{name: "reaper disabled", ledger: grpcLedger, tracer: grpcTracer + "RESERVATION_REAPER_ENABLED=false\n", reason: "RESERVATION_REAPER_ENABLED must stay true"},
		{name: "reaper flag unparsable", ledger: grpcLedger, tracer: grpcTracer + "RESERVATION_REAPER_ENABLED=on\n", reason: "RESERVATION_REAPER_ENABLED must stay true"},
		{name: "reaper flag empty", ledger: grpcLedger, tracer: grpcTracer + "RESERVATION_REAPER_ENABLED=\n", reason: "RESERVATION_REAPER_ENABLED must stay true"},
		{name: "only the tracer multi-tenant", ledger: grpcLedger, tracer: grpcTracer + multiTenant + tenantManager, reason: "MULTI_TENANT_ENABLED differ"},
		{name: "only the ledger multi-tenant", ledger: grpcLedger + multiTenant, tracer: grpcTracer, reason: "MULTI_TENANT_ENABLED differ"},
		{name: "multi-tenancy unparsable on the tracer", ledger: grpcLedger + multiTenant, tracer: grpcTracer + "MULTI_TENANT_ENABLED=on\n" + tenantManager, reason: "MULTI_TENANT_ENABLED differ"},
		{name: "multi-tenant tracer without a tenant-manager url", ledger: grpcLedger + multiTenant, tracer: grpcTracer + multiTenant + "MULTI_TENANT_SERVICE_API_KEY=svc-api-key\n", reason: "requires tracer MULTI_TENANT_URL"},
		{name: "multi-tenant tracer without a service api key", ledger: grpcLedger + multiTenant, tracer: grpcTracer + multiTenant + "MULTI_TENANT_URL=https://tenant-manager.example.test\nMULTI_TENANT_SERVICE_API_KEY= \n", reason: "requires tracer MULTI_TENANT_SERVICE_API_KEY"},
		{name: "multi-tenant tracer in local mode", ledger: restLedger + multiTenant, tracer: "DEPLOYMENT_MODE=local\n" + tokenProducer + multiTenant + tenantManager, reason: "refused under tracer DEPLOYMENT_MODE=local"},
		{name: "saas tracer without a tls mode", ledger: grpcLedger, tracer: "DEPLOYMENT_MODE=saas\n" + grpcPort + tokenCheck + bothProducers, reason: "tracer DEPLOYMENT_MODE=saas requires tracer TRACER_TLS_MODE"},
		{name: "saas ledger with an http tracer url", ledger: restLedger + "DEPLOYMENT_MODE=saas\n", tracer: restTracer, reason: "refuses an http:// TRACER_BASE_URL"},
		{name: "saas ledger with an http tracer url and an empty tls mode", ledger: restLedger + "DEPLOYMENT_MODE= SAAS \nTRACER_TLS_MODE=\n", tracer: restTracer, reason: "refuses an http:// TRACER_BASE_URL"},
		{name: "saas ledger with an http plugin auth host", ledger: "TRACER_BASE_URL=https://tracer:4020\nTRACER_TRANSPORT=rest\nDEPLOYMENT_MODE=saas\n" + clientCredential + pluginAuth, tracer: restTracer, reason: "refuses an http:// PLUGIN_AUTH_HOST"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := checkProfiles(writeProfiles(t, test.ledger, test.tracer))
			require.ErrorContains(t, err, test.reason)
		})
	}
}

// TestProfileCheckNeverEchoesSettingValues keeps every identity failure free
// of the values it compared, so the output can be pasted into a ticket.
func TestProfileCheckNeverEchoesSettingValues(t *testing.T) {
	t.Parallel()

	const (
		privateClientID = "private-client-id"
		privateCertURI  = "spiffe://private.example.test/ledger"
		privateURL      = "https://private-tracer.example.test"
		privateSecret   = "private-client-secret"
	)

	tests := []struct {
		name   string
		ledger string
		tracer string
	}{
		{name: "unmapped client id", ledger: "TRACER_BASE_URL=" + privateURL + "\nTRACER_TRANSPORT=rest\nIDP_M2M_CLIENT_ID=" + privateClientID + "\nIDP_M2M_CLIENT_SECRET=" + privateSecret + "\n" + pluginAuth, tracer: certProducer},
		{name: "tracer verification missing", ledger: "TRACER_BASE_URL=" + privateURL + "\nTRACER_TRANSPORT=rest\n" + clientCredential + "IDP_M2M_CLIENT_SECRET=" + privateSecret + "\n" + pluginAuth, tracer: tokenProducer},
		{name: "plugin auth host missing", ledger: "TRACER_BASE_URL=" + privateURL + "\nTRACER_TRANSPORT=rest\nIDP_M2M_CLIENT_ID=" + privateClientID + "\nPLUGIN_AUTH_ENABLED=true\n", tracer: restTracer},
		{name: "invalid producers", ledger: grpcLedger, tracer: "TRACER_TLS_MODE=mtls\nTRACER_PLATFORM_PRODUCERS='[{\"service\":\"ledger\",\"certUri\":\"" + privateCertURI + "?q=1\"}]'\n"},
		{name: "unknown transport", ledger: "TRACER_BASE_URL=" + privateURL + "\nTRACER_TRANSPORT=" + privateClientID + "\n", tracer: grpcTracer},
		{name: "application outside the roster", ledger: grpcLedger + "APPLICATION_NAME=" + privateClientID + "\n", tracer: grpcTracer},
		{name: "tenant-manager key missing", ledger: grpcLedger + multiTenant, tracer: grpcTracer + multiTenant + "MULTI_TENANT_URL=" + privateURL + "\n"},
		{name: "tenant-manager url missing", ledger: grpcLedger + multiTenant, tracer: grpcTracer + multiTenant + "MULTI_TENANT_SERVICE_API_KEY=" + privateSecret + "\n"},
		{name: "saas plaintext plugin auth", ledger: "TRACER_BASE_URL=" + privateURL + "\nTRACER_TRANSPORT=rest\nDEPLOYMENT_MODE=saas\n" + clientCredential + "PLUGIN_AUTH_ENABLED=true\nPLUGIN_AUTH_HOST=http://" + privateClientID + ":4000\n", tracer: restTracer},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := checkProfiles(writeProfiles(t, test.ledger, test.tracer))
			require.Error(t, err)

			for _, secret := range []string{privateClientID, privateCertURI, privateURL, privateSecret} {
				require.NotContains(t, err.Error(), secret)
			}
		})
	}
}

func TestProfileCheckRequiresBothFiles(t *testing.T) {
	t.Parallel()

	ledgerPath, tracerPath := writeProfiles(t, grpcLedger, grpcTracer)

	require.Error(t, checkProfiles("", tracerPath))
	require.Error(t, checkProfiles(ledgerPath, ""))
	require.Error(t, checkProfiles(filepath.Join(t.TempDir(), "missing.env"), tracerPath))
}

func TestProfileCheckRejectsUnreadableInput(t *testing.T) {
	t.Parallel()

	oversized := grpcTracer + strings.Repeat("#", 1<<20)
	require.Error(t, checkProfiles(writeProfiles(t, grpcLedger, oversized)))
	require.Error(t, checkProfiles(writeProfiles(t, grpcLedger+"TRACER_CONTEXT_MAX_ACCOUNTS=many\n", grpcTracer)))
}

func TestRunReportsTheOutcomeThroughTheExitCode(t *testing.T) {
	t.Parallel()

	ledgerPath, tracerPath := writeProfiles(t, grpcLedger, grpcTracer)

	var stdout, stderr bytes.Buffer

	require.Equal(t, 0, run([]string{"--ledger-env", ledgerPath, "--tracer-env", tracerPath}, &stdout, &stderr))
	require.Contains(t, stdout.String(), "profiles match")
	require.Empty(t, stderr.String())

	stdout.Reset()
	stderr.Reset()

	require.Equal(t, 1, run([]string{"--ledger-env", ledgerPath}, &stdout, &stderr))
	require.Contains(t, stderr.String(), "--tracer-env")
	require.Empty(t, stdout.String())

	require.Equal(t, 2, run([]string{"--unknown"}, &stdout, &stderr))
}
