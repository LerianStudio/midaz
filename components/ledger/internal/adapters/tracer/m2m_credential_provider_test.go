// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/aws/aws-sdk-go-v2/aws"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticCredentials(t *testing.T) {
	t.Parallel()

	provider := NewStaticCredentials("ledger-client", "s3cret")

	creds, err := provider.Credentials(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, Credentials{ClientID: "ledger-client", ClientSecret: "s3cret"}, creds)

	provider.Invalidate("")

	again, err := provider.Credentials(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, creds, again)

	assert.NotContains(t, creds.String(), "s3cret")
	assert.NotContains(t, fmt.Sprintf("%v %+v %#v", creds, creds, creds), "s3cret")
}

// fetchRecorder is a CredentialFetcher that records the tenantOrgID of each read.
// With gate set every read signals entered (when set) and waits for gate.
type fetchRecorder struct {
	mu      sync.Mutex
	reads   []string
	err     error
	gate    chan struct{}
	entered chan struct{}
}

func (f *fetchRecorder) fetch(_ context.Context, tenantOrgID string) (*secretsmanager.M2MCredentials, error) {
	f.mu.Lock()
	f.reads = append(f.reads, tenantOrgID)
	err := f.err
	f.mu.Unlock()

	if f.entered != nil {
		f.entered <- struct{}{}
	}

	if f.gate != nil {
		<-f.gate
	}

	if err != nil {
		return nil, err
	}

	return &secretsmanager.M2MCredentials{ClientID: "id-" + tenantOrgID, ClientSecret: "secret-" + tenantOrgID}, nil
}

func (f *fetchRecorder) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.reads)
}

func TestNewTenantCredentials_RequiresAFetcher(t *testing.T) {
	t.Parallel()

	provider, err := NewTenantCredentials(nil)
	require.Error(t, err)
	assert.Nil(t, provider)
}

func newTestTenantCredentials(t *testing.T, fetch CredentialFetcher, clock *testClock) CredentialProvider {
	t.Helper()

	provider, err := NewTenantCredentials(fetch, WithCredentialClock(clock.Now))
	require.NoError(t, err)

	return provider
}

func TestTenantCredentials_CachedUntilInvalidated(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	recorder := &fetchRecorder{}
	provider := newTestTenantCredentials(t, recorder.fetch, clock)

	ctx := context.Background()

	creds, err := provider.Credentials(ctx, "tenant-a")
	require.NoError(t, err)
	assert.Equal(t, "id-tenant-a", creds.ClientID)

	clock.Advance(24 * time.Hour)

	_, err = provider.Credentials(ctx, "tenant-a")
	require.NoError(t, err)
	assert.Equal(t, 1, recorder.count(), "a credential lives as long as the tokens minted with it")

	provider.Invalidate("tenant-a")

	_, err = provider.Credentials(ctx, "tenant-a")
	require.NoError(t, err)
	assert.Equal(t, 2, recorder.count(), "an invalidated credential is read again")
}

func TestTenantCredentials_SingleFlight(t *testing.T) {
	t.Parallel()

	recorder := &fetchRecorder{gate: make(chan struct{}), entered: make(chan struct{}, 64)}
	provider := newTestTenantCredentials(t, recorder.fetch, newTestClock())

	const callers = 16

	start := make(chan struct{})
	errs := make([]error, callers)

	var wg sync.WaitGroup

	for i := range callers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			_, errs[i] = provider.Credentials(context.Background(), "tenant-a")
		}()
	}

	close(start)
	<-recorder.entered

	select {
	case <-recorder.entered:
		t.Fatal("a second fetch started while the first was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(recorder.gate)
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t, 1, recorder.count(), "concurrent reads of one tenant collapse into one fetch")
}

func TestTenantCredentials_CanonicalTenantID(t *testing.T) {
	t.Parallel()

	recorder := &fetchRecorder{}
	provider := newTestTenantCredentials(t, recorder.fetch, newTestClock())

	_, err := provider.Credentials(context.Background(), "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b")
	require.NoError(t, err)

	_, err = provider.Credentials(context.Background(), "0198a1b2c3d47e5f8a9b0c1d2e3f4a5b")
	require.NoError(t, err)

	_, err = provider.Credentials(context.Background(), "plain-tenant")
	require.NoError(t, err)

	assert.Equal(t, []string{"0198a1b2c3d47e5f8a9b0c1d2e3f4a5b", "plain-tenant"}, recorder.reads,
		"a UUID tenant id is read once, under the dashless id the tenant-manager writes")

	_, err = provider.Credentials(context.Background(), "tenant with spaces")
	require.ErrorIs(t, err, tmcore.ErrInvalidTenantIDFormat)
	assert.Len(t, recorder.reads, 2, "an invalid tenant id is never read")
}

