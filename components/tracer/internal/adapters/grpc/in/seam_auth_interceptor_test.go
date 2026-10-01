// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/bxcodec/dbresolver/v2"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

const (
	seamTestClient      = "lerian/midaz-ledger"
	seamTestOtherClient = "lerian/some-other-app"
	seamTestTenant      = "tenant-claim-001"
	seamTestAPIKey      = "a-32-character-api-key-for-tests" // gitleaks:allow

	// seamTenantA / seamTenantB are tenant ids in the dashless form the
	// tenant-manager writes into the M2M tenantId claim and application name.
	seamTenantA       = "0193b0c4d2a87e4f9c1d2e3f4a5b6c7d"
	seamTenantADashed = "0193b0c4-d2a8-7e4f-9c1d-2e3f4a5b6c7d"
	seamTenantB       = "0193b0c4d2a87e4f9c1d2e3f4a5b6c7e"
)

// seamTestToken signs claims with a throwaway key. The guard parses tokens
// unverified, so the key only has to produce a well-formed JWT.
func seamTestToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("seam-test-signing-key"))
	require.NoError(t, err)

	return token
}

func applicationClaims(sub, tenantID string) jwt.MapClaims {
	claims := jwt.MapClaims{"type": "application", "sub": sub}
	if tenantID != "" {
		claims["tenantId"] = tenantID
	}

	return claims
}

// ledgerClaims mirrors the claims of the per-tenant ledger M2M application
// the tenant-manager provisions: name "ledger-m2m-tracer-{nameTenant}", sub
// "admin/{name}", tenantId claimTenant.
func ledgerClaims(nameTenant, claimTenant string) jwt.MapClaims {
	name := "ledger-m2m-tracer-" + nameTenant
	claims := applicationClaims("admin/"+name, claimTenant)
	claims["name"] = name
	claims["owner"] = "admin"

	return claims
}

// seamIncoming builds an incoming context carrying the given metadata pairs
// plus a recording logger.
func seamIncoming(logger *testutil.MockLogger, pairs ...string) context.Context {
	return metadata.NewIncomingContext(
		libObservability.ContextWithLogger(context.Background(), logger),
		metadata.Pairs(pairs...),
	)
}

func TestSeamAuthPolicyConfig_MapsEveryRPCToItsAction(t *testing.T) {
	t.Parallel()

	cfg := SeamAuthPolicyConfig()

	want := map[string]string{
		reservationv1.ReservationService_Reserve_FullMethodName:              "post",
		reservationv1.ReservationService_ConfirmByTransaction_FullMethodName: "post",
		reservationv1.ReservationService_ConfirmById_FullMethodName:          "post",
		reservationv1.ReservationService_ReleaseByTransaction_FullMethodName: "post",
		reservationv1.ReservationService_ReleaseById_FullMethodName:          "post",
	}

	require.Len(t, cfg.MethodPolicies, len(want))
	assert.Nil(t, cfg.DefaultPolicy, "an unmapped method must fail closed, never fall back to a default grant")

	for method, action := range want {
		policy, ok := cfg.MethodPolicies[method]
		require.True(t, ok, method)
		assert.Equal(t, "reservations", policy.Resource, method)
		assert.Equal(t, action, policy.Action, method)
	}

	require.NotNil(t, cfg.SubResolver)

	product, err := cfg.SubResolver(context.Background(), reservationv1.ReservationService_Reserve_FullMethodName, nil)
	require.NoError(t, err)
	assert.Equal(t, "tracer", product)
}

func TestSeamAuthPolicyConfig_CoversEveryServiceMethod(t *testing.T) {
	t.Parallel()

	cfg := SeamAuthPolicyConfig()

	for _, method := range reservationv1.ReservationService_ServiceDesc.Methods {
		full := "/" + reservationv1.ReservationService_ServiceDesc.ServiceName + "/" + method.MethodName
		_, ok := cfg.MethodPolicies[full]
		assert.True(t, ok, "RPC %s has no seam policy", full)
	}

	assert.Empty(t, reservationv1.ReservationService_ServiceDesc.Streams,
		"the seam installs only a unary auth interceptor; a streaming RPC would need the stream policy too")
}

