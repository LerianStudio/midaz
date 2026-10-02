// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"strings"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/apikey"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/jwtclaims"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// Access Manager policy of the reservation seam: product "tracer", resource
// "reservations", action "post" for every RPC. It is the grant the central
// Access Manager seed carries and permissions.yaml declares.
const (
	SeamAuthResource = "reservations"
	SeamAuthAction   = "post"
)

// SeamAPIKeyMetadataKey carries the tracer API key on a seam RPC; it is the
// lower-cased X-API-Key header the HTTP listener reads.
// #nosec G101 -- metadata key name, not a credential value.
const SeamAPIKeyMetadataKey = "x-api-key"

// The tenant-manager names the ledger's per-tenant M2M client of the tracer
// "{source}-m2m-{target}-{tenantID}" in the Access Manager, with tenantID the
// canonical tenant id it also writes into the token's tenantId claim. The
// application name reaches the token as its "name" claim, the claim the
// Access Manager itself resolves an application token's grants from.
const (
	// SeamClientSourceService is the source segment: the ledger.
	SeamClientSourceService = "ledger"
	// SeamClientTargetService is the target segment: the tracer.
	SeamClientTargetService = "tracer"
	// seamClientNamePrefix is the name every ledger client of the tracer
	// starts with; the tenant id follows it.
	seamClientNamePrefix = SeamClientSourceService + "-m2m-" + SeamClientTargetService + "-"
)

// TokenTenantMetadataKey is the incoming metadata key lib-auth fills from the
// token's tenantId claim once it has authorized the call (multi-tenant only).
const TokenTenantMetadataKey = "md-tenant-id"

const (
	// tokenTypeApplication is the token "type" claim of an Access Manager
	// application (client_credentials) token.
	tokenTypeApplication = "application"
	// authorizationMetadataKey carries the bearer token, as lib-auth reads it.
	authorizationMetadataKey = "authorization"
)

// Fixed refusal messages: they name no claim value and no key.
const (
	seamPrincipalRefusedMessage = "caller is not an allowed reservation seam client"
	seamAPIKeyRefusedMessage    = "API key missing or invalid"
)

// Refusal reasons logged by SeamPrincipalInterceptor.
const (
	principalReasonUnreadable     = "token_unreadable"
	principalReasonNotApplication = "not_application_token"
	principalReasonNotAllowed     = "client_not_allowed"
	principalReasonNoTenantClaim  = "tenant_claim_missing"
	principalReasonTenantInvalid  = "tenant_claim_invalid"
	principalReasonNotLedger      = "client_not_ledger"
	principalReasonTenantMismatch = "tenant_mismatch"
)

// SeamAuthPolicyConfig is the lib-auth policy of every ReservationService
// RPC: reservations/post. It has no DefaultPolicy, so an RPC added to the
// service without a mapping is refused (codes.Internal) instead of inheriting
// a grant. The product is always "tracer".
func SeamAuthPolicyConfig() authMiddleware.PolicyConfig {
	policy := authMiddleware.Policy{Resource: SeamAuthResource, Action: SeamAuthAction}

	return authMiddleware.PolicyConfig{
		MethodPolicies: map[string]authMiddleware.Policy{
			reservationv1.ReservationService_Reserve_FullMethodName:              policy,
			reservationv1.ReservationService_ConfirmByTransaction_FullMethodName: policy,
			reservationv1.ReservationService_ConfirmById_FullMethodName:          policy,
			reservationv1.ReservationService_ReleaseByTransaction_FullMethodName: policy,
			reservationv1.ReservationService_ReleaseById_FullMethodName:          policy,
		},
		SubResolver: func(context.Context, string, any) (string, error) {
			return trcConstant.ApplicationName, nil
		},
	}
}

// SeamPrincipalConfig configures SeamPrincipalInterceptor.
type SeamPrincipalConfig struct {
	// MultiTenant selects the multi-tenant rule (the token must be the
	// ledger's client of the tenant its tenantId claim names) over the
	// single-tenant one (the azp or sub claim must be allowlisted).
	MultiTenant bool
	// AllowedClients are the application client ids (token azp; the token
	// sub, "<owner>/<application id>", also matches) admitted in single-tenant
	// mode. Entries are trimmed; blank entries are dropped.
	AllowedClients []string
}

// SeamPrincipalInterceptor admits only the ledger's application principal on
// the reservation seam. It runs after lib-auth, which has already had the
// Access Manager authorize the token, so the claims are read unverified here:
// the token type must be "application"; in single-tenant mode its azp (the
// Access Manager client id) or its sub must be in AllowedClients; in multi-tenant mode its tenantId claim must be a valid
// tenant id and its name claim must be the ledger's client of that same
// tenant ("ledger-m2m-tracer-{tenant}", tenants compared canonically), because
// the tracer/reservations grant is also held by other products' clients.
// When the token carries a tenantId claim, an x-tenant-id or md-tenant-id
// metadata value naming a different tenant is refused (compared by their
// lib-commons canonical form; an id that does not canonicalize is refused),
// so a caller can never steer the tenant away from its own credential. Every
// refusal is codes.PermissionDenied with a fixed message, a business span
// event and one Warn naming only the sub claim and the reason. Every claim is
// judged by its trimmed value.
//
// An admitted call reaches the handler attributed to the application: a
// "user" principal (the actor type the HTTP listener stamps on every bearer
// token) with the sub claim as its ID and the name claim as its Name, so the
// audit trail names the caller instead of the system actor.
func SeamPrincipalInterceptor(cfg SeamPrincipalConfig) grpc.UnaryServerInterceptor {
	allowed := make(map[string]struct{}, len(cfg.AllowedClients))

	for _, client := range cfg.AllowedClients {
		if trimmed := strings.TrimSpace(client); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}

	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		claims, reason := checkSeamPrincipal(ctx, cfg.MultiTenant, allowed)
		if reason != "" {
			return nil, refuseSeamPrincipal(ctx, claims.sub, reason)
		}

		principal := contextutil.Principal{Type: string(model.ActorTypeUser), ID: claims.sub, Name: claims.name}

		return handler(contextutil.WithPrincipal(ctx, principal), req)
	}
}

