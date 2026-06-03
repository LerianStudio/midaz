// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libSM "github.com/LerianStudio/lib-commons/v5/commons/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSMClient implements libSM.SecretsManagerClient for testing.
type mockSMClient struct {
	output *secretsmanager.GetSecretValueOutput
	err    error
}

func (m *mockSMClient) GetSecretValue(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	return m.output, m.err
}

func validCredentialJSON(clientID, clientSecret string) *string {
	data, _ := json.Marshal(libSM.M2MCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	})
	s := string(data)

	return &s
}

func TestSMCredentialFetcher_FetchCredential_Success(t *testing.T) {
	t.Parallel()

	client := &mockSMClient{
		output: &secretsmanager.GetSecretValueOutput{
			SecretString: validCredentialJSON("client-abc", "secret-xyz"),
		},
	}

	fetcher := newSMCredentialFetcherWithClient(client, "staging", "reporter")

	cred, err := fetcher.FetchCredential(context.Background(), "tenant-123", "fetcher")
	require.NoError(t, err)
	assert.Equal(t, "client-abc", cred.ClientID)
	assert.Equal(t, "secret-xyz", cred.ClientSecret)
}

func TestSMCredentialFetcher_FetchCredential_AWSError(t *testing.T) {
	t.Parallel()

	client := &mockSMClient{
		err: errors.New("ResourceNotFoundException: secret not found"),
	}

	fetcher := newSMCredentialFetcherWithClient(client, "staging", "reporter")

	cred, err := fetcher.FetchCredential(context.Background(), "tenant-404", "fetcher")
	assert.Error(t, err)
	assert.Nil(t, cred)
	assert.Contains(t, err.Error(), "tenant-404")
}

func TestSMCredentialFetcher_FetchCredential_EmptyTenantID(t *testing.T) {
	t.Parallel()

	client := &mockSMClient{
		err: libSM.ErrM2MInvalidInput,
	}

	fetcher := newSMCredentialFetcherWithClient(client, "staging", "reporter")

	cred, err := fetcher.FetchCredential(context.Background(), "", "fetcher")
	assert.Error(t, err)
	assert.Nil(t, cred)
}

func TestBuildCredentialFetcher_EmptyRegion_ReturnsNil(t *testing.T) {
	t.Parallel()

	fetcher, err := BuildCredentialFetcher(context.Background(), SMCredentialFetcherConfig{
		AWSRegion:       "",
		Environment:     "staging",
		ApplicationName: "reporter",
	})

	assert.NoError(t, err)
	assert.Nil(t, fetcher)
}

func TestSMCredentialFetcher_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ CredentialFetcher = (*SMCredentialFetcher)(nil)
}