func TestSeamPrincipalInterceptor(t *testing.T) {
	t.Parallel()

	userToken := func(t *testing.T) string {
		return seamTestToken(t, jwt.MapClaims{"type": "normal-user", "sub": "alice", "owner": "lerian"})
	}

	tests := []struct {
		name        string
		multiTenant bool
		token       func(t *testing.T) string
		pairs       []string
		wantAllowed bool
		wantSub     string
	}{
		{
			name:    "user token is refused",
			token:   userToken,
			wantSub: "alice",
		},
		{
			name:    "single-tenant application outside the allowlist is refused",
			token:   func(t *testing.T) string { return seamTestToken(t, applicationClaims(seamTestOtherClient, "")) },
			wantSub: seamTestOtherClient,
		},
		{
			name:        "single-tenant allowlisted application is admitted",
			token:       func(t *testing.T) string { return seamTestToken(t, applicationClaims(seamTestClient, "")) },
			wantAllowed: true,
		},
		{
			name:        "multi-tenant application without a tenantId claim is refused",
			multiTenant: true,
			token:       func(t *testing.T) string { return seamTestToken(t, applicationClaims(seamTestClient, "")) },
			wantSub:     seamTestClient,
		},
		{
			name:        "multi-tenant ledger application for its own tenant is admitted",
			multiTenant: true,
			token:       func(t *testing.T) string { return seamTestToken(t, ledgerClaims(seamTenantA, seamTenantA)) },
			wantAllowed: true,
		},
		{
			name:        "x-tenant-id that differs from the claim is refused",
			multiTenant: true,
			token:       func(t *testing.T) string { return seamTestToken(t, ledgerClaims(seamTenantA, seamTenantA)) },
			pairs:       []string{seamtenant.MetadataKey, seamTenantB},
			wantSub:     "admin/ledger-m2m-tracer-" + seamTenantA,
		},
		{
			name:        "x-tenant-id equal to the claim is admitted",
			multiTenant: true,
			token:       func(t *testing.T) string { return seamTestToken(t, ledgerClaims(seamTenantA, seamTenantA)) },
			pairs:       []string{seamtenant.MetadataKey, seamTenantA},
			wantAllowed: true,
		},
		{
			name:    "single-tenant x-tenant-id that differs from a present claim is refused",
			token:   func(t *testing.T) string { return seamTestToken(t, applicationClaims(seamTestClient, seamTestTenant)) },
			pairs:   []string{seamtenant.MetadataKey, "another-tenant"},
			wantSub: seamTestClient,
		},
		{
			name:        "md-tenant-id that differs from the claim is refused",
			multiTenant: true,
			token:       func(t *testing.T) string { return seamTestToken(t, ledgerClaims(seamTenantA, seamTenantA)) },
			pairs:       []string{TokenTenantMetadataKey, seamTenantB},
			wantSub:     "admin/ledger-m2m-tracer-" + seamTenantA,
		},
		{
			name:  "unreadable token is refused",
			token: func(*testing.T) string { return "not-a-jwt" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			interceptor := SeamPrincipalInterceptor(SeamPrincipalConfig{
				MultiTenant:    tt.multiTenant,
				AllowedClients: []string{" " + seamTestClient + " "},
			})

			logger := testutil.NewMockLogger()
			pairs := append([]string{"authorization", "Bearer " + tt.token(t)}, tt.pairs...)

			handlerCalled := false

			_, err := interceptor(seamIncoming(logger, pairs...), nil, unaryInfo(), func(context.Context, any) (any, error) {
				handlerCalled = true
				return "ok", nil
			})

			if tt.wantAllowed {
				require.NoError(t, err)
				assert.True(t, handlerCalled)
				assert.Empty(t, logger.Calls)

				return
			}

			require.False(t, handlerCalled)

			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, grpccodes.PermissionDenied, st.Code())
			assert.Equal(t, seamPrincipalRefusedMessage, st.Message(), "the refusal names no claim value")

			require.Len(t, logger.Calls, 1)
			assert.Equal(t, "warn", logger.Calls[0].Level)

			fields := map[string]any{}
			for _, f := range logger.Calls[0].Fields {
				fields[f.Key] = f.Value
			}

			assert.Equal(t, tt.wantSub, fields["sub"])
			assert.NotContains(t, fields, "tenant_id", "no claim value beyond sub reaches the log")
			assert.NotContains(t, fields, "token")
		})
	}
}

