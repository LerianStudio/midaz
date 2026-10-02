// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
)

// tracerSeamIdentity names the credential the ledger presents on the tracer
// reservation seam. It is logged at boot and never carries secret material.
type tracerSeamIdentity string

const (
	// seamIdentityNone sends no credential: identity is left to the transport
	// (mtls or a mesh), or the seam runs unidentified where the tracer allows it.
	seamIdentityNone tracerSeamIdentity = "none"
	// seamIdentityStaticToken presents an application token minted from the
	// single-tenant TRACER_M2M_CLIENT_ID / TRACER_M2M_CLIENT_SECRET pair.
	seamIdentityStaticToken tracerSeamIdentity = "m2m-static"
	// seamIdentityTenantToken presents an application token minted from the
	// calling tenant's own credential, read from the secret store.
	seamIdentityTenantToken tracerSeamIdentity = "m2m-tenant"
	// seamIdentityAPIKey presents TRACER_API_KEY.
	seamIdentityAPIKey tracerSeamIdentity = "api-key"
)

// errSeamAuthClientNotAuthorizing refuses token identity on an Access Manager
// client that is nil, disabled or has no address: every mint would fail, so no
// seam call would ever be sent.
var errSeamAuthClientNotAuthorizing = errors.New("reservation seam: PLUGIN_AUTH_ENABLED=true requires an enabled Access Manager client with an address (PLUGIN_AUTH_HOST)")

// tracerSeamDeps carries the collaborators the seam identity is built from.
// authClient is the ledger's Access Manager client, which mints the seam token
// unless minter overrides it; tenantServiceName is the application segment the
// tenant-manager writes the ledger's M2M secrets under; newAWSSecretsClient
// builds the AWS Secrets Manager client when that is the custody backend (from
// the default AWS credential chain when nil).
type tracerSeamDeps struct {
	authClient          *middleware.AuthClient
	minter              tracerclient.TokenMinter
	tenantServiceName   string
	newAWSSecretsClient func(ctx context.Context) (secretsmanager.SecretsManagerClient, error)
}

// buildTracerSeamIdentity selects the one credential every seam call carries.
// With PLUGIN_AUTH_ENABLED=true it is an Access Manager application token: the
// static TRACER_M2M_CLIENT_ID / TRACER_M2M_CLIENT_SECRET pair in single-tenant
// mode, whose token is warmed in the background, and each tenant's own
// credential from the secret store in multi-tenant mode (the static pair is
// never a fallback there, because its token would carry no tenant). With auth
// disabled it is TRACER_API_KEY when set, otherwise nothing. Auth together with
// an API key is refused: a deployment has one seam identity.
func buildTracerSeamIdentity(ctx context.Context, cfg *Config, logger libLog.Logger, deps tracerSeamDeps) (tracerSeamIdentity, []tracerclient.TracerGRPCClientOption, error) {
	apiKey := strings.TrimSpace(cfg.TracerAPIKey)

	if !cfg.AuthEnabled {
		if apiKey == "" {
			return seamIdentityNone, nil, nil
		}

		return seamIdentityAPIKey, []tracerclient.TracerGRPCClientOption{tracerclient.WithAPIKey(apiKey)}, nil
	}

	if apiKey != "" {
		return "", nil, errors.New("reservation seam accepts one identity: PLUGIN_AUTH_ENABLED=true presents an Access Manager token, so unset TRACER_API_KEY")
	}

	if deps.authClient == nil || !deps.authClient.Enabled || strings.TrimSpace(deps.authClient.Address) == "" {
		return "", nil, errSeamAuthClientNotAuthorizing
	}

	minter := deps.minter
	if minter == nil {
		minter = deps.authClient
	}

	identity, creds, err := buildTracerSeamCredentials(ctx, cfg, deps)
	if err != nil {
		return "", nil, err
	}

	tokenTimeout, err := tracerM2MTokenTimeout(cfg)
	if err != nil {
		return "", nil, err
	}

	src, err := tracerclient.NewM2MTokenSource(minter, creds,
		tracerclient.WithTokenLogger(logger), tracerclient.WithTokenWaitTimeout(tokenTimeout))
	if err != nil {
		return "", nil, err
	}

	if identity == seamIdentityStaticToken {
		src.Warm(ctx, logger)
	}

	return identity, []tracerclient.TracerGRPCClientOption{tracerclient.WithM2MCredentials(src)}, nil
}

