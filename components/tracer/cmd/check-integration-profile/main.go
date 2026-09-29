// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// check-integration-profile compares rendered Ledger and Tracer deployment
// settings: resource bounds, deployment posture, tenancy, producer identity per
// transport and the reaper.
// It never connects to services, loads credentials or changes environment state.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// Transports and TLS modes the ledger selects with TRACER_TRANSPORT and
// TRACER_TLS_MODE. An empty transport is gRPC, as it is in the ledger.
const (
	transportGRPC = "grpc"
	transportREST = "rest"
	tlsModeMTLS   = "mtls"
	tlsModeMesh   = "mesh"

	deploymentModeSaaS  = "saas"
	deploymentModeLocal = "local"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses args, checks both files and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check-integration-profile", flag.ContinueOnError)
	flags.SetOutput(stderr)

	ledger := flags.String("ledger-env", "", "rendered Ledger environment file")
	tracer := flags.String("tracer-env", "", "rendered Tracer environment file")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if err := checkProfiles(*ledger, *tracer); err != nil {
		fmt.Fprintln(stderr, err)

		return 1
	}

	fmt.Fprintln(stdout, "Ledger and Tracer integration profiles match; this does not certify credentials, remote readiness or SLOs.")

	return 0
}

func checkProfiles(ledgerPath, tracerPath string) error {
	if ledgerPath == "" || tracerPath == "" {
		return fmt.Errorf("both --ledger-env and --tracer-env are required")
	}

	ledgerEnv, err := readEnvironment(ledgerPath)
	if err != nil {
		return err
	}

	tracerEnv, err := readEnvironment(tracerPath)
	if err != nil {
		return err
	}

	ledger, err := resourceProfile(ledgerEnv, true)
	if err != nil {
		return err
	}

	tracer, err := resourceProfile(tracerEnv, false)
	if err != nil {
		return err
	}

	if ledger != tracer {
		return fmt.Errorf("ledger and tracer resource profiles differ; align accounts, entries, text, integer/fraction digits, body bytes and reservation counts before activation")
	}

	if err := checkTracerListenerTLS(tracerEnv); err != nil {
		return err
	}

	if err := checkTenancy(ledgerEnv, tracerEnv); err != nil {
		return err
	}

	if err := checkProducerIdentity(ledgerEnv, tracerEnv); err != nil {
		return err
	}

	return requireReaperEnabled(tracerEnv)
}

// checkTracerListenerTLS mirrors the Tracer's refusal to boot under
// DEPLOYMENT_MODE=saas without a TRACER_TLS_MODE for its listeners.
func checkTracerListenerTLS(tracerEnv map[string]string) error {
	if deploymentMode(tracerEnv) == deploymentModeSaaS && tlsMode(tracerEnv) == "" {
		return fmt.Errorf("tracer DEPLOYMENT_MODE=saas requires tracer TRACER_TLS_MODE (mtls, or mesh when a sidecar terminates TLS): otherwise the tracer refuses to boot")
	}

	return nil
}

// checkTenancy requires both services to agree on multi-tenancy, parsed as
// each boot parses it, and a multi-tenant Tracer to reach the tenant-manager
// and authorize producers through plugin-auth. Unlike the single-tenant
// surface, DEPLOYMENT_MODE=local does not waive plugin-auth under
// multi-tenancy. Setting values are never reported.
func checkTenancy(ledgerEnv, tracerEnv map[string]string) error {
	tracerMT := bootBool(tracerEnv, "MULTI_TENANT_ENABLED")
	if bootBool(ledgerEnv, "MULTI_TENANT_ENABLED") != tracerMT {
		return fmt.Errorf("ledger and tracer MULTI_TENANT_ENABLED differ: both services must run single-tenant or both multi-tenant, or the tracer reads reservations under the wrong tenant")
	}

	if !tracerMT {
		return nil
	}

	if !bootBool(tracerEnv, "PLUGIN_AUTH_ENABLED") {
		return fmt.Errorf("tracer MULTI_TENANT_ENABLED=true requires tracer PLUGIN_AUTH_ENABLED=true in every DEPLOYMENT_MODE, local included: without plugin-auth the tracer verifies no producer, so any caller could name any tenant, and the tracer refuses to boot")
	}

	if strings.TrimSpace(tracerEnv["MULTI_TENANT_URL"]) == "" {
		return fmt.Errorf("tracer MULTI_TENANT_ENABLED=true requires tracer MULTI_TENANT_URL: the tracer resolves tenants through the tenant-manager")
	}

	if strings.TrimSpace(tracerEnv["MULTI_TENANT_SERVICE_API_KEY"]) == "" {
		return fmt.Errorf("tracer MULTI_TENANT_ENABLED=true requires tracer MULTI_TENANT_SERVICE_API_KEY: the tenant-manager answers no tenant lookup without it")
	}

	return nil
}

