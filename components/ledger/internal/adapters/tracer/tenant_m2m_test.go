// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/aws/aws-sdk-go-v2/aws"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

const (
	testM2MEnv         = "staging"
	testM2MApplication = "ledger"
	testM2MTarget      = "tracer"
)

// fakeCustody is an in-memory custody backend keyed by secret path.
type fakeCustody struct {
	mu      sync.Mutex
	secrets map[string]string
	err     error
	reads   atomic.Int32
	paths   []string
	hold    chan struct{}
}

func newFakeCustody() *fakeCustody {
	return &fakeCustody{secrets: map[string]string{}}
}

func (f *fakeCustody) put(t *testing.T, tenantID, clientID, secret string) {
	t.Helper()

	payload, err := json.Marshal(map[string]string{"clientId": clientID, "clientSecret": secret})
	require.NoError(t, err)

	f.mu.Lock()
	defer f.mu.Unlock()

	f.secrets[secretsmanager.BuildM2MSecretPath(testM2MEnv, tenantID, testM2MApplication, testM2MTarget)] = string(payload)
}

func (f *fakeCustody) readPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.paths)
}

func (f *fakeCustody) failWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.err = err
}

func (f *fakeCustody) GetSecretValue(_ context.Context, params *awssm.GetSecretValueInput, _ ...func(*awssm.Options)) (*awssm.GetSecretValueOutput, error) {
	f.reads.Add(1)

	if f.hold != nil {
		<-f.hold
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	path := aws.ToString(params.SecretId)
	f.paths = append(f.paths, path)

	if f.err != nil {
		return nil, f.err
	}

	value, ok := f.secrets[path]
	if !ok {
		return nil, secretsmanager.ErrBackendSecretNotFound
	}

	return &awssm.GetSecretValueOutput{SecretString: aws.String(value)}, nil
}

func tenantCtx(t *testing.T, tenantID string) context.Context {
	t.Helper()

	return tmcore.ContextWithTenantID(t.Context(), tenantID)
}

func newTestCredentialProvider(t *testing.T, custody *fakeCustody, clk *testClock) *M2MCredentialProvider {
	t.Helper()

	provider, err := NewM2MCredentialProvider(custody, testM2MEnv, testM2MApplication, testM2MTarget, clk.Now)
	require.NoError(t, err)

	return provider
}

func TestM2MCredentialProvider_ReadsTenantPathAndCaches(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	custody.put(t, "tenant-b", "client-b", "secret-b")
	provider := newTestCredentialProvider(t, custody, clk)

	credsA, err := provider.GetCredentials(tenantCtx(t, "tenant-a"))
	require.NoError(t, err)
	assert.Equal(t, "client-a", credsA.ClientID)

	credsB, err := provider.GetCredentials(tenantCtx(t, "tenant-b"))
	require.NoError(t, err)
	assert.Equal(t, "client-b", credsB.ClientID)

	_, err = provider.GetCredentials(tenantCtx(t, "tenant-a"))
	require.NoError(t, err)
	assert.Equal(t, int32(2), custody.reads.Load(), "a cached tenant is not read again")
	assert.ElementsMatch(t, []string{
		"tenants/staging/tenant-a/ledger/m2m/tracer/credentials",
		"tenants/staging/tenant-b/ledger/m2m/tracer/credentials",
	}, custody.readPaths())

	clk.Advance(credentialCacheTTL)

	_, err = provider.GetCredentials(tenantCtx(t, "tenant-a"))
	require.NoError(t, err)
	assert.Equal(t, int32(3), custody.reads.Load(), "an expired entry is read again")
}

func TestM2MCredentialProvider_RequiresTenant(t *testing.T) {
	t.Parallel()

	custody := newFakeCustody()
	provider := newTestCredentialProvider(t, custody, &testClock{now: tokenSourceEpoch})

	_, err := provider.GetCredentials(t.Context())
	require.ErrorIs(t, err, ErrTenantRequired)
	assert.Zero(t, custody.reads.Load())
}

func TestM2MCredentialProvider_InvalidateRereads(t *testing.T) {
	t.Parallel()

	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	provider := newTestCredentialProvider(t, custody, &testClock{now: tokenSourceEpoch})
	ctx := tenantCtx(t, "tenant-a")

	_, err := provider.GetCredentials(ctx)
	require.NoError(t, err)

	custody.put(t, "tenant-a", "client-a2", "secret-a2")
	provider.InvalidateCredentials(ctx)

	creds, err := provider.GetCredentials(ctx)
	require.NoError(t, err)
	assert.Equal(t, "client-a2", creds.ClientID)
	assert.Equal(t, int32(2), custody.reads.Load())
}

func TestM2MCredentialProvider_ConcurrentMissesShareOneRead(t *testing.T) {
	t.Parallel()

	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	custody.hold = make(chan struct{})
	provider := newTestCredentialProvider(t, custody, &testClock{now: tokenSourceEpoch})

	const callers = 16

	var wg sync.WaitGroup

	errs := make(chan error, callers)

	for range callers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := provider.GetCredentials(tenantCtx(t, "tenant-a"))
			errs <- err
		}()
	}

	require.Eventually(t, func() bool { return custody.reads.Load() == 1 }, 5*time.Second, time.Millisecond)
	close(custody.hold)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), custody.reads.Load())
}