// TestSeamPrincipalInterceptor_TenantComparisonIsCanonical proves the
// x-tenant-id cross-check compares the lib-commons canonical tenant ids: a
// dashed and a dashless spelling of one UUID are one tenant, non-UUID ids
// compare verbatim, and an id lib-commons cannot canonicalize is refused.
func TestSeamPrincipalInterceptor_TenantComparisonIsCanonical(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		multiTenant bool
		claim       string
		header      string
		wantAllowed bool
	}{
		{name: "dashed header matches a dashless claim", multiTenant: true, claim: seamTenantA, header: seamTenantADashed, wantAllowed: true},
		{name: "dashless header matches a dashed claim", multiTenant: true, claim: seamTenantADashed, header: seamTenantA, wantAllowed: true},
		{name: "upper-case header matches", multiTenant: true, claim: seamTenantA, header: "0193B0C4-D2A8-7E4F-9C1D-2E3F4A5B6C7D", wantAllowed: true},
		{name: "single-tenant dashed header matches a dashless claim", claim: seamTenantA, header: seamTenantADashed, wantAllowed: true},
		{name: "different tenant is refused", multiTenant: true, claim: seamTenantA, header: seamTenantB},
		{name: "non-UUID tenants compare verbatim", claim: "tenant-a", header: "tenant-a", wantAllowed: true},
		{name: "non-UUID tenants differing in case are refused", claim: "Tenant-A", header: "tenant-a"},
		{name: "padded header is refused", multiTenant: true, claim: seamTenantA, header: " " + seamTenantA + " "},
		{name: "braced UUID header is refused", multiTenant: true, claim: seamTenantA, header: "{" + seamTenantADashed + "}"},
		{name: "urn UUID header is refused", multiTenant: true, claim: seamTenantA, header: "urn:uuid:" + seamTenantADashed},
		{name: "single-tenant malformed claim with a header is refused", claim: "bad tenant!", header: "bad tenant!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			interceptor := SeamPrincipalInterceptor(SeamPrincipalConfig{MultiTenant: tt.multiTenant, AllowedClients: []string{seamTestClient}})

			claims := applicationClaims(seamTestClient, tt.claim)
			if tt.multiTenant {
				claims = ledgerClaims(seamTenantA, tt.claim)
			}

			ctx := seamIncoming(
				testutil.NewMockLogger(),
				"authorization", "Bearer "+seamTestToken(t, claims),
				seamtenant.MetadataKey, tt.header,
			)

			_, err := interceptor(ctx, nil, unaryInfo(), func(context.Context, any) (any, error) { return "ok", nil })

			if tt.wantAllowed {
				require.NoError(t, err)

				return
			}

			assert.Equal(t, grpccodes.PermissionDenied, status.Code(err))
		})
	}
}

