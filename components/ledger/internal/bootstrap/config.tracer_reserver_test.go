// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/aws/aws-sdk-go-v2/aws"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// TestBuildTracerReserver_MTLSGuard pins the boot-time fail-fast guard after the
// mTLS rework: identity on the reservation seam is mutual TLS, so the
// discriminator is the transport's security, NOT tenancy. With the integration
// off (TRACER_BASE_URL empty) the reserver is a genuine nil in every tenancy
// mode. With it on and TRACER_TLS_MODE=mtls, missing cert/key/CA material fails
// fast ("reservation seam requires mTLS material"); complete material wires a
// non-nil reserver in BOTH single- and multi-tenant mode (MT no longer gates the
// seam). With TRACER_TLS_MODE=mesh the operator's sidecar terminates mTLS, so no
// cert material is required and the reserver wires non-nil.
func TestBuildTracerReserver_MTLSGuard(t *testing.T) {
	t.Parallel()

	logger := newBootstrapTestLogger(t)

	type certMode int

	const (
		certNone certMode = iota
		certPresent
	)

	tests := []struct {
		name               string
		multiTenantEnabled bool
		tracerBaseURL      string
		tlsMode            string
		certs              certMode
		wantErrContains    string
		wantReserverNil    bool
	}{
		{
			name:            "integration off boots disabled (single-tenant)",
			tracerBaseURL:   "",
			wantReserverNil: true,
		},
		{
			name:               "integration off boots disabled (multi-tenant)",
			multiTenantEnabled: true,
			tracerBaseURL:      "",
			wantReserverNil:    true,
		},
		{
			name:            "mtls with missing material fails fast",
			tracerBaseURL:   "https://tracer:4020",
			tlsMode:         "mtls",
			certs:           certNone,
			wantErrContains: "reservation seam requires mTLS material",
		},
		{
			name:          "mtls with material boots (single-tenant)",
			tracerBaseURL: "https://tracer:4020",
			tlsMode:       "mtls",
			certs:         certPresent,
		},
		{
			name:               "mtls with material boots (multi-tenant)",
			multiTenantEnabled: true,
			tracerBaseURL:      "https://tracer:4020",
			tlsMode:            "mtls",
			certs:              certPresent,
		},
		{
			name:          "mesh boots without certs (single-tenant)",
			tracerBaseURL: "https://tracer:4020",
			tlsMode:       "mesh",
			certs:         certNone,
		},
		{
			name:               "mesh boots without certs (multi-tenant)",
			multiTenantEnabled: true,
			tracerBaseURL:      "https://tracer:4020",
			tlsMode:            "mesh",
			certs:              certNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				MultiTenantEnabled: tt.multiTenantEnabled,
				TracerBaseURL:      tt.tracerBaseURL,
				TracerTLSMode:      tt.tlsMode,
			}

			if tt.certs == certPresent {
				files := writeSeamCertFiles(t)
				cfg.TracerTLSCertFile = files.certFile
				cfg.TracerTLSKeyFile = files.keyFile
				cfg.TracerTLSCAFile = files.caFile
			}

			reserver, err := buildTracerReserver(context.Background(), cfg, logger, tracerSeamDeps{})

			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				assert.Nil(t, reserver)

				return
			}

			require.NoError(t, err)

			if tt.wantReserverNil {
				assert.Nil(t, reserver)
			} else {
				assert.NotNil(t, reserver)
			}

			if closer, ok := reserver.(interface{ Close() error }); ok {
				t.Cleanup(func() { _ = closer.Close() })
			}
		})
	}
}

