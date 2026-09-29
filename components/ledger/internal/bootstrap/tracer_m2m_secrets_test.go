// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/aws/aws-sdk-go-v2/aws"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/require"

	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// recordingCustody holds no secret and records every path it is asked for.
type recordingCustody struct {
	mu    sync.Mutex
	paths []string
}

func (c *recordingCustody) GetSecretValue(_ context.Context, params *awssm.GetSecretValueInput, _ ...func(*awssm.Options)) (*awssm.GetSecretValueOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.paths = append(c.paths, aws.ToString(params.SecretId))

	return nil, secretsmanager.ErrBackendSecretNotFound
}

func (c *recordingCustody) readPaths() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Clone(c.paths)
}

// countingSecretsFactory records how often boot asked for a custody reader.
type countingSecretsFactory struct {
	calls   atomic.Int32
	err     error
	custody recordingCustody
}

func (f *countingSecretsFactory) build(context.Context, *Config) (secretsmanager.SecretsManagerClient, error) {
	f.calls.Add(1)

	if f.err != nil {
		return nil, f.err
	}

	return &f.custody, nil
}

// multiTenantRESTTracerTestConfig is a multi-tenant REST integration without
// static M2M credentials: each tenant's own are read from custody.
func multiTenantRESTTracerTestConfig() Config {
	cfg := restTracerTestConfig()
	cfg.TracerTLSMode, cfg.TracerBaseURL = "mesh", "http://tracer.test:4020"
	cfg.MultiTenantEnabled = true
	cfg.ApplicationName, cfg.EnvName = "ledger", "staging"
	cfg.IDPM2MClientID, cfg.IDPM2MClientSecret = "", ""

	return cfg
}

func TestContextTracerRESTMultiTenantUsesTenantCredentials(t *testing.T) {
	t.Parallel()

	cfg := multiTenantRESTTracerTestConfig()
	minter := &countingTokenMinter{}
	secrets := &countingSecretsFactory{}

	runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, minter, testTracerAuthHost, secrets.build, newBootstrapTestLogger(t))
	require.NoError(t, err, "static IDP_M2M credentials are not required under multi-tenancy")
	require.NotNil(t, runtime)
	require.Equal(t, int32(1), secrets.calls.Load())
	require.Zero(t, minter.calls.Load(), "no tenant exists at boot, so nothing is pre-minted")
}

func TestBuildTracerTokenSourceMultiTenantReadsTheTenantCustodyPath(t *testing.T) {
	t.Parallel()

	for name, scenario := range map[string]struct {
		env  string
		want string
	}{
		"with environment":    {env: "staging", want: "tenants/staging/tenant-a/ledger/m2m/tracer/credentials"},
		"without environment": {env: "", want: "tenants/tenant-a/ledger/m2m/tracer/credentials"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg := multiTenantRESTTracerTestConfig()
			cfg.EnvName = scenario.env
			minter := &countingTokenMinter{}
			secrets := &countingSecretsFactory{}

			tokens, err := buildTracerTokenSource(&cfg, minter, testTracerAuthHost, "mesh", secrets.build)
			require.NoError(t, err)

			_, err = tokens.Token(tmcore.ContextWithTenantID(t.Context(), "tenant-a"))
			require.ErrorIs(t, err, tracerclient.ErrTracerRequestRejected, "an unprovisioned tenant is refused")
			require.ErrorIs(t, err, secretsmanager.ErrM2MCredentialsNotFound)
			require.Equal(t, []string{scenario.want}, secrets.custody.readPaths())
			require.Zero(t, minter.calls.Load())
		})
	}
}

func TestBuildTracerTokenSourceMultiTenantNeverFallsBackToStaticCredentials(t *testing.T) {
	t.Parallel()

	cfg := multiTenantRESTTracerTestConfig()
	cfg.IDPM2MClientID, cfg.IDPM2MClientSecret = "ledger-client", "ledger-secret"
	minter := &countingTokenMinter{}
	secrets := &countingSecretsFactory{}

	tokens, err := buildTracerTokenSource(&cfg, minter, testTracerAuthHost, "mesh", secrets.build)
	require.NoError(t, err)
	require.IsType(t, &tracerclient.TenantTokenSource{}, tokens)

	_, err = tokens.Token(context.Background())
	require.ErrorIs(t, err, constant.ErrReservationTenantRequired)
	require.Zero(t, minter.calls.Load())
}