// TestSeamPrincipalInterceptor_MultiTenantBindsTheLedgerClient proves that in
// multi-tenant mode the Access Manager grant alone admits nobody: the token
// must be the tenant-manager's ledger application for the tenant its own
// tenantId claim names, so another product's client holding
// tracer/reservations, or the ledger client of another tenant, is refused.
func TestSeamPrincipalInterceptor_MultiTenantBindsTheLedgerClient(t *testing.T) {
	t.Parallel()

	withName := func(claims jwt.MapClaims, name any) jwt.MapClaims {
		claims["name"] = name
		return claims
	}

	tests := []struct {
		name        string
		claims      jwt.MapClaims
		wantAllowed bool
		wantReason  string
	}{
		{name: "ledger client of tenant A with claim A is admitted", claims: ledgerClaims(seamTenantA, seamTenantA), wantAllowed: true},
		{name: "ledger client named dashed with a dashless claim is admitted", claims: ledgerClaims(seamTenantADashed, seamTenantA), wantAllowed: true},
		{name: "ledger client named dashless with a dashed claim is admitted", claims: ledgerClaims(seamTenantA, seamTenantADashed), wantAllowed: true},
		{name: "ledger client of tenant B with claim A is refused", claims: ledgerClaims(seamTenantB, seamTenantA), wantReason: "client_not_ledger"},
		{name: "another product's client with the grant is refused", claims: withName(ledgerClaims(seamTenantA, seamTenantA), "flowker-m2m-tracer-"+seamTenantA), wantReason: "client_not_ledger"},
		{name: "ledger client of another target is refused", claims: withName(ledgerClaims(seamTenantA, seamTenantA), "ledger-m2m-midaz-"+seamTenantA), wantReason: "client_not_ledger"},
		{name: "a different-case prefix is refused", claims: withName(ledgerClaims(seamTenantA, seamTenantA), "Ledger-m2m-tracer-"+seamTenantA), wantReason: "client_not_ledger"},
		{name: "a bare prefix is refused", claims: withName(ledgerClaims(seamTenantA, seamTenantA), "ledger-m2m-tracer-"), wantReason: "client_not_ledger"},
		{name: "a token without a name claim is refused", claims: applicationClaims(seamTestClient, seamTenantA), wantReason: "client_not_ledger"},
		{name: "a non-string name claim is refused", claims: withName(ledgerClaims(seamTenantA, seamTenantA), 42), wantReason: "client_not_ledger"},
		{name: "a malformed tenantId claim is refused", claims: ledgerClaims("bad tenant!", "bad tenant!"), wantReason: "tenant_claim_invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			interceptor := SeamPrincipalInterceptor(SeamPrincipalConfig{MultiTenant: true})
			logger := testutil.NewMockLogger()

			handlerCalled := false

			_, err := interceptor(
				seamIncoming(logger, "authorization", "Bearer "+seamTestToken(t, tt.claims)),
				nil, unaryInfo(),
				func(context.Context, any) (any, error) {
					handlerCalled = true
					return "ok", nil
				},
			)

			if tt.wantAllowed {
				require.NoError(t, err)
				assert.True(t, handlerCalled)

				return
			}

			require.False(t, handlerCalled)

			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, grpccodes.PermissionDenied, st.Code())
			assert.Equal(t, seamPrincipalRefusedMessage, st.Message(), "the refusal names no claim value")

			require.Len(t, logger.Calls, 1)

			fields := map[string]any{}
			for _, f := range logger.Calls[0].Fields {
				fields[f.Key] = f.Value
			}

			assert.Equal(t, tt.wantReason, fields["reason"])
			assert.NotContains(t, fields, "name", "the application name never reaches the log")
		})
	}
}

// TestSeamClientName_IsTheTenantManagerConvention locks the application name
// the tenant-manager gives the ledger's per-tenant M2M client of the tracer.
func TestSeamClientName_IsTheTenantManagerConvention(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "ledger", SeamClientSourceService)
	assert.Equal(t, "tracer", SeamClientTargetService)
	assert.Equal(t, "ledger-m2m-tracer-", seamClientNamePrefix)
}