// SeamAPIKeyInterceptor admits a seam RPC whose x-api-key metadata matches
// key, using the constant-time check the HTTP listener uses. A missing or
// wrong key is codes.Unauthenticated with a fixed message, a business span
// event and one Warn that never carries a key. An admitted call is attributed
// to label (API_KEY_LABEL, or the default label when blank) in the audit
// trail, as on HTTP.
func SeamAPIKeyInterceptor(key, label string) grpc.UnaryServerInterceptor {
	principal := contextutil.Principal{Type: string(model.ActorTypeAPIKey), ID: apikey.ResolveLabel(label)}

	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if reason := apikey.Validate(firstMetadataValue(ctx, SeamAPIKeyMetadataKey), key); reason != "" {
			err := status.Error(codes.Unauthenticated, seamAPIKeyRefusedMessage)

			libOpentelemetry.HandleSpanBusinessErrorEvent(trace.SpanFromContext(ctx), "Reservation seam API key refused", err)

			logger := libObservability.NewLoggerFromContext(ctx)
			logger.Log(ctx, libLog.LevelWarn, "Reservation seam refused the caller's API key",
				libLog.String("operation", "grpc.reservations.authenticate_api_key"),
				libLog.String("reason", reason))

			return nil, err
		}

		return handler(contextutil.WithPrincipal(ctx, principal), req)
	}
}

// seamClaims are the trimmed identity claims of a seam token.
type seamClaims struct {
	sub    string
	name   string
	azp    string
	tenant string
}

// checkSeamPrincipal returns the token's identity claims and the refusal
// reason, or an empty reason when the principal is admitted.
func checkSeamPrincipal(ctx context.Context, multiTenant bool, allowed map[string]struct{}) (seamClaims, string) {
	raw, ok := jwtclaims.ParseUnverified(jwtclaims.StripBearer(firstMetadataValue(ctx, authorizationMetadataKey)))
	if !ok {
		return seamClaims{}, principalReasonUnreadable
	}

	claims := seamClaims{
		sub:    jwtclaims.String(raw, "sub"),
		name:   jwtclaims.String(raw, "name"),
		azp:    jwtclaims.String(raw, "azp"),
		tenant: jwtclaims.String(raw, "tenantId"),
	}

	if jwtclaims.String(raw, "type") != tokenTypeApplication {
		return claims, principalReasonNotApplication
	}

	if multiTenant {
		if reason := checkLedgerClient(claims); reason != "" {
			return claims, reason
		}
	} else {
		_, byClientID := allowed[claims.azp]
		_, bySub := allowed[claims.sub]

		if !byClientID && !bySub {
			return claims, principalReasonNotAllowed
		}
	}

	if claims.tenant != "" && !tenantMetadataMatches(ctx, claims.tenant) {
		return claims, principalReasonTenantMismatch
	}

	return claims, ""
}

// checkLedgerClient returns the refusal reason of a multi-tenant token that is
// not the ledger's client of the tenant its tenantId claim names, or "".
func checkLedgerClient(claims seamClaims) string {
	if claims.tenant == "" {
		return principalReasonNoTenantClaim
	}

	if _, err := tmcore.CanonicalTenantID(claims.tenant); err != nil {
		return principalReasonTenantInvalid
	}

	nameTenant, ok := strings.CutPrefix(claims.name, seamClientNamePrefix)
	if !ok || !sameTenant(nameTenant, claims.tenant) {
		return principalReasonNotLedger
	}

	return ""
}

// refuseSeamPrincipal records a refused principal and returns its status.
func refuseSeamPrincipal(ctx context.Context, sub, reason string) error {
	err := status.Error(codes.PermissionDenied, seamPrincipalRefusedMessage)

	libOpentelemetry.HandleSpanBusinessErrorEvent(trace.SpanFromContext(ctx), "Reservation seam principal refused", err)

	logger := libObservability.NewLoggerFromContext(ctx)
	logger.Log(ctx, libLog.LevelWarn, "Reservation seam refused the caller's principal",
		libLog.String("operation", "grpc.reservations.authorize_principal"),
		libLog.String("reason", reason),
		libLog.String("sub", sub))

	return err
}

// tenantMetadataMatches reports whether every tenant carrier present in the
// incoming metadata names the same tenant as the token's tenantId claim.
func tenantMetadataMatches(ctx context.Context, tenantClaim string) bool {
	for _, key := range []string{seamtenant.MetadataKey, TokenTenantMetadataKey} {
		if value := firstMetadataValue(ctx, key); value != "" && !sameTenant(value, tenantClaim) {
			return false
		}
	}

	return true
}

// sameTenant reports whether a and b both canonicalize (tmcore.CanonicalTenantID)
// to the same tenant id. An id that does not canonicalize matches nothing.
func sameTenant(a, b string) bool {
	ca, errA := tmcore.CanonicalTenantID(a)
	cb, errB := tmcore.CanonicalTenantID(b)

	return errA == nil && errB == nil && ca == cb
}

// firstMetadataValue returns the first incoming metadata value of key, or ""
// when absent.
func firstMetadataValue(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}