func TestM2MCredentialProvider_AbsentSecretIsRememberedBriefly(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	provider := newTestCredentialProvider(t, custody, clk)
	ctx := tenantCtx(t, "tenant-unprovisioned")

	for range 3 {
		_, err := provider.GetCredentials(ctx)
		require.ErrorIs(t, err, secretsmanager.ErrM2MCredentialsNotFound)
	}

	assert.Equal(t, int32(1), custody.reads.Load(), "an absence is served from memory within its window")

	clk.Advance(credentialAbsentTTL)
	custody.put(t, "tenant-unprovisioned", "client-late", "secret-late")

	creds, err := provider.GetCredentials(ctx)
	require.NoError(t, err)
	assert.Equal(t, "client-late", creds.ClientID)
}

func TestM2MCredentialProvider_RetrievalFailureIsNotCached(t *testing.T) {
	t.Parallel()

	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	custody.failWith(errors.New("connection reset"))
	provider := newTestCredentialProvider(t, custody, &testClock{now: tokenSourceEpoch})
	ctx := tenantCtx(t, "tenant-a")

	_, err := provider.GetCredentials(ctx)
	require.ErrorIs(t, err, secretsmanager.ErrM2MRetrievalFailed)

	custody.failWith(nil)

	_, err = provider.GetCredentials(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(2), custody.reads.Load())
}

func TestM2MCredentialProvider_DeniedReadIsRememberedBriefly(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	custody.failWith(secretsmanager.ErrBackendAccessDenied)
	provider := newTestCredentialProvider(t, custody, clk)
	ctx := tenantCtx(t, "tenant-a")

	for range 3 {
		_, err := provider.GetCredentials(ctx)
		require.ErrorIs(t, err, secretsmanager.ErrM2MVaultAccessDenied)
	}

	assert.Equal(t, int32(1), custody.reads.Load(), "a denied read is served from memory within its window")

	clk.Advance(credentialAbsentTTL)
	custody.failWith(nil)

	creds, err := provider.GetCredentials(ctx)
	require.NoError(t, err)
	assert.Equal(t, "client-a", creds.ClientID)
}

func TestNewM2MCredentialProvider_RejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	custody := newFakeCustody()

	for name, args := range map[string][3]string{
		"empty application": {testM2MEnv, "", testM2MTarget},
		"empty target":      {testM2MEnv, testM2MApplication, " "},
		"traversal target":  {testM2MEnv, testM2MApplication, "../tracer"},
		"separator env":     {"stag/ing", testM2MApplication, testM2MTarget},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := NewM2MCredentialProvider(custody, args[0], args[1], args[2], nil)
			require.Error(t, err)
		})
	}

	_, err := NewM2MCredentialProvider(nil, testM2MEnv, testM2MApplication, testM2MTarget, nil)
	require.Error(t, err)
}

// tenantMinter mints a token naming the client it was minted for, so a test
// can tell which tenant's credentials a token came from.
type tenantMinter struct {
	calls   atomic.Int32
	refuse  atomic.Bool
	perCall atomic.Int32

	mu      sync.Mutex
	refusal error
}

// refuseWith makes every mint fail with err; nil restores minting.
func (m *tenantMinter) refuseWith(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.refusal = err
}