func TestSeamPrincipalInterceptor_RefusalIsABusinessSpanEvent(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	ctx, span := provider.Tracer("test").Start(context.Background(), "grpc.server")

	interceptor := SeamPrincipalInterceptor(SeamPrincipalConfig{AllowedClients: []string{seamTestClient}})

	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(
		"authorization", "Bearer "+seamTestToken(t, applicationClaims(seamTestOtherClient, "")),
	))

	_, err := interceptor(ctx, nil, unaryInfo(), func(context.Context, any) (any, error) { return "ok", nil })
	require.Error(t, err)

	span.End()

	ended := recorder.Ended()
	require.Len(t, ended, 1)
	assert.Equal(t, codes.Unset, ended[0].Status().Code, "a refused caller is a business outcome, not a technical failure")
	assert.NotEmpty(t, ended[0].Events())
}

func TestSeamAPIKeyInterceptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pairs       []string
		wantAllowed bool
	}{
		{name: "missing key is unauthenticated"},
		{name: "wrong key is unauthenticated", pairs: []string{SeamAPIKeyMetadataKey, "not-the-key"}},
		{name: "right key reaches the handler", pairs: []string{SeamAPIKeyMetadataKey, seamTestAPIKey}, wantAllowed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			interceptor := SeamAPIKeyInterceptor(seamTestAPIKey, "ledger-seam")
			logger := testutil.NewMockLogger()

			var handlerCtx context.Context

			_, err := interceptor(seamIncoming(logger, tt.pairs...), nil, unaryInfo(), func(c context.Context, _ any) (any, error) {
				handlerCtx = c
				return "ok", nil
			})

			if tt.wantAllowed {
				require.NoError(t, err)
				require.NotNil(t, handlerCtx)

				principal, ok := contextutil.GetPrincipal(handlerCtx)
				require.True(t, ok, "an API-key caller is attributed in the audit trail")
				assert.Equal(t, string(model.ActorTypeAPIKey), principal.Type)
				assert.Equal(t, "ledger-seam", principal.ID)
				assert.Empty(t, logger.Calls)

				return
			}

			require.Nil(t, handlerCtx)

			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, grpccodes.Unauthenticated, st.Code())
			assert.Equal(t, seamAPIKeyRefusedMessage, st.Message())

			require.Len(t, logger.Calls, 1)
			assert.Equal(t, "warn", logger.Calls[0].Level)

			for _, f := range logger.Calls[0].Fields {
				assert.NotEqual(t, seamTestAPIKey, f.Value, "the key never reaches the log")
				assert.NotEqual(t, "not-the-key", f.Value, "the presented key never reaches the log")
			}
		})
	}
}

func TestSeamAPIKeyInterceptor_BlankLabelFallsBackToTheDefault(t *testing.T) {
	t.Parallel()

	interceptor := SeamAPIKeyInterceptor(seamTestAPIKey, "")

	var handlerCtx context.Context

	_, err := interceptor(
		seamIncoming(testutil.NewMockLogger(), SeamAPIKeyMetadataKey, seamTestAPIKey),
		nil, unaryInfo(),
		func(c context.Context, _ any) (any, error) {
			handlerCtx = c
			return "ok", nil
		},
	)
	require.NoError(t, err)

	principal, ok := contextutil.GetPrincipal(handlerCtx)
	require.True(t, ok)
	assert.Equal(t, "tracer-default", principal.ID)
}

// TestSeamAPIKeyMetadataKey_IsTheLowercasedHTTPHeader locks the wire key the
// ledger sends: gRPC metadata keys are lower case.
func TestSeamAPIKeyMetadataKey_IsTheLowercasedHTTPHeader(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "x-api-key", SeamAPIKeyMetadataKey)
	assert.Equal(t, "md-tenant-id", TokenTenantMetadataKey)
}

