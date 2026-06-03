// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package auth

import (
	"context"
	"fmt"

	libSM "github.com/LerianStudio/lib-commons/v5/commons/secretsmanager"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	sm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// SMCredentialFetcherConfig holds configuration for creating an SMCredentialFetcher.
type SMCredentialFetcherConfig struct {
	// AWSRegion is the AWS region for Secrets Manager access (e.g., "us-east-1").
	AWSRegion string

	// Environment is the deployment environment used in the secret path
	// (e.g., "staging", "production"). Maps to MULTI_TENANT_ENVIRONMENT.
	Environment string

	// ApplicationName is the service name used in the secret path (e.g., "reporter").
	ApplicationName string
}

// SMCredentialFetcher implements CredentialFetcher by fetching M2M credentials
// from AWS Secrets Manager via lib-commons GetM2MCredentials.
//
// Secret path convention (from lib-commons):
//
//	tenants/{env}/{tenantOrgID}/{applicationName}/m2m/{targetService}/credentials
type SMCredentialFetcher struct {
	client          libSM.SecretsManagerClient
	environment     string
	applicationName string
}

// NewSMCredentialFetcher creates a new SMCredentialFetcher backed by AWS Secrets Manager.
func NewSMCredentialFetcher(ctx context.Context, cfg SMCredentialFetcherConfig) (*SMCredentialFetcher, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return nil, fmt.Errorf("load AWS config for Secrets Manager: %w", err)
	}

	return &SMCredentialFetcher{
		client:          sm.NewFromConfig(awsCfg),
		environment:     cfg.Environment,
		applicationName: cfg.ApplicationName,
	}, nil
}

// newSMCredentialFetcherWithClient creates an SMCredentialFetcher with an injected
// SecretsManagerClient. Used in unit tests (sm_credential_fetcher_test.go).
//
//nolint:unused // used in unit tests with //go:build unit tag
func newSMCredentialFetcherWithClient(client libSM.SecretsManagerClient, environment, applicationName string) *SMCredentialFetcher {
	return &SMCredentialFetcher{
		client:          client,
		environment:     environment,
		applicationName: applicationName,
	}
}

// FetchCredential retrieves M2M credentials for the given tenant and target service
// from AWS Secrets Manager.
func (f *SMCredentialFetcher) FetchCredential(ctx context.Context, tenantID, targetService string) (*M2MCredential, error) {
	creds, err := libSM.GetM2MCredentials(ctx, f.client, f.environment, tenantID, f.applicationName, targetService)
	if err != nil {
		return nil, fmt.Errorf("fetch M2M credential for tenant %s, service %s: %w", tenantID, targetService, err)
	}

	return &M2MCredential{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
	}, nil
}

// BuildCredentialFetcher creates an SMCredentialFetcher when M2M auth conditions
// are met (AWSRegion configured). Returns nil when AWSRegion is empty, allowing
// callers to gracefully fall back to no-auth mode (single-tenant).
func BuildCredentialFetcher(ctx context.Context, cfg SMCredentialFetcherConfig) (CredentialFetcher, error) {
	if cfg.AWSRegion == "" {
		return nil, nil
	}

	return NewSMCredentialFetcher(ctx, cfg)
}