func (m *tenantMinter) GetApplicationToken(_ context.Context, clientID, _ string) (string, error) {
	m.calls.Add(1)

	if m.refuse.Load() {
		return "", libCommons.Response{Code: "AUT-1004", Message: "invalid client credentials"}
	}

	m.mu.Lock()
	refusal := m.refusal
	m.mu.Unlock()

	if refusal != nil {
		return "", refusal
	}

	return fmt.Sprintf("token-%s-%d", clientID, m.perCall.Add(1)), nil
}

func newTestTenantTokenSource(t *testing.T, custody *fakeCustody, minter *tenantMinter, clk *testClock) (*TenantTokenSource, *countingProvider) {
	t.Helper()

	provider := &countingProvider{inner: newTestCredentialProvider(t, custody, clk)}

	source, err := NewTenantTokenSource(provider, minter, clk.Now)
	require.NoError(t, err)

	return source, provider
}

// countingProvider records credential invalidations.
type countingProvider struct {
	inner         CredentialProvider
	invalidations atomic.Int32
}

func (p *countingProvider) GetCredentials(ctx context.Context) (*secretsmanager.M2MCredentials, error) {
	return p.inner.GetCredentials(ctx)
}

func (p *countingProvider) InvalidateCredentials(ctx context.Context) {
	p.invalidations.Add(1)
	p.inner.InvalidateCredentials(ctx)
}

func TestTenantTokenSource_IsolatesTenants(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	custody.put(t, "tenant-b", "client-b", "secret-b")
	minter := &tenantMinter{}
	source, _ := newTestTenantTokenSource(t, custody, minter, clk)

	tokenA, err := source.Token(tenantCtx(t, "tenant-a"))
	require.NoError(t, err)
	tokenB, err := source.Token(tenantCtx(t, "tenant-b"))
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(tokenA, "token-client-a-"))
	assert.True(t, strings.HasPrefix(tokenB, "token-client-b-"))

	again, err := source.Token(tenantCtx(t, "tenant-a"))
	require.NoError(t, err)
	assert.Equal(t, tokenA, again, "a tenant's token is cached by its own source")
	assert.Equal(t, int32(2), minter.calls.Load())
}

func TestTenantTokenSource_MissingTenantIsRefusedBeforeSending(t *testing.T) {
	t.Parallel()

	custody := newFakeCustody()
	minter := &tenantMinter{}
	source, _ := newTestTenantTokenSource(t, custody, minter, &testClock{now: tokenSourceEpoch})

	_, err := source.Token(t.Context())
	require.ErrorIs(t, err, ErrTracerRequestRejected)
	require.ErrorIs(t, err, constant.ErrReservationTenantRequired)
	assert.Zero(t, custody.reads.Load())
	assert.Zero(t, minter.calls.Load())
	assert.False(t, source.Invalidate(t.Context(), "anything"))
}

func TestTenantTokenSource_UnprovisionedTenantIsRefused(t *testing.T) {
	t.Parallel()

	custody := newFakeCustody()
	minter := &tenantMinter{}
	source, _ := newTestTenantTokenSource(t, custody, minter, &testClock{now: tokenSourceEpoch})

	_, err := source.Token(tenantCtx(t, "tenant-unprovisioned"))
	require.ErrorIs(t, err, ErrTracerRequestRejected)
	require.ErrorIs(t, err, ErrTenantIdentityUnprovisioned)
	require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
	require.ErrorIs(t, err, secretsmanager.ErrM2MCredentialsNotFound)
	assert.Zero(t, minter.calls.Load(), "no static credential is ever tried")
}

func TestTenantTokenSource_PermanentCustodyAnswersAreRefusals(t *testing.T) {
	t.Parallel()

	for name, scenario := range map[string]struct {
		tenantID string
		secret   string
		fail     error
		sentinel error
	}{
		"access denied":       {fail: secretsmanager.ErrBackendAccessDenied, sentinel: secretsmanager.ErrM2MVaultAccessDenied},
		"not json":            {secret: "not-json", sentinel: secretsmanager.ErrM2MUnmarshalFailed},
		"missing secret":      {secret: `{"clientId":"client-a"}`, sentinel: secretsmanager.ErrM2MInvalidCredentials},
		"blank client id":     {secret: `{"clientId":" ","clientSecret":"secret-a"}`, sentinel: secretsmanager.ErrM2MInvalidCredentials},
		"traversal in tenant": {tenantID: "../tenant-a", sentinel: secretsmanager.ErrM2MInvalidPathSegment},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tenantID := scenario.tenantID
			if tenantID == "" {
				tenantID = "tenant-a"
			}

			custody := newFakeCustody()
			custody.failWith(scenario.fail)

			if scenario.secret != "" {
				custody.secrets[secretsmanager.BuildM2MSecretPath(testM2MEnv, tenantID, testM2MApplication, testM2MTarget)] = scenario.secret
			}

			minter := &tenantMinter{}
			source, _ := newTestTenantTokenSource(t, custody, minter, &testClock{now: tokenSourceEpoch})

			_, err := source.Token(tenantCtx(t, tenantID))
			require.ErrorIs(t, err, ErrTracerRequestRejected)
			require.ErrorIs(t, err, ErrTenantIdentityUnprovisioned)
			require.ErrorIs(t, err, scenario.sentinel)
			assert.Zero(t, minter.calls.Load())
		})
	}
}