// tracerM2MTokenTimeout resolves TRACER_M2M_TOKEN_TIMEOUT_MS: unset is
// tracerclient.DefaultTokenWaitTimeout, and a value must be positive and no
// longer than the mint the wait is for (tracerclient.MaxTokenWaitTimeout).
func tracerM2MTokenTimeout(cfg *Config) (time.Duration, error) {
	ms := cfg.TracerM2MTokenTimeoutMs
	if ms == 0 {
		return tracerclient.DefaultTokenWaitTimeout, nil
	}

	maxMs := int(tracerclient.MaxTokenWaitTimeout.Milliseconds())
	if ms < 0 || ms > maxMs {
		return 0, fmt.Errorf("invalid TRACER_M2M_TOKEN_TIMEOUT_MS %d: expected 1..%d", ms, maxMs)
	}

	return time.Duration(ms) * time.Millisecond, nil
}

// buildTracerSeamCredentials resolves the credential provider for token
// identity.
func buildTracerSeamCredentials(ctx context.Context, cfg *Config, deps tracerSeamDeps) (tracerSeamIdentity, tracerclient.CredentialProvider, error) {
	if !cfg.MultiTenantEnabled {
		clientID := strings.TrimSpace(cfg.TracerM2MClientID)
		if clientID == "" {
			return "", nil, errors.New("reservation seam token identity requires TRACER_M2M_CLIENT_ID when PLUGIN_AUTH_ENABLED=true and TRACER_BASE_URL is set")
		}

		clientSecret := strings.TrimSpace(cfg.TracerM2MClientSecret)
		if clientSecret == "" {
			return "", nil, errors.New("reservation seam token identity requires TRACER_M2M_CLIENT_SECRET when PLUGIN_AUTH_ENABLED=true and TRACER_BASE_URL is set")
		}

		return seamIdentityStaticToken, tracerclient.NewStaticCredentials(clientID, clientSecret), nil
	}

	applicationName := strings.TrimSpace(deps.tenantServiceName)
	if applicationName == "" {
		return "", nil, errors.New("reservation seam tenant credentials require APPLICATION_NAME, the ledger's tenant-manager service name")
	}

	reader, err := buildM2MSecretsReader(ctx, cfg, deps)
	if err != nil {
		return "", nil, err
	}

	fetch := tracerclient.NewSecretsManagerCredentialFetcher(reader, strings.TrimSpace(cfg.EnvName), applicationName)

	creds, err := tracerclient.NewTenantCredentials(fetch)
	if err != nil {
		return "", nil, err
	}

	return seamIdentityTenantToken, creds, nil
}

// buildM2MSecretsReader builds the reader of the custody backend
// M2M_SECRETS_BACKEND selects: AWS Secrets Manager by default, or Vault KV v2
// under M2M_VAULT_MOUNT, connected through Vault's own environment (VAULT_ADDR,
// VAULT_TOKEN, VAULT_CACERT, VAULT_NAMESPACE). An unknown backend refuses boot;
// a selected backend that cannot be built never falls back to the other.
func buildM2MSecretsReader(ctx context.Context, cfg *Config, deps tracerSeamDeps) (secretsmanager.SecretsManagerClient, error) {
	backend, err := secretsmanager.ParseBackend(cfg.M2MSecretsBackend)
	if err != nil {
		return nil, fmt.Errorf("M2M_SECRETS_BACKEND: %w", err)
	}

	var awsClient secretsmanager.SecretsManagerClient

	if backend == secretsmanager.BackendAWS {
		newAWSClient := deps.newAWSSecretsClient
		if newAWSClient == nil {
			newAWSClient = newAWSSecretsManagerClient
		}

		awsClient, err = newAWSClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("build the secret store client for the reservation seam M2M credentials: %w", err)
		}
	}

	custody := secretsmanager.Config{
		Backend: backend,
		Vault:   secretsmanager.VaultConfig{Mount: strings.TrimSpace(cfg.M2MVaultMount)},
	}

	reader, err := custody.NewReader(awsClient)
	if err != nil {
		return nil, fmt.Errorf("build the secret store reader for the reservation seam M2M credentials: %w", err)
	}

	return reader, nil
}

// newAWSSecretsManagerClient builds the AWS Secrets Manager client from the
// default credential chain; AWS_REGION selects the region.
func newAWSSecretsManagerClient(ctx context.Context) (secretsmanager.SecretsManagerClient, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	return awssm.NewFromConfig(awsCfg), nil
}