// checkProducerIdentity verifies that the Tracer will recognize the Ledger as
// a platform producer over the transport the Ledger selects: by its client
// certificate over native mTLS on gRPC; on REST, single-tenant, by its M2M
// client id, and multi-tenant by the platform claims the tenant-manager
// writes onto the per-tenant M2M application it provisions, so no client id
// is mapped. It then requires the plugin-auth posture the Tracer's
// reservation surface boots under. Messages name settings only, never their
// values.
func checkProducerIdentity(ledgerEnv, tracerEnv map[string]string) error {
	if strings.TrimSpace(ledgerEnv["TRACER_BASE_URL"]) == "" {
		return fmt.Errorf("ledger TRACER_BASE_URL must be set: without it the ledger builds no tracer integration")
	}

	service := strings.TrimSpace(ledgerEnv["APPLICATION_NAME"])
	if service == "" {
		service = producerauth.ServiceLedger
	}

	if !producerauth.InRoster(service) {
		return fmt.Errorf("ledger APPLICATION_NAME is not a platform producer; the ledger acts as %q", producerauth.ServiceLedger)
	}

	multiTenant := bootBool(tracerEnv, "MULTI_TENANT_ENABLED")

	registry, err := producerRegistry(tracerEnv, multiTenant)
	if err != nil {
		return err
	}

	clientIDs, certURIs := registry.Credentials(service)
	if !multiTenant && len(clientIDs) == 0 && len(certURIs) == 0 {
		return fmt.Errorf("TRACER_PLATFORM_PRODUCERS maps no credential onto the ledger's APPLICATION_NAME")
	}

	transport := strings.ToLower(strings.TrimSpace(ledgerEnv["TRACER_TRANSPORT"]))
	if transport == "" {
		transport = transportGRPC
	}

	switch {
	case transport == transportREST && multiTenant:
		err = checkTenantTokenIdentity(ledgerEnv, tracerEnv)
	case transport == transportREST:
		err = checkTokenIdentity(ledgerEnv, tracerEnv, registry, service)
	case transport == transportGRPC:
		err = checkCertificateIdentity(ledgerEnv, tracerEnv, certURIs)
	default:
		err = fmt.Errorf("ledger TRACER_TRANSPORT must be %q or %q", transportGRPC, transportREST)
	}

	if err != nil {
		return err
	}

	return checkTokenAuthorization(tracerEnv)
}

// producerRegistry parses TRACER_PLATFORM_PRODUCERS as the Tracer boot does.
// Single-tenant, it is the switch of the reservation surface and must be set.
// Multi-tenant, the surface is always mounted and the tenant-manager decides
// per tenant: the roster is optional, serves gRPC certificates only, and a
// clientId entry refuses boot. A nil registry means no roster.
func producerRegistry(tracerEnv map[string]string, multiTenant bool) (*producerauth.Registry, error) {
	raw := tracerEnv["TRACER_PLATFORM_PRODUCERS"]
	if strings.TrimSpace(raw) == "" {
		if multiTenant {
			return nil, nil
		}

		return nil, fmt.Errorf("tracer TRACER_PLATFORM_PRODUCERS must be set: without it the tracer serves validations only and mounts no reservation surface")
	}

	registry, err := producerauth.ParsePlatformProducers(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid TRACER_PLATFORM_PRODUCERS (contents suppressed)")
	}

	if multiTenant && registry.HasClientIDMappings() {
		return nil, fmt.Errorf("tracer MULTI_TENANT_ENABLED=true refuses clientId entries in TRACER_PLATFORM_PRODUCERS: multi-tenant REST reservations identify the ledger by the tenant-manager's token claims; keep only certUri entries, which serve gRPC")
	}

	return registry, nil
}