func TestTenantTokenSource_TransientCustodyFailureWithoutSourceIsUnavailability(t *testing.T) {
	t.Parallel()

	for name, fail := range map[string]error{
		"connection reset":  errors.New("connection reset"),
		"deadline exceeded": context.DeadlineExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			custody := newFakeCustody()
			custody.put(t, "tenant-a", "client-a", "secret-a")
			custody.failWith(fail)
			source, _ := newTestTenantTokenSource(t, custody, &tenantMinter{}, &testClock{now: tokenSourceEpoch})

			_, err := source.Token(tenantCtx(t, "tenant-a"))
			require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
			require.ErrorIs(t, err, secretsmanager.ErrM2MRetrievalFailed)
			require.NotErrorIs(t, err, ErrTracerRequestRejected)
			require.NotErrorIs(t, err, ErrTenantIdentityUnprovisioned)
		})
	}
}

func TestTenantTokenSource_CancelledCustodyReadIsUnavailability(t *testing.T) {
	t.Parallel()

	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	custody.hold = make(chan struct{})
	t.Cleanup(func() { close(custody.hold) })

	source, _ := newTestTenantTokenSource(t, custody, &tenantMinter{}, &testClock{now: tokenSourceEpoch})

	ctx, cancel := context.WithCancel(tenantCtx(t, "tenant-a"))
	cancel()

	_, err := source.Token(ctx)
	require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, ErrTracerRequestRejected)
}

func TestTenantTokenSource_DeniedCustodyDropsTheTenant(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	source, _ := newTestTenantTokenSource(t, custody, &tenantMinter{}, clk)
	ctx := tenantCtx(t, "tenant-a")

	_, err := source.Token(ctx)
	require.NoError(t, err)

	clk.Advance(credentialCacheTTL)
	custody.failWith(secretsmanager.ErrBackendAccessDenied)

	_, err = source.Token(ctx)
	require.ErrorIs(t, err, ErrTracerRequestRejected)
	require.ErrorIs(t, err, ErrTenantIdentityUnprovisioned)
	require.ErrorIs(t, err, secretsmanager.ErrM2MVaultAccessDenied)
	assert.Empty(t, source.tenants, "denied credentials leave no source behind")
}

func TestTenantTokenSource_TransientCustodyFailureKeepsTheTenantsSource(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	minter := &tenantMinter{}
	source, _ := newTestTenantTokenSource(t, custody, minter, clk)
	ctx := tenantCtx(t, "tenant-a")

	first, err := source.Token(ctx)
	require.NoError(t, err)

	clk.Advance(credentialCacheTTL)
	custody.failWith(errors.New("connection reset"))

	again, err := source.Token(ctx)
	require.NoError(t, err)
	assert.Equal(t, first, again)
	assert.Equal(t, int32(1), minter.calls.Load())
}

func TestTenantTokenSource_RotatedCredentialsReplaceTheSource(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	source, _ := newTestTenantTokenSource(t, custody, &tenantMinter{}, clk)
	ctx := tenantCtx(t, "tenant-a")

	_, err := source.Token(ctx)
	require.NoError(t, err)

	custody.put(t, "tenant-a", "client-a-rotated", "secret-a-rotated")
	clk.Advance(credentialCacheTTL)

	token, err := source.Token(ctx)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(token, "token-client-a-rotated-"))
}