func TestTenantCredentials_Failures(t *testing.T) {
	t.Parallel()

	t.Run("missing tenant in context", func(t *testing.T) {
		t.Parallel()

		recorder := &fetchRecorder{}
		provider := newTestTenantCredentials(t, recorder.fetch, newTestClock())

		_, err := provider.Credentials(context.Background(), "")
		require.Error(t, err)
		assert.Zero(t, recorder.count(), "no tenant means nothing to read")
	})

	permanent := []error{
		secretsmanager.ErrM2MInvalidCredentials,
		secretsmanager.ErrM2MUnmarshalFailed,
		secretsmanager.ErrM2MInvalidPathSegment,
	}

	for _, sentinel := range permanent {
		t.Run("a permanent failure is cached briefly: "+sentinel.Error(), func(t *testing.T) {
			t.Parallel()

			clock := newTestClock()
			recorder := &fetchRecorder{err: fmt.Errorf("%w: .../credentials", sentinel)}
			provider := newTestTenantCredentials(t, recorder.fetch, clock)

			_, err := provider.Credentials(context.Background(), "tenant-a")
			require.ErrorIs(t, err, sentinel)

			clock.Advance(permanentCredentialFailureTTL - time.Second)

			_, err = provider.Credentials(context.Background(), "tenant-a")
			require.ErrorIs(t, err, sentinel)
			assert.Equal(t, 1, recorder.count(), "a permanent failure is not re-read inside its window")

			_, err = provider.Credentials(context.Background(), "tenant-b")
			require.Error(t, err)
			assert.Equal(t, 2, recorder.count(), "the failure is cached per tenant")

			clock.Advance(time.Second)

			_, err = provider.Credentials(context.Background(), "tenant-a")
			require.ErrorIs(t, err, sentinel)
			assert.Equal(t, 3, recorder.count(), "the window closes and the credential is read again")
		})
	}

	transient := []error{
		secretsmanager.ErrM2MRetrievalFailed,
		secretsmanager.ErrM2MVaultAccessDenied,
		errors.New("connection reset"),
	}

	for _, cause := range transient {
		t.Run("a transient failure is never cached: "+cause.Error(), func(t *testing.T) {
			t.Parallel()

			recorder := &fetchRecorder{err: fmt.Errorf("%w: .../credentials", cause)}
			provider := newTestTenantCredentials(t, recorder.fetch, newTestClock())

			_, err := provider.Credentials(context.Background(), "tenant-a")
			require.ErrorIs(t, err, cause)

			_, err = provider.Credentials(context.Background(), "tenant-a")
			require.ErrorIs(t, err, cause)
			assert.Equal(t, 2, recorder.count())
		})
	}

	t.Run("a failed read logs nothing; the mint span carries it", func(t *testing.T) {
		t.Parallel()

		logger := &recordingLogger{}
		ctx := libObservability.ContextWithLogger(context.Background(), logger)

		recorder := &fetchRecorder{err: secretsmanager.ErrM2MCredentialsNotFound}
		provider := newTestTenantCredentials(t, recorder.fetch, newTestClock())

		_, err := provider.Credentials(ctx, "tenant-a")
		require.Error(t, err)

		for _, level := range []int{libLog.LevelError, libLog.LevelWarn, libLog.LevelInfo} {
			assert.Empty(t, logger.atLevel(level))
		}
	})
}

// pathRecordingSecrets is a secret store client that records the secret id it is
// asked for and answers with a fixed credential document.
type pathRecordingSecrets struct {
	mu  sync.Mutex
	ids []string
}

func (p *pathRecordingSecrets) GetSecretValue(_ context.Context, in *awssm.GetSecretValueInput, _ ...func(*awssm.Options)) (*awssm.GetSecretValueOutput, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ids = append(p.ids, aws.ToString(in.SecretId))

	return &awssm.GetSecretValueOutput{SecretString: aws.String(`{"clientId":"ledger-tenant","clientSecret":"s3cret"}`)}, nil
}

func TestSecretsManagerCredentialFetcher_ReadsTheTenantManagerPath(t *testing.T) {
	t.Parallel()

	client := &pathRecordingSecrets{}
	provider, err := NewTenantCredentials(NewSecretsManagerCredentialFetcher(client, "staging", "ledger"))
	require.NoError(t, err)

	creds, err := provider.Credentials(context.Background(), "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b")
	require.NoError(t, err)
	assert.Equal(t, "ledger-tenant", creds.ClientID)

	assert.Equal(t, []string{"tenants/staging/0198a1b2c3d47e5f8a9b0c1d2e3f4a5b/ledger/m2m/tracer/credentials"}, client.ids)
}

func TestTenantCredentials_MissingSecretIsCachedBriefly(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	recorder := &fetchRecorder{err: fmt.Errorf("%w: .../credentials", secretsmanager.ErrM2MCredentialsNotFound)}
	provider := newTestTenantCredentials(t, recorder.fetch, clock)

	_, err := provider.Credentials(context.Background(), "tenant-a")
	require.ErrorIs(t, err, secretsmanager.ErrM2MCredentialsNotFound)

	clock.Advance(missingCredentialFailureTTL - time.Millisecond)

	_, err = provider.Credentials(context.Background(), "tenant-a")
	require.ErrorIs(t, err, secretsmanager.ErrM2MCredentialsNotFound)
	assert.Equal(t, 1, recorder.count(), "a missing secret is answered from memory inside its window")

	clock.Advance(time.Millisecond)

	_, err = provider.Credentials(context.Background(), "tenant-a")
	require.ErrorIs(t, err, secretsmanager.ErrM2MCredentialsNotFound)
	assert.Equal(t, 2, recorder.count(), "a secret provisioned after the miss is read within seconds")
}

func TestTenantCredentials_PanicInReadIsRecovered(t *testing.T) {
	t.Parallel()

	provider := newTestTenantCredentials(t, func(context.Context, string) (*secretsmanager.M2MCredentials, error) {
		panic("secret store exploded")
	}, newTestClock())

	_, err := provider.Credentials(context.Background(), "tenant-a")
	require.Error(t, err)
	assert.NotErrorIs(t, err, errTokenMintPanicked, "a read panic is not a mint panic")
	assert.Contains(t, err.Error(), "credential read panicked")
}