// checkTenantTokenIdentity requires the Ledger to reach the Tracer's REST
// listener over the scheme it serves and to mint its per-tenant tokens from
// plugin-auth. The per-tenant credentials are provisioned by the
// tenant-manager, not configured, so no client id is checked.
func checkTenantTokenIdentity(ledgerEnv, tracerEnv map[string]string) error {
	if err := checkTokenTransport(ledgerEnv, tracerEnv); err != nil {
		return err
	}

	return checkTokenMinter(ledgerEnv)
}

// checkTokenIdentity requires the Ledger to reach the Tracer's REST listener
// over the scheme it serves and to obtain M2M tokens from plugin-auth under a
// client id the Tracer maps onto the Ledger's service.
func checkTokenIdentity(ledgerEnv, tracerEnv map[string]string, registry *producerauth.Registry, service string) error {
	if err := checkTokenTransport(ledgerEnv, tracerEnv); err != nil {
		return err
	}

	return checkTokenIssuance(ledgerEnv, registry, service)
}

// checkTokenTransport requires both ends of the REST call to agree on TLS: a
// Ledger that speaks TLS needs a Tracer serving it, and a Tracer serving HTTPS
// only needs a Ledger that originates TLS itself, through an https:// base
// URL, or through a TLS-originating mesh.
func checkTokenTransport(ledgerEnv, tracerEnv map[string]string) error {
	if isMTLS(ledgerEnv) && !isMTLS(tracerEnv) {
		return fmt.Errorf("ledger TRACER_TRANSPORT=rest with ledger TRACER_TLS_MODE=mtls requires tracer TRACER_TLS_MODE=mtls: otherwise the tracer listens plaintext and the TLS handshake fails")
	}

	baseURL := strings.ToLower(strings.TrimSpace(ledgerEnv["TRACER_BASE_URL"]))
	if isMTLS(tracerEnv) && !isMTLS(ledgerEnv) && tlsMode(ledgerEnv) != tlsModeMesh && !strings.HasPrefix(baseURL, "https://") {
		return fmt.Errorf("tracer TRACER_TLS_MODE=mtls serves HTTPS only: ledger TRACER_TRANSPORT=rest requires ledger TRACER_TLS_MODE=mtls, an https:// TRACER_BASE_URL, or ledger TRACER_TLS_MODE=mesh behind a TLS-originating mesh")
	}

	return checkSaaSTokenTLS(ledgerEnv)
}

// checkSaaSTokenTLS mirrors the Ledger's refusal to boot under
// DEPLOYMENT_MODE=saas when the REST seam or its plugin-auth hop is an
// http:// address outside an explicit TRACER_TLS_MODE=mesh: both carry the M2M
// credential. PLUGIN_AUTH_HOST is checked as configured; an address service
// discovery resolves is not known here.
func checkSaaSTokenTLS(ledgerEnv map[string]string) error {
	if deploymentMode(ledgerEnv) != deploymentModeSaaS || tlsMode(ledgerEnv) == tlsModeMesh {
		return nil
	}

	if isCleartextURL(ledgerEnv["TRACER_BASE_URL"]) {
		return fmt.Errorf("ledger DEPLOYMENT_MODE=saas refuses an http:// TRACER_BASE_URL for TRACER_TRANSPORT=rest: use an https:// URL, or ledger TRACER_TLS_MODE=mesh behind a TLS-originating mesh")
	}

	if isCleartextURL(ledgerEnv["PLUGIN_AUTH_HOST"]) {
		return fmt.Errorf("ledger DEPLOYMENT_MODE=saas refuses an http:// PLUGIN_AUTH_HOST for TRACER_TRANSPORT=rest: use an https:// URL, or ledger TRACER_TLS_MODE=mesh behind a TLS-originating mesh")
	}

	return nil
}