func TestTenantTokenSource_MintRefusalRereadsCredentialsOncePerWindow(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	minter := &tenantMinter{}
	minter.refuse.Store(true)
	source, provider := newTestTenantTokenSource(t, custody, minter, clk)
	ctx := tenantCtx(t, "tenant-a")

	_, err := source.Token(ctx)
	require.ErrorIs(t, err, ErrTracerRequestRejected)
	require.ErrorIs(t, err, ErrTenantIdentityUnprovisioned)
	assert.Zero(t, provider.invalidations.Load(), "a source younger than the backoff is kept")

	_, err = source.Token(ctx)
	require.ErrorIs(t, err, ErrTracerRequestRejected, "the refusal holds through the mint pause")
	require.ErrorIs(t, err, ErrTenantIdentityUnprovisioned)
	assert.Equal(t, int32(1), minter.calls.Load(), "the mint pause holds within the window")

	clk.Advance(tokenRenewBackoff)

	_, err = source.Token(ctx)
	require.ErrorIs(t, err, ErrTracerRequestRejected)
	assert.Equal(t, int32(1), provider.invalidations.Load(), "a refused mint re-reads the tenant's credentials")
	assert.Empty(t, source.tenants)
}

func TestTenantTokenSource_UnavailableMintKeepsTheSource(t *testing.T) {
	t.Parallel()

	for name, failure := range map[string]error{
		"5xx with plugin-auth code": libCommons.Response{Code: "AUT-0005", Message: "internal server error"},
		"numeric 500":               libCommons.Response{Code: "500", Message: "Internal Server Error"},
		"numeric 429":               libCommons.Response{Code: "429", Message: "Too Many Requests"},
		"no code":                   libCommons.Response{Message: "Bad Gateway"},
		"transport":                 errors.New("failed to make request: connection refused"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			clk := &testClock{now: tokenSourceEpoch}
			custody := newFakeCustody()
			custody.put(t, "tenant-a", "client-a", "secret-a")
			minter := &tenantMinter{}
			minter.refuseWith(failure)
			source, provider := newTestTenantTokenSource(t, custody, minter, clk)
			ctx := tenantCtx(t, "tenant-a")

			for range 2 {
				_, err := source.Token(ctx)
				require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
				require.NotErrorIs(t, err, ErrTracerRequestRejected)
				require.NotErrorIs(t, err, ErrTenantIdentityUnprovisioned)

				clk.Advance(tokenRenewBackoff)
			}

			assert.Zero(t, provider.invalidations.Load(), "an unavailable plugin-auth never invalidates the credentials")
			assert.Contains(t, source.tenants, "tenant-a", "an unavailable plugin-auth never drops the source")
			assert.Equal(t, int32(1), custody.reads.Load())
		})
	}
}

func TestIsMintRefusal(t *testing.T) {
	t.Parallel()

	for name, scenario := range map[string]struct {
		err  error
		want bool
	}{
		"invalid client credentials": {err: libCommons.Response{Code: "AUT-1004"}, want: true},
		"malformed grant":            {err: libCommons.Response{Code: "AUT-0014"}, want: true},
		"numeric 400":                {err: libCommons.Response{Code: "400"}, want: true},
		"numeric 401":                {err: libCommons.Response{Code: "401"}, want: true},
		"numeric 403":                {err: libCommons.Response{Code: "403"}, want: true},
		"wrapped":                    {err: fmt.Errorf("%w: %w", constant.ErrTracerTokenUnavailable, libCommons.Response{Code: "AUT-1004"}), want: true},
		"internal error code":        {err: libCommons.Response{Code: "AUT-0005"}},
		"numeric 500":                {err: libCommons.Response{Code: "500"}},
		"numeric 503":                {err: libCommons.Response{Code: "503"}},
		"numeric 429":                {err: libCommons.Response{Code: "429"}},
		"numeric 404":                {err: libCommons.Response{Code: "404"}},
		"no code":                    {err: libCommons.Response{Message: "Unauthorized"}},
		"not a response":             {err: errors.New("dial tcp: connection refused")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, scenario.want, isMintRefusal(scenario.err))
		})
	}
}

func TestTenantTokenSource_KeepsNoPlaintextSecret(t *testing.T) {
	t.Parallel()

	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	source, _ := newTestTenantTokenSource(t, custody, &tenantMinter{}, &testClock{now: tokenSourceEpoch})

	_, err := source.Token(tenantCtx(t, "tenant-a"))
	require.NoError(t, err)

	entry := source.tenants["tenant-a"]
	require.NotNil(t, entry)
	assert.Equal(t, sha256.Sum256([]byte("secret-a")), entry.secretDigest)
	assert.NotContains(t, fmt.Sprintf("%+v", *entry), "secret-a")
}