func TestTokenTenantUnaryInterceptor_ResolvesFromTheClaimMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		pairs []string
	}{
		{name: "claim only", pairs: []string{TokenTenantMetadataKey, seamTestTenant}},
		{name: "claim wins over x-tenant-id", pairs: []string{TokenTenantMetadataKey, seamTestTenant, seamtenant.MetadataKey, "header-tenant"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var resolved string

			resolver := seamtenant.NewResolverWithPool(
				func(_ context.Context, tenantID string) (dbresolver.DB, error) {
					resolved = tenantID
					return stubPoolDB(t), nil
				},
				true,
			)

			interceptor := TokenTenantUnaryInterceptor(resolver, nil)

			var handlerCtx context.Context

			_, err := interceptor(seamIncoming(testutil.NewMockLogger(), tt.pairs...), nil, unaryInfo(),
				func(c context.Context, _ any) (any, error) {
					handlerCtx = c
					return "ok", nil
				})
			require.NoError(t, err)
			assert.Equal(t, seamTestTenant, resolved)
			assert.Equal(t, seamTestTenant, tmcore.GetTenantIDContext(handlerCtx))
		})
	}
}

// TestTokenTenantUnaryInterceptor_ResolvesTheCanonicalClaim proves the
// tenant handed to the resolver is the lib-commons canonical form of the
// claim (dashless, lower case for a UUID), the key the HTTP tenant middleware
// resolves, so both listeners share one pool and one worker set per tenant.
func TestTokenTenantUnaryInterceptor_ResolvesTheCanonicalClaim(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		claim string
		want  string
	}{
		{name: "dashless claim", claim: seamTenantA, want: seamTenantA},
		{name: "dashed claim", claim: seamTenantADashed, want: seamTenantA},
		{name: "upper-case dashed claim", claim: "0193B0C4-D2A8-7E4F-9C1D-2E3F4A5B6C7D", want: seamTenantA},
		{name: "non-UUID claim is verbatim", claim: "Tenant-Slug", want: "Tenant-Slug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var resolved string

			resolver := seamtenant.NewResolverWithPool(
				func(_ context.Context, tenantID string) (dbresolver.DB, error) {
					resolved = tenantID
					return stubPoolDB(t), nil
				},
				true,
			)

			var handlerCtx context.Context

			_, err := TokenTenantUnaryInterceptor(resolver, nil)(
				seamIncoming(
					testutil.NewMockLogger(),
					TokenTenantMetadataKey, tt.claim,
					seamtenant.MetadataKey, seamTenantADashed,
				),
				nil, unaryInfo(),
				func(c context.Context, _ any) (any, error) {
					handlerCtx = c
					return "ok", nil
				},
			)
			require.NoError(t, err)
			assert.Equal(t, tt.want, resolved)
			assert.Equal(t, tt.want, tmcore.GetTenantIDContext(handlerCtx))
		})
	}
}

func TestTokenTenantUnaryInterceptor_MalformedClaimIsAMissingTenant(t *testing.T) {
	t.Parallel()

	resolver := seamtenant.NewResolverWithPool(
		func(context.Context, string) (dbresolver.DB, error) {
			t.Error("a malformed tenant must never reach the pool")
			return stubPoolDB(t), nil
		},
		true,
	)

	_, err := TokenTenantUnaryInterceptor(resolver, nil)(
		seamIncoming(testutil.NewMockLogger(), TokenTenantMetadataKey, "{"+seamTenantADashed+"}"),
		nil, unaryInfo(),
		func(context.Context, any) (any, error) { return "ok", nil },
	)
	assert.Equal(t, grpccodes.InvalidArgument, status.Code(err))
}

func TestTokenTenantUnaryInterceptor_IgnoresXTenantIDForResolution(t *testing.T) {
	t.Parallel()

	resolver := seamtenant.NewResolverWithPool(
		func(context.Context, string) (dbresolver.DB, error) { return stubPoolDB(t), nil },
		true,
	)

	interceptor := TokenTenantUnaryInterceptor(resolver, nil)

	_, err := interceptor(
		seamIncoming(testutil.NewMockLogger(), seamtenant.MetadataKey, "header-tenant"),
		nil, unaryInfo(),
		func(context.Context, any) (any, error) { return "ok", nil },
	)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, grpccodes.InvalidArgument, st.Code(),
		"under token identity a header without the claim is a missing tenant")
}