// checkTokenIssuance requires the Ledger to mint its token from plugin-auth,
// reached statically or through service discovery, with a complete client
// credential whose id the Tracer maps onto the Ledger's service.
func checkTokenIssuance(ledgerEnv map[string]string, registry *producerauth.Registry, service string) error {
	if err := checkTokenMinter(ledgerEnv); err != nil {
		return err
	}

	clientID := ledgerEnv["IDP_M2M_CLIENT_ID"]
	if clientID == "" {
		return fmt.Errorf("ledger TRACER_TRANSPORT=rest requires IDP_M2M_CLIENT_ID: the tracer identifies the ledger by its M2M token")
	}

	if strings.TrimSpace(ledgerEnv["IDP_M2M_CLIENT_SECRET"]) == "" {
		return fmt.Errorf("ledger TRACER_TRANSPORT=rest requires IDP_M2M_CLIENT_SECRET: plugin-auth mints no token without it")
	}

	producer, ok := registry.ByClientID(clientID)
	if !ok || producer.Service != service {
		return fmt.Errorf("ledger IDP_M2M_CLIENT_ID is not a clientId that TRACER_PLATFORM_PRODUCERS maps onto the ledger's APPLICATION_NAME")
	}

	return nil
}

// checkTokenMinter requires the Ledger to reach plugin-auth, statically or
// through service discovery, to mint its tracer tokens.
func checkTokenMinter(ledgerEnv map[string]string) error {
	if !bootBool(ledgerEnv, "PLUGIN_AUTH_ENABLED") {
		return fmt.Errorf("ledger TRACER_TRANSPORT=rest requires ledger PLUGIN_AUTH_ENABLED=true: the ledger obtains its tracer token from plugin-auth")
	}

	if strings.TrimSpace(ledgerEnv["PLUGIN_AUTH_HOST"]) == "" && !discoveryEnabled(ledgerEnv) {
		return fmt.Errorf("ledger TRACER_TRANSPORT=rest requires ledger PLUGIN_AUTH_HOST unless ledger SD_ENABLED=true resolves plugin-auth: the ledger obtains its tracer token from plugin-auth")
	}

	return nil
}

// checkTokenAuthorization mirrors the Tracer's refusal to boot a reservation
// surface without plugin-auth outside DEPLOYMENT_MODE=local, and its refusal
// to boot plugin-auth without an Access Manager address: its reservation
// routes authorize the caller through the Access Manager. A local Tracer
// without plugin-auth attributes every HTTP reservation to the ledger.
func checkTokenAuthorization(tracerEnv map[string]string) error {
	if bootBool(tracerEnv, "PLUGIN_AUTH_ENABLED") {
		if strings.TrimSpace(tracerEnv["PLUGIN_AUTH_ADDRESS"]) == "" {
			return fmt.Errorf("tracer PLUGIN_AUTH_ENABLED=true requires tracer PLUGIN_AUTH_ADDRESS: the tracer authorizes reservation callers through the Access Manager and otherwise refuses to boot")
		}

		return nil
	}

	if deploymentMode(tracerEnv) == deploymentModeLocal {
		return nil
	}

	return fmt.Errorf("tracer reservations require tracer PLUGIN_AUTH_ENABLED=true unless tracer DEPLOYMENT_MODE=local: the tracer authorizes reservation callers through the Access Manager and otherwise refuses to boot")
}

// checkCertificateIdentity requires native mTLS on both sides, a Tracer gRPC
// listener and a certificate URI mapped onto the Ledger's service.
func checkCertificateIdentity(ledgerEnv, tracerEnv map[string]string, certURIs []string) error {
	if !isMTLS(ledgerEnv) {
		return fmt.Errorf("ledger TRACER_TRANSPORT=grpc requires ledger TRACER_TLS_MODE=mtls: the tracer identifies the ledger by its client certificate")
	}

	if !isMTLS(tracerEnv) {
		return fmt.Errorf("ledger TRACER_TRANSPORT=grpc requires tracer TRACER_TLS_MODE=mtls: the tracer identifies the ledger by its client certificate")
	}

	if strings.TrimSpace(tracerEnv["TRACER_GRPC_PORT"]) == "" {
		return fmt.Errorf("ledger TRACER_TRANSPORT=grpc requires tracer TRACER_GRPC_PORT: without it the tracer starts no gRPC listener")
	}

	if len(certURIs) == 0 {
		return fmt.Errorf("ledger TRACER_TRANSPORT=grpc requires a certUri that TRACER_PLATFORM_PRODUCERS maps onto the ledger's APPLICATION_NAME")
	}

	return nil
}