// TestBuildTracerReserver_TransportSelection pins the TRACER_TRANSPORT handling:
// the empty default and "grpc" (normalized: trimmed, case-insensitive) build the
// gRPC client, the stale "rest" value refuses boot with the contract message,
// and any other value fails fast listing grpc as the only accepted transport.
// The integration is single-tenant in every case so the multi-tenant boot guard
// does not fire. grpc.NewClient is lazy, so building the gRPC reserver never
// blocks on tracer reachability.
func TestBuildTracerReserver_TransportSelection(t *testing.T) {
	t.Parallel()

	logger := newBootstrapTestLogger(t)

	tests := []struct {
		name            string
		transport       string
		wantType        any
		wantErr         string
		wantErrContains string
	}{
		{
			name:      "empty defaults to gRPC",
			transport: "",
			wantType:  &tracerclient.TracerGRPCClient{},
		},
		{
			name:      "grpc selects gRPC client",
			transport: "grpc",
			wantType:  &tracerclient.TracerGRPCClient{},
		},
		{
			name:      "GRPC with surrounding whitespace is normalized",
			transport: " GRPC ",
			wantType:  &tracerclient.TracerGRPCClient{},
		},
		{
			name:      "rest refuses boot with the contract message",
			transport: "rest",
			wantErr:   "TRACER_TRANSPORT=rest is no longer supported: the ledger reaches the tracer over gRPC only; unset TRACER_TRANSPORT",
		},
		{
			name:            "unknown transport fails fast listing grpc only",
			transport:       "soap",
			wantErrContains: `invalid TRACER_TRANSPORT "soap": expected "grpc"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				TracerBaseURL:   "http://tracer:4021",
				TracerTransport: tt.transport,
			}

			reserver, err := buildTracerReserver(context.Background(), cfg, logger, tracerSeamDeps{})

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
				assert.Nil(t, reserver)

				return
			}

			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				assert.NotContains(t, err.Error(), `"rest"`)
				assert.Nil(t, reserver)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, reserver)
			assert.IsType(t, tt.wantType, reserver)

			// The gRPC reserver holds a persistent connection; close it so its
			// background goroutines do not trip the package goleak check.
			if closer, ok := reserver.(interface{ Close() error }); ok {
				t.Cleanup(func() { _ = closer.Close() })
			}
		})
	}
}

// recordingMinter is a TokenMinter that never reaches the network and counts
// its mints.
type recordingMinter struct {
	mints atomic.Int64
}

func (m *recordingMinter) GetApplicationToken(context.Context, string, string) (string, error) {
	m.mints.Add(1)

	return "minted-token", nil
}

// fakeSecretsClient satisfies secretsmanager.SecretsManagerClient and answers
// every read with one M2M credential document.
type fakeSecretsClient struct{}

func (fakeSecretsClient) GetSecretValue(context.Context, *awssm.GetSecretValueInput, ...func(*awssm.Options)) (*awssm.GetSecretValueOutput, error) {
	return &awssm.GetSecretValueOutput{SecretString: aws.String(`{"clientId":"ledger-tenant","clientSecret":"s3cret"}`)}, nil
}

// authorizingClient is an Access Manager client that would authorize calls.
func authorizingClient() *middleware.AuthClient {
	return &middleware.AuthClient{Enabled: true, Address: "http://access-manager:4000"}
}

// tokenDeps are the collaborators token identity needs, with a minter that never
// reaches the network.
func tokenDeps() tracerSeamDeps {
	return tracerSeamDeps{authClient: authorizingClient(), minter: &recordingMinter{}}
}

func TestBuildTracerSeamIdentity(t *testing.T) {
	t.Parallel()

	files := writeSeamCertFiles(t)

	tenantDeps := func() tracerSeamDeps {
		deps := tokenDeps()
		deps.tenantServiceName = "ledger"

		return deps
	}

	tests := []struct {
		name              string
		cfg               Config
		deps              tracerSeamDeps
		wantIdentity      tracerSeamIdentity
		wantErrContains   []string
		wantErrIs         error
		wantSecretsClient bool
	}{
		{
			name:         "auth disabled without an API key leaves identity to the transport",
			cfg:          Config{},
			wantIdentity: seamIdentityNone,
		},
		{
			name:         "auth disabled with an API key presents the key",
			cfg:          Config{TracerAPIKey: "k3y"},
			wantIdentity: seamIdentityAPIKey,
		},
		{
			name:         "single-tenant auth presents the static M2M pair",
			cfg:          Config{AuthEnabled: true, TracerM2MClientID: "ledger", TracerM2MClientSecret: "s3cret"},
			deps:         tokenDeps(),
			wantIdentity: seamIdentityStaticToken,
		},
		{
			name:            "single-tenant auth without a client id refuses boot",
			cfg:             Config{AuthEnabled: true, TracerM2MClientSecret: "s3cret"},
			deps:            tokenDeps(),
			wantErrContains: []string{"TRACER_M2M_CLIENT_ID"},
		},
		{
			name:            "single-tenant auth without a client secret refuses boot",
			cfg:             Config{AuthEnabled: true, TracerM2MClientID: "ledger"},
			deps:            tokenDeps(),
			wantErrContains: []string{"TRACER_M2M_CLIENT_SECRET"},
		},
		{
			name:            "auth and an API key together refuse boot",
			cfg:             Config{AuthEnabled: true, TracerM2MClientID: "ledger", TracerM2MClientSecret: "s3cret", TracerAPIKey: "k3y"},
			deps:            tokenDeps(),
			wantErrContains: []string{"PLUGIN_AUTH_ENABLED", "TRACER_API_KEY"},
		},
		{
			name:      "token identity without an Access Manager client refuses boot",
			cfg:       Config{AuthEnabled: true, TracerM2MClientID: "ledger", TracerM2MClientSecret: "s3cret"},
			deps:      tracerSeamDeps{minter: &recordingMinter{}},
			wantErrIs: errSeamAuthClientNotAuthorizing,
		},
		{
			name:      "token identity with a disabled Access Manager client refuses boot",
			cfg:       Config{AuthEnabled: true, TracerM2MClientID: "ledger", TracerM2MClientSecret: "s3cret"},
			deps:      tracerSeamDeps{authClient: &middleware.AuthClient{Address: "http://access-manager:4000"}},
			wantErrIs: errSeamAuthClientNotAuthorizing,
		},
		{
			name:      "token identity with an Access Manager client without an address refuses boot",
			cfg:       Config{AuthEnabled: true, MultiTenantEnabled: true, EnvName: "staging"},
			deps:      tracerSeamDeps{authClient: &middleware.AuthClient{Enabled: true}, tenantServiceName: "ledger"},
			wantErrIs: errSeamAuthClientNotAuthorizing,
		},
		{
			name:              "multi-tenant auth reads each tenant's credential from the secret store",
			cfg:               Config{AuthEnabled: true, MultiTenantEnabled: true, EnvName: "staging"},
			deps:              tenantDeps(),
			wantIdentity:      seamIdentityTenantToken,
			wantSecretsClient: true,
		},
		{
			name:              "multi-tenant auth defaults to the AWS custody backend",
			cfg:               Config{AuthEnabled: true, MultiTenantEnabled: true, EnvName: "staging", M2MSecretsBackend: "AWS"},
			deps:              tenantDeps(),
			wantIdentity:      seamIdentityTenantToken,
			wantSecretsClient: true,
		},
		{
			name:            "multi-tenant auth refuses an unknown custody backend",
			cfg:             Config{AuthEnabled: true, MultiTenantEnabled: true, EnvName: "staging", M2MSecretsBackend: "gcp"},
			deps:            tenantDeps(),
			wantErrIs:       secretsmanager.ErrBackendUnknown,
			wantErrContains: []string{"M2M_SECRETS_BACKEND"},
		},
		{
			name:              "multi-tenant auth ignores the static pair",
			cfg:               Config{AuthEnabled: true, MultiTenantEnabled: true, EnvName: "staging", TracerM2MClientID: "ledger", TracerM2MClientSecret: "s3cret"},
			deps:              tenantDeps(),
			wantIdentity:      seamIdentityTenantToken,
			wantSecretsClient: true,
		},
		{
			name:            "multi-tenant auth without an application name refuses boot",
			cfg:             Config{AuthEnabled: true, MultiTenantEnabled: true, EnvName: "staging"},
			deps:            tokenDeps(),
			wantErrContains: []string{"APPLICATION_NAME"},
		},
		{
			name: "multi-tenant auth refuses boot when the secret store client cannot be built",
			cfg:  Config{AuthEnabled: true, MultiTenantEnabled: true, EnvName: "staging"},
			deps: func() tracerSeamDeps {
				deps := tenantDeps()
				deps.newAWSSecretsClient = func(context.Context) (secretsmanager.SecretsManagerClient, error) {
					return nil, errors.New("no aws region")
				}

				return deps
			}(),
			wantErrContains: []string{"secret store"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := tt.cfg
			deps := tt.deps

			var secretsClientBuilt atomic.Bool

			if tt.wantSecretsClient {
				deps.newAWSSecretsClient = func(context.Context) (secretsmanager.SecretsManagerClient, error) {
					secretsClientBuilt.Store(true)

					return fakeSecretsClient{}, nil
				}
			}

			identity, opts, err := buildTracerSeamIdentity(context.Background(), &cfg, newBootstrapTestLogger(t), deps)

			if len(tt.wantErrContains) > 0 || tt.wantErrIs != nil {
				require.Error(t, err)

				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				for _, want := range tt.wantErrContains {
					assert.Contains(t, err.Error(), want)
				}

				assert.NotContains(t, err.Error(), "s3cret")
				assert.NotContains(t, err.Error(), "k3y")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantIdentity, identity)
			assert.Equal(t, tt.wantSecretsClient, secretsClientBuilt.Load())

			if tt.wantIdentity == seamIdentityNone {
				assert.Empty(t, opts)
			} else {
				assert.Len(t, opts, 1, "exactly one identity option is wired")
			}
		})
	}

	t.Run("the reserver refuses boot on an identity error and boots on a valid one", func(t *testing.T) {
		t.Parallel()

		logger := newBootstrapTestLogger(t)

		bad := &Config{TracerBaseURL: "tracer:4021", AuthEnabled: true, TracerAPIKey: "k3y"}
		reserver, err := buildTracerReserver(context.Background(), bad, logger, tokenDeps())
		require.Error(t, err)
		assert.Nil(t, reserver)

		good := &Config{
			TracerBaseURL:         "tracer:4021",
			TracerTLSMode:         "server",
			TracerTLSCAFile:       files.caFile,
			AuthEnabled:           true,
			TracerM2MClientID:     "ledger",
			TracerM2MClientSecret: "s3cret",
		}
		reserver, err = buildTracerReserver(context.Background(), good, logger, tokenDeps())
		require.NoError(t, err)
		require.NotNil(t, reserver)

		if closer, ok := reserver.(interface{ Close() error }); ok {
			t.Cleanup(func() { _ = closer.Close() })
		}
	})

	t.Run("integration off checks no identity", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{AuthEnabled: true, TracerAPIKey: "k3y"}

		reserver, err := buildTracerReserver(context.Background(), cfg, newBootstrapTestLogger(t), tracerSeamDeps{})
		require.NoError(t, err)
		assert.Nil(t, reserver)
	})

	t.Run("server mode without a CA refuses boot", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{TracerBaseURL: "tracer:4021", TracerTLSMode: "server"}

		reserver, err := buildTracerReserver(context.Background(), cfg, newBootstrapTestLogger(t), tracerSeamDeps{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TRACER_TLS_CA_FILE")
		assert.Nil(t, reserver)
	})

	t.Run("single-tenant token identity warms its token at boot", func(t *testing.T) {
		t.Parallel()

		minter := &recordingMinter{}
		cfg := &Config{AuthEnabled: true, TracerM2MClientID: "ledger", TracerM2MClientSecret: "s3cret"}

		_, _, err := buildTracerSeamIdentity(context.Background(), cfg, newBootstrapTestLogger(t),
			tracerSeamDeps{authClient: authorizingClient(), minter: minter})
		require.NoError(t, err)

		require.Eventually(t, func() bool { return minter.mints.Load() == 1 }, 5*time.Second, time.Millisecond)
	})
}

// seamWire is a bufconn tracer that records the metadata of every call it
// receives.
type seamWire struct {
	mu  sync.Mutex
	mds []metadata.MD
}

func (w *seamWire) intercept(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)

	w.mu.Lock()
	w.mds = append(w.mds, md.Copy())
	w.mu.Unlock()

	return handler(ctx, req)
}

func (w *seamWire) last(t *testing.T) metadata.MD {
	t.Helper()

	w.mu.Lock()
	defer w.mu.Unlock()

	require.NotEmpty(t, w.mds, "the call reached the tracer")

	return w.mds[len(w.mds)-1]
}

// dialSeamWire builds the production client with identityOpts against a bufconn
// tracer that records what arrives on the wire.
func dialSeamWire(t *testing.T, identityOpts []tracerclient.TracerGRPCClientOption) (*tracerclient.TracerGRPCClient, *seamWire) {
	t.Helper()

	wire := &seamWire{}
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer(grpc.UnaryInterceptor(wire.intercept))
	reservationv1.RegisterReservationServiceServer(srv, reservationv1.UnimplementedReservationServiceServer{})

	go func() { _ = srv.Serve(lis) }()

	opts := append([]tracerclient.TracerGRPCClientOption{
		tracerclient.WithGRPCOperationTimeout(5 * time.Second),
		tracerclient.WithGRPCDialOptions(
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	}, identityOpts...)

	client, err := tracerclient.NewTracerGRPCClient("passthrough:///bufnet", opts...)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = client.Close()
		srv.Stop()
		_ = lis.Close()
	})

	return client, wire
}

// clientIDMinter is a TokenMinter that never reaches the network and records the
// client id of every mint.
type clientIDMinter struct {
	mu        sync.Mutex
	clientIDs []string
}

func (m *clientIDMinter) GetApplicationToken(_ context.Context, clientID, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clientIDs = append(m.clientIDs, clientID)

	return "minted-token", nil
}

func (m *clientIDMinter) minted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]string(nil), m.clientIDs...)
}

// secretIDRecorder is a secret store client that records the secret id of every
// read and answers with one M2M credential document.
type secretIDRecorder struct {
	mu  sync.Mutex
	ids []string
}

func (r *secretIDRecorder) GetSecretValue(_ context.Context, in *awssm.GetSecretValueInput, _ ...func(*awssm.Options)) (*awssm.GetSecretValueOutput, error) {
	r.mu.Lock()
	r.ids = append(r.ids, aws.ToString(in.SecretId))
	r.mu.Unlock()

	return &awssm.GetSecretValueOutput{SecretString: aws.String(`{"clientId":"ledger-tenant","clientSecret":"s3cret"}`)}, nil
}

func (r *secretIDRecorder) read() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.ids...)
}

// seamTenantID is the tenant the multi-tenant wire tests call as, and
// seamTenantSecretPath the secret the tenant-manager writes its ledger-to-tracer
// credential under in the staging environment.
const (
	seamTenantID         = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	seamTenantSecretPath = "tenants/staging/0198a1b2c3d47e5f8a9b0c1d2e3f4a5b/ledger/m2m/tracer/credentials"
)

func TestBuildTracerSeamIdentity_Wire(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		cfg           Config
		deps          func(minter *clientIDMinter, secrets *secretIDRecorder) tracerSeamDeps
		ctx           context.Context
		wantBearer    []string
		wantAPIKey    []string
		wantSecretIDs []string
		wantClientIDs []string
	}{
		{
			name: "no identity sends no credential",
			cfg:  Config{},
			ctx:  context.Background(),
		},
		{
			name:       "api key identity sends x-api-key",
			cfg:        Config{TracerAPIKey: "k3y"},
			ctx:        context.Background(),
			wantAPIKey: []string{"k3y"},
		},
		{
			name: "static token identity sends a bearer token",
			cfg:  Config{AuthEnabled: true, TracerM2MClientID: "ledger", TracerM2MClientSecret: "s3cret"},
			deps: func(minter *clientIDMinter, _ *secretIDRecorder) tracerSeamDeps {
				return tracerSeamDeps{authClient: authorizingClient(), minter: minter}
			},
			ctx:           context.Background(),
			wantBearer:    []string{"Bearer minted-token"},
			wantClientIDs: []string{"ledger"},
		},
		{
			name: "tenant token identity sends the token minted from the tenant's own secret",
			cfg:  Config{AuthEnabled: true, MultiTenantEnabled: true, EnvName: "staging"},
			deps: func(minter *clientIDMinter, secrets *secretIDRecorder) tracerSeamDeps {
				return tracerSeamDeps{
					authClient:        authorizingClient(),
					minter:            minter,
					tenantServiceName: "ledger",
					newAWSSecretsClient: func(context.Context) (secretsmanager.SecretsManagerClient, error) {
						return secrets, nil
					},
				}
			},
			ctx:           tmcore.ContextWithTenantID(context.Background(), seamTenantID),
			wantBearer:    []string{"Bearer minted-token"},
			wantSecretIDs: []string{seamTenantSecretPath},
			wantClientIDs: []string{"ledger-tenant"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := tt.cfg
			minter := &clientIDMinter{}
			secrets := &secretIDRecorder{}

			var deps tracerSeamDeps
			if tt.deps != nil {
				deps = tt.deps(minter, secrets)
			}

			_, identityOpts, err := buildTracerSeamIdentity(context.Background(), &cfg, newBootstrapTestLogger(t), deps)
			require.NoError(t, err)

			client, wire := dialSeamWire(t, identityOpts)

			_ = client.ReleaseByTransaction(tt.ctx, [16]byte{1})

			md := wire.last(t)
			assert.Equal(t, tt.wantBearer, md.Get("authorization"))
			assert.Equal(t, tt.wantAPIKey, md.Get("x-api-key"))
			assert.Equal(t, tt.wantSecretIDs, secrets.read(), "the credential is read from the tenant-manager's path")
			assert.Equal(t, tt.wantClientIDs, minter.minted(), "the token is minted with the credential's client id")
		})
	}
}

// fakeVaultKV serves Vault KV v2 reads of one M2M credential document and
// records the request paths.
func fakeVaultKV(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()

	var (
		mu    sync.Mutex
		paths []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"clientId":"ledger-tenant","clientSecret":"s3cret"},` +
			`"metadata":{"created_time":"2026-10-01T12:00:00Z","deletion_time":"","destroyed":false,"version":1}}}`))
	}))
	t.Cleanup(srv.Close)

	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()

		return append([]string(nil), paths...)
	}
}