func TestTenantTokenSource_InvalidationIsIsolatedPerTenant(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	custody.put(t, "tenant-b", "client-b", "secret-b")
	minter := &tenantMinter{}
	source, provider := newTestTenantTokenSource(t, custody, minter, clk)
	ctxA, ctxB := tenantCtx(t, "tenant-a"), tenantCtx(t, "tenant-b")

	tokenA, err := source.Token(ctxA)
	require.NoError(t, err)
	tokenB, err := source.Token(ctxB)
	require.NoError(t, err)

	readsBefore, mintsBefore := custody.reads.Load(), minter.calls.Load()

	clk.Advance(tokenRenewBackoff)
	require.True(t, source.Invalidate(ctxA, tokenA))
	source.RejectCredentials(ctxA, tokenA)

	againB, err := source.Token(ctxB)
	require.NoError(t, err)
	assert.Equal(t, tokenB, againB, "tenant B keeps its token when tenant A's is rejected")
	assert.Equal(t, readsBefore, custody.reads.Load(), "tenant B's credentials are not read again")
	assert.Equal(t, mintsBefore, minter.calls.Load(), "tenant B is not minted again")
	assert.Equal(t, int32(1), provider.invalidations.Load(), "only tenant A's credentials are invalidated")

	renewedA, err := source.Token(ctxA)
	require.NoError(t, err)
	assert.NotEqual(t, tokenA, renewedA)
	assert.True(t, strings.HasPrefix(renewedA, "token-client-a-"))
}

func TestTenantTokenSource_ConcurrentTenantsBeyondTheLimitGetTheirOwnTokens(t *testing.T) {
	t.Parallel()

	const (
		workers = 8
		tenants = tenantTokenSourceLimit + 64
	)

	custody := newFakeCustody()

	ids := make([]string, tenants)
	for i := range ids {
		ids[i] = fmt.Sprintf("tenant-%05d", i)
		custody.put(t, ids[i], "client-"+ids[i], "secret-"+ids[i])
	}

	source, _ := newTestTenantTokenSource(t, custody, &tenantMinter{}, &testClock{now: tokenSourceEpoch})

	var (
		wg         sync.WaitGroup
		mismatches atomic.Int32
		failures   atomic.Int32
	)

	for worker := range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := range tenants {
				tenantID := ids[(i*7+worker*131)%tenants]

				token, err := source.Token(tmcore.ContextWithTenantID(t.Context(), tenantID))
				if err != nil {
					failures.Add(1)

					continue
				}

				if !strings.HasPrefix(token, "token-client-"+tenantID+"-") {
					mismatches.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	assert.Zero(t, failures.Load())
	assert.Zero(t, mismatches.Load(), "a tenant only ever receives a token minted from its own credentials")

	source.mu.Lock()
	defer source.mu.Unlock()

	assert.LessOrEqual(t, len(source.tenants), tenantTokenSourceLimit)
}

func TestTenantTokenSource_EvictsIdleAndLeastRecentlyUsed(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	source, _ := newTestTenantTokenSource(t, custody, &tenantMinter{}, clk)

	for i := range tenantTokenSourceLimit {
		tenantID := fmt.Sprintf("tenant-%04d", i)
		custody.put(t, tenantID, "client-"+tenantID, "secret")

		_, err := source.Token(tenantCtx(t, tenantID))
		require.NoError(t, err)
		clk.Advance(time.Millisecond)
	}

	require.Len(t, source.tenants, tenantTokenSourceLimit)

	custody.put(t, "tenant-extra", "client-extra", "secret")
	_, err := source.Token(tenantCtx(t, "tenant-extra"))
	require.NoError(t, err)
	require.Len(t, source.tenants, tenantTokenSourceLimit)
	assert.NotContains(t, source.tenants, "tenant-0000", "the least recently used tenant makes room")

	clk.Advance(tenantTokenSourceIdleTTL)
	custody.put(t, "tenant-late", "client-late", "secret")
	_, err = source.Token(tenantCtx(t, "tenant-late"))
	require.NoError(t, err)
	assert.Len(t, source.tenants, 1, "idle tenants are dropped")
}

// mtTracerServer answers confirm calls, refusing tokens not in accepted, and
// records the bearer and tenant of every request.
type mtTracerServer struct {
	mu       sync.Mutex
	accepted map[string]bool
	seen     []string
}

func (s *mtTracerServer) seenRequests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.seen)
}