func isMTLS(values map[string]string) bool {
	return tlsMode(values) == tlsModeMTLS
}

func tlsMode(values map[string]string) string {
	return strings.ToLower(strings.TrimSpace(values["TRACER_TLS_MODE"]))
}

func deploymentMode(values map[string]string) string {
	return strings.ToLower(strings.TrimSpace(values["DEPLOYMENT_MODE"]))
}

// isCleartextURL reports an http:// address with a host, the form the Ledger
// boot treats as a cleartext dial.
func isCleartextURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))

	return err == nil && strings.EqualFold(parsed.Scheme, "http") && parsed.Host != ""
}

// bootBool parses key as both services parse a boolean setting at boot:
// strconv.ParseBool on the raw value, with unset or unparsable read as false.
// The value is deliberately not trimmed: the services do not trim it either,
// so a quoted, padded "true" boots as false and must be reported as false.
func bootBool(values map[string]string, key string) bool {
	enabled, err := strconv.ParseBool(values[key])

	return err == nil && enabled
}

// discoveryEnabled parses service discovery's switch as the ledger reads it
// at boot: only the literal "true" enables it, under SD_ENABLED or its legacy
// name SERVICE_DISCOVERY_ENABLED.
func discoveryEnabled(values map[string]string) bool {
	return values["SD_ENABLED"] == "true" || values["SERVICE_DISCOVERY_ENABLED"] == "true"
}

// requireReaperEnabled rejects a Tracer that admits decisions without the
// reaper: an operation whose completion never arrives only returns its
// capacity when the reaper expires it. Unset keeps the Tracer default (on).
func requireReaperEnabled(tracerEnv map[string]string) error {
	raw, present := tracerEnv["RESERVATION_REAPER_ENABLED"]
	if !present {
		return nil
	}

	enabled, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil || !enabled {
		return fmt.Errorf("RESERVATION_REAPER_ENABLED must stay true while the tracer admits reservations; without it, capacity held by an operation that never completes is never returned")
	}

	return nil
}

func readEnvironment(path string) (map[string]string, error) {
	// Local operator CLI: the explicit input path is not supplied by a network
	// caller. Reading arbitrary deployment files is intentional; contents stay private.
	file, err := os.Open(path) // #nosec G304 -- operator-selected, read-only input; bounded below.
	if err != nil {
		return nil, fmt.Errorf("open deployment environment: %w", err)
	}
	defer func() { _ = file.Close() }()

	const maxBytes = 1 << 20

	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || len(raw) > maxBytes {
		return nil, fmt.Errorf("deployment environment cannot be read within the 1 MiB limit")
	}

	values, err := godotenv.Unmarshal(string(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid deployment environment syntax (contents suppressed)")
	}

	return values, nil
}

func resourceProfile(values map[string]string, ledger bool) (tracercontract.ResourceProfile, error) {
	profile := tracercontract.DefaultResourceProfile()

	prefix, reserve := "CONTEXT_", "CONTEXT_RESERVE_"
	if ledger {
		prefix, reserve = "TRACER_CONTEXT_", "TRACER_CONTEXT_"
	}

	fields := []struct {
		key    string
		target *int
	}{
		{prefix + "MAX_ACCOUNTS", &profile.Facts.MaxAccounts},
		{prefix + "MAX_ENTRIES", &profile.Facts.MaxEntries},
		{prefix + "MAX_TEXT_BYTES", &profile.Facts.MaxTextBytes},
		{prefix + "MAX_INTEGER_DIGITS", &profile.Facts.MaxIntegerDigits},
		{prefix + "MAX_FRACTION_DIGITS", &profile.Facts.MaxFractionDigits},
		{reserve + "MAX_BODY_BYTES", &profile.MaxBodyBytes},
		{reserve + "MAX_RESERVATIONS", &profile.MaxReservations},
	}
	for _, field := range fields {
		if raw, present := values[field.key]; present {
			value, err := strconv.Atoi(raw)
			if err != nil {
				return profile, fmt.Errorf("invalid numeric resource setting %s", field.key)
			}

			*field.target = value
		}
	}

	return profile, profile.Validate()
}
