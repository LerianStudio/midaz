// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Struct(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Bucket:            "test-bucket",
		S3Endpoint:        "http://localhost:9000",
		S3Region:          "us-east-1",
		S3AccessKeyID:     "access-key",
		S3SecretAccessKey: "secret-key",
		S3UsePathStyle:    true,
		S3DisableSSL:      true,
	}

	assert.Equal(t, "test-bucket", cfg.Bucket)
	assert.Equal(t, "http://localhost:9000", cfg.S3Endpoint)
	assert.Equal(t, "us-east-1", cfg.S3Region)
	assert.Equal(t, "access-key", cfg.S3AccessKeyID)
	assert.Equal(t, "secret-key", cfg.S3SecretAccessKey)
	assert.True(t, cfg.S3UsePathStyle)
	assert.True(t, cfg.S3DisableSSL)
}

func TestNewStorageClient_MissingBucket(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Bucket: "",
	}

	_, err := NewStorageClient(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket name is required")
}

func TestNewStorageClient_ConfigMapping(t *testing.T) {
	t.Parallel()

	// This test verifies that the config is correctly mapped to S3Config
	// We can't fully test the client creation without actual S3 endpoint
	cfg := Config{
		Bucket:            "test-bucket",
		S3Endpoint:        "http://invalid-endpoint:9999",
		S3Region:          "us-west-2",
		S3AccessKeyID:     "test-key",
		S3SecretAccessKey: "test-secret",
		S3UsePathStyle:    true,
		S3DisableSSL:      true,
	}

	// This will fail at connection but validates config mapping
	_, err := NewStorageClient(context.Background(), cfg)
	// Error is expected since the endpoint is invalid
	// But we can verify the error is not about config mapping
	if err != nil {
		assert.NotContains(t, err.Error(), "bucket name is required")
	}
}

func TestConfig_EmptyFields(t *testing.T) {
	t.Parallel()

	cfg := Config{}

	assert.Empty(t, cfg.Bucket)
	assert.Empty(t, cfg.S3Endpoint)
	assert.Empty(t, cfg.S3Region)
	assert.Empty(t, cfg.S3AccessKeyID)
	assert.Empty(t, cfg.S3SecretAccessKey)
	assert.False(t, cfg.S3UsePathStyle)
	assert.False(t, cfg.S3DisableSSL)
}

func TestConfig_WithAllFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "MinIO configuration",
			config: Config{
				Bucket:            "reports",
				S3Endpoint:        "http://minio:9000",
				S3Region:          "us-east-1",
				S3AccessKeyID:     "minioadmin",
				S3SecretAccessKey: "minioadmin",
				S3UsePathStyle:    true,
				S3DisableSSL:      true,
			},
		},
		{
			name: "AWS S3 configuration",
			config: Config{
				Bucket:            "my-bucket",
				S3Endpoint:        "",
				S3Region:          "eu-west-1",
				S3AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				S3SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				S3UsePathStyle:    false,
				S3DisableSSL:      false,
			},
		},
		{
			name: "SeaweedFS S3 configuration",
			config: Config{
				Bucket:            "templates",
				S3Endpoint:        "http://seaweedfs:8333",
				S3Region:          "us-east-1",
				S3AccessKeyID:     "any",
				S3SecretAccessKey: "any",
				S3UsePathStyle:    true,
				S3DisableSSL:      true,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.NotEmpty(t, tt.config.Bucket)
			// Other validations are config-specific
		})
	}
}
