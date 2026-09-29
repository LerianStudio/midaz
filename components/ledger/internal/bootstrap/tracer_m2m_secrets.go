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

	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// tracerM2MTargetService is the target segment of the tenant secret path: the
// Tracer's service name in the tenant manager.
const tracerM2MTargetService = "tracer"

// tracerM2MSecretsLoadTimeout bounds loading the AWS configuration at boot.
const tracerM2MSecretsLoadTimeout = 10 * time.Second

// tracerM2MSecretsReaderFactory builds the reader of per-tenant M2M
// credentials.
type tracerM2MSecretsReaderFactory func(ctx context.Context, cfg *Config) (secretsmanager.SecretsManagerClient, error)

// newTracerM2MSecretsReader builds an AWS Secrets Manager client whose region
// and credentials come from the AWS SDK default chain (AWS_REGION and the
// identity injected by the platform, shared config, instance metadata).
func newTracerM2MSecretsReader(ctx context.Context, _ *Config) (secretsmanager.SecretsManagerClient, error) {
	loadCtx, cancel := context.WithTimeout(ctx, tracerM2MSecretsLoadTimeout)
	defer cancel()

	awsCfg, err := awsconfig.LoadDefaultConfig(loadCtx)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration for tenant M2M credentials: %w: %w", err, constant.ErrTracerContractUnavailable)
	}

	return tracerM2MSecretsClient(awsCfg)
}

// tracerM2MSecretsClient refuses a configuration the default chain resolved no
// region for: Secrets Manager is regional, and no region is guessed.
func tracerM2MSecretsClient(awsCfg aws.Config) (secretsmanager.SecretsManagerClient, error) {
	if strings.TrimSpace(awsCfg.Region) == "" {
		return nil, fmt.Errorf("tenant M2M credentials are read from AWS Secrets Manager, but the AWS default chain resolved no region (set AWS_REGION or a shared-config region for the workload): %w", constant.ErrTracerContractUnavailable)
	}

	return awssm.NewFromConfig(awsCfg), nil
}

// buildTenantTracerTokenSource builds the multi-tenant token source: each
// tenant's credentials are read at
// tenants/{ENV_NAME}/{tenant}/{APPLICATION_NAME}/m2m/tracer/credentials.
// A reader that cannot be built refuses boot with
// constant.ErrTracerContractUnavailable. An empty ENV_NAME omits the
// environment segment.
func buildTenantTracerTokenSource(cfg *Config, minter tracerclient.TokenMinter, newSecrets tracerM2MSecretsReaderFactory) (tracerclient.TokenSource, error) {
	application := strings.TrimSpace(cfg.ApplicationName)
	if application == "" {
		return nil, fmt.Errorf("TRACER_TRANSPORT=rest with MULTI_TENANT_ENABLED=true reads tenant M2M credentials under APPLICATION_NAME, which is empty: %w", constant.ErrTracerContractUnavailable)
	}

	if newSecrets == nil {
		return nil, fmt.Errorf("TRACER_TRANSPORT=rest with MULTI_TENANT_ENABLED=true requires a tenant M2M credential reader: %w", constant.ErrTracerContractUnavailable)
	}

	reader, err := newSecrets(context.Background(), cfg)
	if err != nil {
		if errors.Is(err, constant.ErrTracerContractUnavailable) {
			return nil, err
		}

		return nil, fmt.Errorf("build tenant M2M credential reader: %w: %w", err, constant.ErrTracerContractUnavailable)
	}

	if reader == nil {
		return nil, fmt.Errorf("TRACER_TRANSPORT=rest with MULTI_TENANT_ENABLED=true built no tenant M2M credential reader: %w", constant.ErrTracerContractUnavailable)
	}

	credentials, err := tracerclient.NewM2MCredentialProvider(reader, cfg.EnvName, application, tracerM2MTargetService, time.Now)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, constant.ErrTracerContractUnavailable)
	}

	tokens, err := tracerclient.NewTenantTokenSource(credentials, minter, time.Now)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, constant.ErrTracerContractUnavailable)
	}

	return tokens, nil
}