func TestTenantUnaryInterceptor_APIKeyIdentityKeepsReadingXTenantID(t *testing.T) {
	t.Parallel()

	var resolved string

	resolver := seamtenant.NewResolverWithPool(
		func(_ context.Context, tenantID string) (dbresolver.DB, error) {
			resolved = tenantID
			return stubPoolDB(t), nil
		},
		true,
	)

	apiKey := SeamAPIKeyInterceptor(seamTestAPIKey, "")
	tenant := TenantUnaryInterceptor(resolver, nil)

	ctx := seamIncoming(
		testutil.NewMockLogger(),
		SeamAPIKeyMetadataKey, seamTestAPIKey,
		seamtenant.MetadataKey, interceptorTenantID,
		TokenTenantMetadataKey, "forged-claim-tenant",
	)

	_, err := apiKey(ctx, nil, unaryInfo(), func(c context.Context, req any) (any, error) {
		return tenant(c, req, unaryInfo(), func(context.Context, any) (any, error) { return "ok", nil })
	})
	require.NoError(t, err)
	assert.Equal(t, interceptorTenantID, resolved)
}

// TestSeamPrincipalInterceptor_StampsTheApplicationPrincipal proves an
// admitted call reaches the handler attributed to the token's application, so
// the RESERVATION_* audit rows name the ledger's client instead of system. The
// actor type is "user", the type the HTTP listener stamps on every bearer
// token, application tokens included.
func TestSeamPrincipalInterceptor_StampsTheApplicationPrincipal(t *testing.T) {
	t.Parallel()

	singleTenant := applicationClaims(seamTestClient, "")
	singleTenant["name"] = "midaz-ledger"

	tests := []struct {
		name        string
		multiTenant bool
		claims      jwt.MapClaims
		want        contextutil.Principal
	}{
		{
			name:   "single-tenant allowlisted application",
			claims: singleTenant,
			want:   contextutil.Principal{Type: string(model.ActorTypeUser), ID: seamTestClient, Name: "midaz-ledger"},
		},
		{
			name:   "single-tenant application without a name claim",
			claims: applicationClaims(seamTestClient, ""),
			want:   contextutil.Principal{Type: string(model.ActorTypeUser), ID: seamTestClient},
		},
		{
			name:        "multi-tenant ledger client",
			multiTenant: true,
			claims:      ledgerClaims(seamTenantA, seamTenantA),
			want: contextutil.Principal{
				Type: string(model.ActorTypeUser),
				ID:   "admin/ledger-m2m-tracer-" + seamTenantA,
				Name: "ledger-m2m-tracer-" + seamTenantA,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			interceptor := SeamPrincipalInterceptor(SeamPrincipalConfig{MultiTenant: tt.multiTenant, AllowedClients: []string{seamTestClient}})

			var handlerCtx context.Context

			_, err := interceptor(
				seamIncoming(testutil.NewMockLogger(), "authorization", "Bearer "+seamTestToken(t, tt.claims)),
				nil, unaryInfo(),
				func(ctx context.Context, _ any) (any, error) {
					handlerCtx = ctx
					return "ok", nil
				},
			)
			require.NoError(t, err)

			principal, ok := contextutil.GetPrincipal(handlerCtx)
			require.True(t, ok, "an admitted call carries the application principal")
			assert.Equal(t, tt.want, principal)
			assert.True(t, model.ActorType(principal.Type).IsValid(), "the audit trail accepts the actor type")
		})
	}
}