func (s *mtTracerServer) accept(tokens map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.accepted = tokens
}

func (s *mtTracerServer) handler(transactionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get(AuthorizationHeader), bearerPrefix)

		s.mu.Lock()
		s.seen = append(s.seen, r.Header.Get(TenantHeader)+"|"+token)
		ok := s.accepted[token] || s.accepted["*"]
		s.mu.Unlock()

		if !ok {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}

		_, _ = fmt.Fprintf(w, `{"contractRevision":%q,"transactionId":%q,"status":"CONFIRMED","flipped":0}`, tracercontract.ReserveContractRevision, transactionID)
	}
}

func TestContextHTTPClient_MultiTenantSendsEachTenantItsOwnToken(t *testing.T) {
	t.Parallel()

	request, config := contextClientFixture(t)
	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	custody.put(t, "tenant-b", "client-b", "secret-b")
	source, _ := newTestTenantTokenSource(t, custody, &tenantMinter{}, clk)

	tracer := &mtTracerServer{accepted: map[string]bool{"*": true}}
	server := httptest.NewServer(tracer.handler(request.TransactionID.String()))
	t.Cleanup(server.Close)

	client, err := NewContextHTTPClient(server.URL, config, source)
	require.NoError(t, err)

	for _, tenantID := range []string{"tenant-a", "tenant-b", "tenant-a"} {
		_, err := client.ConfirmByTransaction(tenantCtx(t, tenantID), request.TransactionID)
		require.NoError(t, err)
	}

	seenRequests := tracer.seenRequests()
	require.Len(t, seenRequests, 3)

	for _, seen := range seenRequests {
		tenantID, token, _ := strings.Cut(seen, "|")
		client := strings.Replace(tenantID, "tenant-", "client-", 1)
		assert.True(t, strings.HasPrefix(token, "token-"+client+"-"), "tenant %s presented %s", tenantID, token)
	}
}

func TestContextHTTPClient_MultiTenantPersistentRejectionRereadsCredentials(t *testing.T) {
	t.Parallel()

	request, config := contextClientFixture(t)
	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	minter := &tenantMinter{}
	source, provider := newTestTenantTokenSource(t, custody, minter, clk)
	ctx := tenantCtx(t, "tenant-a")

	tracer := &mtTracerServer{accepted: map[string]bool{"*": true}}
	server := httptest.NewServer(tracer.handler(request.TransactionID.String()))
	t.Cleanup(server.Close)

	client, err := NewContextHTTPClient(server.URL, config, source)
	require.NoError(t, err)

	_, err = client.ConfirmByTransaction(ctx, request.TransactionID)
	require.NoError(t, err)

	tracer.accept(map[string]bool{})
	clk.Advance(tokenRenewBackoff)

	_, err = client.ConfirmByTransaction(ctx, request.TransactionID)
	require.ErrorIs(t, err, ErrTracerUnavailable)
	require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
	assert.Equal(t, int32(2), minter.calls.Load(), "the rejected token was renewed once")
	assert.Equal(t, int32(1), provider.invalidations.Load(), "a renewed token still refused re-reads the credentials")
	assert.Len(t, tracer.seenRequests(), 3)

	tracer.accept(map[string]bool{"*": true})

	_, err = client.ConfirmByTransaction(ctx, request.TransactionID)
	require.NoError(t, err)
	assert.Equal(t, int32(3), minter.calls.Load(), "the tenant's next call mints from freshly read credentials")
	assert.Equal(t, int32(2), custody.reads.Load())
}