// TestBuildTracerSeamIdentity_VaultBackend is sequential: Vault's connection
// comes from the process environment.
func TestBuildTracerSeamIdentity_VaultBackend(t *testing.T) {
	cfg := Config{
		AuthEnabled:        true,
		MultiTenantEnabled: true,
		EnvName:            "staging",
		M2MSecretsBackend:  "vault",
		M2MVaultMount:      "ledger-m2m",
	}

	vaultDeps := func(minter *clientIDMinter, awsBuilt *atomic.Bool) tracerSeamDeps {
		return tracerSeamDeps{
			authClient:        authorizingClient(),
			minter:            minter,
			tenantServiceName: "ledger",
			newAWSSecretsClient: func(context.Context) (secretsmanager.SecretsManagerClient, error) {
				awsBuilt.Store(true)

				return fakeSecretsClient{}, nil
			},
		}
	}

	t.Run("the tenant's credential is read from Vault under the configured mount", func(t *testing.T) {
		srv, paths := fakeVaultKV(t)
		t.Setenv("VAULT_ADDR", srv.URL)
		t.Setenv("VAULT_TOKEN", "vault-test-token")

		minter := &clientIDMinter{}

		var awsBuilt atomic.Bool

		cfg := cfg

		identity, identityOpts, err := buildTracerSeamIdentity(context.Background(), &cfg, newBootstrapTestLogger(t), vaultDeps(minter, &awsBuilt))
		require.NoError(t, err)
		assert.Equal(t, seamIdentityTenantToken, identity)

		client, wire := dialSeamWire(t, identityOpts)

		_ = client.ReleaseByTransaction(tmcore.ContextWithTenantID(context.Background(), seamTenantID), [16]byte{1})

		assert.Equal(t, []string{"Bearer minted-token"}, wire.last(t).Get("authorization"))
		assert.Equal(t, []string{"/v1/ledger-m2m/data/" + seamTenantSecretPath}, paths())
		assert.Equal(t, []string{"ledger-tenant"}, minter.minted())
		assert.False(t, awsBuilt.Load(), "the Vault backend never builds an AWS client")
	})

	t.Run("a missing Vault connection refuses boot", func(t *testing.T) {
		srv, paths := fakeVaultKV(t)
		t.Setenv("VAULT_ADDR", srv.URL)
		t.Setenv("VAULT_TOKEN", "")

		var awsBuilt atomic.Bool

		cfg := cfg

		_, _, err := buildTracerSeamIdentity(context.Background(), &cfg, newBootstrapTestLogger(t), vaultDeps(&clientIDMinter{}, &awsBuilt))
		require.Error(t, err)
		require.ErrorIs(t, err, secretsmanager.ErrBackendMisconfigured)
		assert.Contains(t, err.Error(), "secret store reader")
		assert.False(t, awsBuilt.Load(), "a Vault that cannot be reached never falls back to AWS")
		assert.Empty(t, paths())
	})
}