// TestSeamPrincipalInterceptor_TrimsTheNameAndTenantClaims proves the name and
// tenantId claims are judged by their trimmed value, like sub and type: a
// padded claim of the ledger's own client is admitted, and nothing that is not
// exactly the ledger client of the claimed tenant once trimmed is.
func TestSeamPrincipalInterceptor_TrimsTheNameAndTenantClaims(t *testing.T) {
	t.Parallel()

	padded := func(claims jwt.MapClaims, key, value string) jwt.MapClaims {
		claims[key] = value
		return claims
	}

	tests := []struct {
		name        string
		multiTenant bool
		claims      jwt.MapClaims
		pairs       []string
		wantName    string
		wantReason  string
	}{
		{
			name:        "padded ledger client name is admitted",
			multiTenant: true,
			claims:      padded(ledgerClaims(seamTenantA, seamTenantA), "name", "  ledger-m2m-tracer-"+seamTenantA+"\t"),
			wantName:    "ledger-m2m-tracer-" + seamTenantA,
		},
		{
			name:        "padded tenantId claim is admitted",
			multiTenant: true,
			claims:      ledgerClaims(seamTenantA, " "+seamTenantA+" "),
			wantName:    "ledger-m2m-tracer-" + seamTenantA,
		},
		{
			name:        "padded tenantId claim matches an equal x-tenant-id",
			multiTenant: true,
			claims:      ledgerClaims(seamTenantA, " "+seamTenantA+" "),
			pairs:       []string{seamtenant.MetadataKey, seamTenantA},
			wantName:    "ledger-m2m-tracer-" + seamTenantA,
		},
		{
			name:        "padded tenantId claim still refuses a different x-tenant-id",
			multiTenant: true,
			claims:      ledgerClaims(seamTenantA, " "+seamTenantA+" "),
			pairs:       []string{seamtenant.MetadataKey, seamTenantB},
			wantReason:  principalReasonTenantMismatch,
		},
		{
			name:        "padded name of another product is refused",
			multiTenant: true,
			claims:      padded(ledgerClaims(seamTenantA, seamTenantA), "name", " flowker-m2m-tracer-"+seamTenantA+" "),
			wantReason:  principalReasonNotLedger,
		},
		{
			name:        "padded name of the ledger client of another tenant is refused",
			multiTenant: true,
			claims:      padded(ledgerClaims(seamTenantA, seamTenantA), "name", " ledger-m2m-tracer-"+seamTenantB+" "),
			wantReason:  principalReasonNotLedger,
		},
		{
			name:        "whitespace inside the name is refused",
			multiTenant: true,
			claims:      padded(ledgerClaims(seamTenantA, seamTenantA), "name", "ledger-m2m-tracer- "+seamTenantA),
			wantReason:  principalReasonNotLedger,
		},
		{
			name:        "blank tenantId claim is a missing claim",
			multiTenant: true,
			claims:      ledgerClaims(seamTenantA, "   "),
			wantReason:  principalReasonNoTenantClaim,
		},
		{
			name:     "single-tenant padded tenantId claim matches an equal x-tenant-id",
			claims:   applicationClaims(seamTestClient, " tenant-a "),
			pairs:    []string{seamtenant.MetadataKey, "tenant-a"},
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			interceptor := SeamPrincipalInterceptor(SeamPrincipalConfig{MultiTenant: tt.multiTenant, AllowedClients: []string{seamTestClient}})
			logger := testutil.NewMockLogger()
			pairs := append([]string{"authorization", "Bearer " + seamTestToken(t, tt.claims)}, tt.pairs...)

			var handlerCtx context.Context

			_, err := interceptor(seamIncoming(logger, pairs...), nil, unaryInfo(), func(ctx context.Context, _ any) (any, error) {
				handlerCtx = ctx
				return "ok", nil
			})

			if tt.wantReason == "" {
				require.NoError(t, err)

				principal, ok := contextutil.GetPrincipal(handlerCtx)
				require.True(t, ok)
				assert.Equal(t, tt.wantName, principal.Name, "the principal carries the trimmed name")

				return
			}

			assert.Nil(t, handlerCtx, "a refused call never reaches the handler")
			assert.Equal(t, grpccodes.PermissionDenied, status.Code(err))
			require.Len(t, logger.Calls, 1)

			fields := map[string]any{}
			for _, f := range logger.Calls[0].Fields {
				fields[f.Key] = f.Value
			}

			assert.Equal(t, tt.wantReason, fields["reason"])
		})
	}
}