func TestContextTracerRESTMultiTenantBootRules(t *testing.T) {
	t.Parallel()

	for name, scenario := range map[string]struct {
		mutate  func(*Config)
		secrets func(context.Context, *Config) (secretsmanager.SecretsManagerClient, error)
		mention string
	}{
		"reader cannot be built": {
			secrets: (&countingSecretsFactory{err: errors.New("custody unreachable")}).build,
			mention: "custody unreachable",
		},
		"no reader factory": {mention: "tenant M2M credential reader"},
		"no application name": {
			mutate:  func(cfg *Config) { cfg.ApplicationName = " " },
			secrets: (&countingSecretsFactory{}).build,
			mention: "APPLICATION_NAME",
		},
		"plugin auth disabled": {
			mutate:  func(cfg *Config) { cfg.AuthEnabled = false },
			secrets: (&countingSecretsFactory{}).build,
			mention: "PLUGIN_AUTH_ENABLED=false",
		},
		"traversal application name": {
			mutate:  func(cfg *Config) { cfg.ApplicationName = "../ledger" },
			secrets: (&countingSecretsFactory{}).build,
			mention: "is not a Tracer reservation producer",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg := multiTenantRESTTracerTestConfig()
			if scenario.mutate != nil {
				scenario.mutate(&cfg)
			}

			runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, &countingTokenMinter{}, testTracerAuthHost, scenario.secrets, newBootstrapTestLogger(t))
			require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
			require.ErrorContains(t, err, scenario.mention)
			require.Nil(t, runtime)
		})
	}
}

func TestContextTracerSingleTenantNeverReadsCustody(t *testing.T) {
	t.Parallel()

	cfg := restTracerTestConfig()
	cfg.TracerTLSMode, cfg.TracerBaseURL = "mesh", "http://tracer.test:4020"
	minter := &countingTokenMinter{}
	secrets := &countingSecretsFactory{}

	runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, minter, testTracerAuthHost, secrets.build, newBootstrapTestLogger(t))
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.Zero(t, secrets.calls.Load())
	require.Equal(t, int32(1), minter.calls.Load(), "single-tenant pre-mints its static token")
}

func TestContextTracerGRPCMultiTenantNeedsNoCustody(t *testing.T) {
	t.Parallel()

	certs := writeSeamCertFiles(t)
	cfg := contextTracerTestConfig()
	cfg.TracerTransport, cfg.MultiTenantEnabled = "grpc", true
	cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile, cfg.TracerTLSCAFile = certs.certFile, certs.keyFile, certs.caFile
	secrets := &countingSecretsFactory{}

	runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, nil, testTracerAuthHost, secrets.build, newBootstrapTestLogger(t))
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.Zero(t, secrets.calls.Load())

	closeContextTracerRuntime(t, runtime)
}

func TestNewTracerM2MSecretsReaderUsesTheAWSDefaultChain(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_DEFAULT_REGION", "")

	t.Run("region resolved by the chain", func(t *testing.T) {
		t.Setenv("AWS_REGION", "us-east-1")

		reader, err := newTracerM2MSecretsReader(t.Context(), &Config{})
		require.NoError(t, err)
		require.IsType(t, &awssm.Client{}, reader)
	})

	t.Run("no region resolved refuses boot", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")

		reader, err := newTracerM2MSecretsReader(t.Context(), &Config{})
		require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
		require.ErrorContains(t, err, "AWS default chain resolved no region")
		require.Nil(t, reader)
	})
}

func TestTracerM2MSecretsClientRefusesAnEmptyRegion(t *testing.T) {
	t.Parallel()

	for name, region := range map[string]string{"empty": "", "blank": "  "} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			reader, err := tracerM2MSecretsClient(aws.Config{Region: region})
			require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
			require.Nil(t, reader)
		})
	}

	reader, err := tracerM2MSecretsClient(aws.Config{Region: "sa-east-1"})
	require.NoError(t, err)
	require.NotNil(t, reader)
}