func TestContextHTTPClient_MultiTenantIdentityFailures(t *testing.T) {
	t.Parallel()

	request, config := contextClientFixture(t)

	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	newClient := func(t *testing.T, custody *fakeCustody, minter *tenantMinter) *ContextHTTPClient {
		t.Helper()

		source, _ := newTestTenantTokenSource(t, custody, minter, &testClock{now: tokenSourceEpoch})

		client, err := NewContextHTTPClient(server.URL, config, source)
		require.NoError(t, err)

		return client
	}

	t.Run("unprovisioned tenant is refused", func(t *testing.T) {
		client := newClient(t, newFakeCustody(), &tenantMinter{})

		_, err := client.ConfirmByTransaction(tenantCtx(t, "tenant-unprovisioned"), request.TransactionID)
		require.ErrorIs(t, err, ErrTracerRequestRejected)
		require.ErrorIs(t, err, ErrTenantIdentityUnprovisioned)
		require.ErrorIs(t, err, secretsmanager.ErrM2MCredentialsNotFound)
		require.NotErrorIs(t, err, ErrTracerUnavailable, "an unprovisioned tenant is deterministic, not an outage")
	})

	t.Run("refused mint is refused", func(t *testing.T) {
		custody := newFakeCustody()
		custody.put(t, "tenant-a", "client-a", "secret-a")
		minter := &tenantMinter{}
		minter.refuse.Store(true)
		client := newClient(t, custody, minter)

		_, err := client.ConfirmByTransaction(tenantCtx(t, "tenant-a"), request.TransactionID)
		require.ErrorIs(t, err, ErrTracerRequestRejected)
		require.ErrorIs(t, err, ErrTenantIdentityUnprovisioned)
		require.NotErrorIs(t, err, ErrTracerUnavailable)
	})

	t.Run("retrieval failure is unavailability", func(t *testing.T) {
		custody := newFakeCustody()
		custody.failWith(errors.New("connection reset"))
		client := newClient(t, custody, &tenantMinter{})

		_, err := client.ConfirmByTransaction(tenantCtx(t, "tenant-a"), request.TransactionID)
		require.ErrorIs(t, err, ErrTracerUnavailable)
		require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		require.ErrorIs(t, err, secretsmanager.ErrM2MRetrievalFailed)
		require.NotErrorIs(t, err, ErrTracerRequestRejected)
	})

	t.Run("missing tenant is refused", func(t *testing.T) {
		client := newClient(t, newFakeCustody(), &tenantMinter{})

		_, err := client.ConfirmByTransaction(t.Context(), request.TransactionID)
		require.ErrorIs(t, err, ErrTracerRequestRejected)
		require.ErrorIs(t, err, constant.ErrReservationTenantRequired)
		require.NotErrorIs(t, err, ErrTracerUnavailable, "a missing tenant is deterministic, not an outage")
	})

	assert.Zero(t, calls.Load(), "nothing reaches the Tracer without a tenant token")
}

func TestContextHTTPClient_MultiTenantRenewalRefusalIsNotAnOutage(t *testing.T) {
	t.Parallel()

	request, config := contextClientFixture(t)
	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, "tenant-a", "client-a", "secret-a")
	minter := &tenantMinter{}
	source, _ := newTestTenantTokenSource(t, custody, minter, clk)
	ctx := tenantCtx(t, "tenant-a")

	tracer := &mtTracerServer{accepted: map[string]bool{"*": true}}
	server := httptest.NewServer(tracer.handler(request.TransactionID.String()))
	t.Cleanup(server.Close)

	client, err := NewContextHTTPClient(server.URL, config, source)
	require.NoError(t, err)

	_, err = client.ConfirmByTransaction(ctx, request.TransactionID)
	require.NoError(t, err)

	tracer.accept(map[string]bool{})
	minter.refuse.Store(true)
	clk.Advance(tokenRenewBackoff)

	_, err = client.ConfirmByTransaction(ctx, request.TransactionID)
	require.ErrorIs(t, err, ErrTracerRequestRejected, "plugin-auth refusing the renewal is the answer")
	require.ErrorIs(t, err, ErrTenantIdentityUnprovisioned)
	require.NotErrorIs(t, err, ErrTracerUnavailable)
}

func TestM2MCredentialProvider_ReadsTheCanonicalTenantPath(t *testing.T) {
	t.Parallel()

	const dashless = "0d1a2b3c4d5e6f708192a3b4c5d6e7f8"

	clk := &testClock{now: tokenSourceEpoch}
	custody := newFakeCustody()
	custody.put(t, dashless, "client-a", "secret-a")
	provider := newTestCredentialProvider(t, custody, clk)

	creds, err := provider.GetCredentials(tenantCtx(t, "0d1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8"))
	require.NoError(t, err)
	assert.Equal(t, "client-a", creds.ClientID)

	_, err = provider.GetCredentials(tenantCtx(t, dashless))
	require.NoError(t, err)
	assert.Equal(t, int32(1), custody.reads.Load(), "both spellings share one cache entry")
	assert.Equal(t, []string{"tenants/staging/" + dashless + "/ledger/m2m/tracer/credentials"}, custody.readPaths())
}
