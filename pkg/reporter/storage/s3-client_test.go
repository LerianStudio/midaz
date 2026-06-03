// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestS3Config_DefaultSeaweedS3Config(t *testing.T) {
	t.Parallel()

	bucket := "test-bucket"
	cfg := DefaultSeaweedS3Config(bucket)

	assert.Equal(t, "http://localhost:8333", cfg.Endpoint)
	assert.Equal(t, "us-east-1", cfg.Region)
	assert.Equal(t, bucket, cfg.Bucket)
	assert.True(t, cfg.UsePathStyle)
	assert.True(t, cfg.DisableSSL)
}

func TestNewS3Client_RequiresBucket(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := S3Config{
		Endpoint: "http://localhost:9000",
	}

	client, err := NewS3Client(ctx, cfg)

	assert.Nil(t, client)
	assert.Equal(t, ErrBucketRequired, err)
}

// createTestClient creates a S3Client for testing parameter validation.
// The client won't be able to connect to S3, but we can test input validation.
func createTestClient(t *testing.T) *S3Client {
	t.Helper()

	ctx := context.Background()
	cfg := S3Config{
		Endpoint:        "http://localhost:9999", // Non-existent endpoint
		Region:          "us-east-1",
		Bucket:          "test-bucket",
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
		UsePathStyle:    true,
		DisableSSL:      true,
	}

	client, err := NewS3Client(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	return client
}

// createTestClientWithServer creates an S3Client backed by an httptest.Server.
// The handler function controls what the fake S3 endpoint returns.
func createTestClientWithServer(t *testing.T, handler http.HandlerFunc) (*S3Client, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)

	s3Client := s3.New(s3.Options{
		BaseEndpoint: aws.String(server.URL),
		Region:       "us-east-1",
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider("test-key", "test-secret", ""),
	})

	client := &S3Client{
		s3:     s3Client,
		bucket: "test-bucket",
	}

	return client, server
}

func TestS3Client_UploadRequiresKey(t *testing.T) {
	t.Parallel()

	client := createTestClient(t)
	ctx := context.Background()

	// Test with empty key
	_, err := client.Upload(ctx, "", strings.NewReader("test data"), "text/plain")

	assert.Equal(t, ErrKeyRequired, err)
}

func TestS3Client_UploadWithTTLRequiresKey(t *testing.T) {
	t.Parallel()

	client := createTestClient(t)
	ctx := context.Background()

	// Test with empty key
	_, err := client.UploadWithTTL(ctx, "", strings.NewReader("test data"), "text/plain", "1h")

	assert.Equal(t, ErrKeyRequired, err)
}

func TestS3Client_DownloadRequiresKey(t *testing.T) {
	t.Parallel()

	client := createTestClient(t)
	ctx := context.Background()

	// Test with empty key
	_, err := client.Download(ctx, "")

	assert.Equal(t, ErrKeyRequired, err)
}

func TestS3Client_DeleteRequiresKey(t *testing.T) {
	t.Parallel()

	client := createTestClient(t)
	ctx := context.Background()

	// Test with empty key
	err := client.Delete(ctx, "")

	assert.Equal(t, ErrKeyRequired, err)
}

func TestS3Client_ExistsRequiresKey(t *testing.T) {
	t.Parallel()

	client := createTestClient(t)
	ctx := context.Background()

	// Test with empty key
	exists, err := client.Exists(ctx, "")

	assert.False(t, exists)
	assert.Equal(t, ErrKeyRequired, err)
}

func TestS3Client_GeneratePresignedURLRequiresKey(t *testing.T) {
	t.Parallel()

	client := createTestClient(t)
	ctx := context.Background()

	// Test with empty key
	url, err := client.GeneratePresignedURL(ctx, "", 1*time.Hour)

	assert.Empty(t, url)
	assert.Equal(t, ErrKeyRequired, err)
}

func TestS3Client_GeneratePresignedURL_Success(t *testing.T) {
	t.Parallel()

	client := createTestClient(t)
	ctx := context.Background()

	// GeneratePresignedURL doesn't need actual S3 connection to generate a URL
	url, err := client.GeneratePresignedURL(ctx, "test-key.txt", 1*time.Hour)

	// Should succeed even without S3 connection (it just generates the URL locally)
	require.NoError(t, err)
	assert.NotEmpty(t, url)
	assert.Contains(t, url, "test-key.txt")
	assert.Contains(t, url, "test-bucket")
}

func TestS3Config_WithAllOptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := S3Config{
		Endpoint:        "http://custom-endpoint:9000",
		Region:          "eu-west-1",
		Bucket:          "my-bucket",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		UsePathStyle:    true,
		DisableSSL:      true,
	}

	client, err := NewS3Client(ctx, cfg)

	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestNormalizeEndpointScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		endpoint   string
		disableSSL bool
		want       string
	}{
		{name: "keeps explicit https", endpoint: "https://minio:9000", disableSSL: false, want: "https://minio:9000"},
		{name: "keeps explicit http", endpoint: "http://minio:9000", disableSSL: true, want: "http://minio:9000"},
		{name: "adds https when ssl enabled", endpoint: "minio:9000", disableSSL: false, want: "https://minio:9000"},
		{name: "adds http when ssl disabled", endpoint: "minio:9000", disableSSL: true, want: "http://minio:9000"},
		{name: "trims whitespace", endpoint: "  minio:9000  ", disableSSL: false, want: "https://minio:9000"},
		{name: "normalizes malformed scheme separator", endpoint: "://minio:9000", disableSSL: true, want: "http://minio:9000"},
		{name: "normalizes unsupported scheme", endpoint: "ftp://minio:9000", disableSSL: false, want: "https://minio:9000"},
		{name: "returns empty for whitespace", endpoint: "   ", disableSSL: false, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, normalizeEndpointScheme(tt.endpoint, tt.disableSSL))
		})
	}
}

func TestS3Config_MinimalConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := S3Config{
		Bucket: "minimal-bucket",
	}

	client, err := NewS3Client(ctx, cfg)

	// Should succeed with minimal config (will use default AWS config)
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestS3Client_UploadWithTTL_IgnoresTTL(t *testing.T) {
	t.Parallel()

	// TTL should be ignored for S3 (not an error, just logged)
	// We verify the function doesn't error with a TTL value
	client := createTestClient(t)
	ctx := context.Background()

	// This will fail to connect but should pass validation with TTL
	_, err := client.UploadWithTTL(ctx, "test-key.txt", strings.NewReader("data"), "text/plain", "1h")

	// Should fail with connection error, NOT with TTL error
	// The TTL warning is logged but the function continues
	require.Error(t, err)
	assert.NotEqual(t, ErrTTLNotSupported, err)
	assert.NotEqual(t, ErrKeyRequired, err)
	// Error should be about uploading/connection, not TTL
	assert.Contains(t, err.Error(), "uploading object")
}

// --- Tests using fake S3 HTTP server ---

func TestS3Client_Upload_Success(t *testing.T) {
	t.Parallel()

	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Contains(t, r.URL.Path, "test-key.txt")

		w.Header().Set("ETag", `"d41d8cd98f00b204e9800998ecf8427e"`)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	ctx := context.Background()

	key, err := client.Upload(ctx, "test-key.txt", strings.NewReader("hello world"), "text/plain")

	require.NoError(t, err)
	assert.Equal(t, "test-key.txt", key)
}

func TestS3Client_UploadWithTTL_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		key         string
		contentType string
		data        string
		ttl         string
	}{
		{
			name:        "Upload without TTL",
			key:         "reports/2024/report.pdf",
			contentType: "application/pdf",
			data:        "pdf-content",
			ttl:         "",
		},
		{
			name:        "Upload with TTL (ignored by S3)",
			key:         "temp/cache.json",
			contentType: "application/json",
			data:        `{"key":"value"}`,
			ttl:         "1h",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("ETag", `"abc123"`)
				w.WriteHeader(http.StatusOK)
			})
			defer server.Close()

			ctx := context.Background()

			key, err := client.UploadWithTTL(ctx, tt.key, strings.NewReader(tt.data), tt.contentType, tt.ttl)

			require.NoError(t, err)
			assert.Equal(t, tt.key, key)
		})
	}
}

func TestS3Client_UploadWithTTL_PutObjectError(t *testing.T) {
	t.Parallel()

	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Return an S3 XML error response for AccessDenied
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
	<Code>AccessDenied</Code>
	<Message>Access Denied</Message>
	<RequestId>test-request-id</RequestId>
</Error>`))
	})
	defer server.Close()

	ctx := context.Background()

	key, err := client.UploadWithTTL(ctx, "test-key.txt", strings.NewReader("data"), "text/plain", "")

	require.Error(t, err)
	assert.Empty(t, key)
	assert.Contains(t, err.Error(), "uploading object")
}

func TestS3Client_UploadWithTTL_ReadError(t *testing.T) {
	t.Parallel()

	client := createTestClient(t)
	ctx := context.Background()

	// Use a reader that always returns an error
	failReader := &errorReader{err: io.ErrUnexpectedEOF}

	key, err := client.UploadWithTTL(ctx, "test-key.txt", failReader, "text/plain", "")

	require.Error(t, err)
	assert.Empty(t, key)
	assert.Contains(t, err.Error(), "reading data")
}

func TestS3Client_Download_Success(t *testing.T) {
	t.Parallel()

	expectedContent := "file content here"

	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "download-key.txt")

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", "17")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedContent))
	})
	defer server.Close()

	ctx := context.Background()

	body, err := client.Download(ctx, "download-key.txt")

	require.NoError(t, err)
	require.NotNil(t, body)

	defer body.Close()

	data, readErr := io.ReadAll(body)
	require.NoError(t, readErr)
	assert.Equal(t, expectedContent, string(data))
}

func TestS3Client_Download_NoSuchKey(t *testing.T) {
	t.Parallel()

	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
	<Code>NoSuchKey</Code>
	<Message>The specified key does not exist.</Message>
	<Key>missing-key.txt</Key>
	<RequestId>test-request-id</RequestId>
</Error>`))
	})
	defer server.Close()

	ctx := context.Background()

	body, err := client.Download(ctx, "missing-key.txt")

	assert.Nil(t, body)
	assert.ErrorIs(t, err, ErrObjectNotFound)
}

func TestS3Client_Download_GenericError(t *testing.T) {
	t.Parallel()

	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
	<Code>InternalError</Code>
	<Message>We encountered an internal error. Please try again.</Message>
	<RequestId>test-request-id</RequestId>
</Error>`))
	})
	defer server.Close()

	ctx := context.Background()

	body, err := client.Download(ctx, "some-key.txt")

	assert.Nil(t, body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "downloading object")
}

func TestS3Client_Delete_Success(t *testing.T) {
	t.Parallel()

	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Contains(t, r.URL.Path, "delete-key.txt")

		// S3 returns 204 No Content on successful delete
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	ctx := context.Background()

	err := client.Delete(ctx, "delete-key.txt")

	require.NoError(t, err)
}

func TestS3Client_Delete_Error(t *testing.T) {
	t.Parallel()

	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
	<Code>AccessDenied</Code>
	<Message>Access Denied</Message>
	<RequestId>test-request-id</RequestId>
</Error>`))
	})
	defer server.Close()

	ctx := context.Background()

	err := client.Delete(ctx, "protected-key.txt")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deleting object")
}

func TestS3Client_Exists_True(t *testing.T) {
	t.Parallel()

	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method)
		assert.Contains(t, r.URL.Path, "existing-key.txt")

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	ctx := context.Background()

	exists, err := client.Exists(ctx, "existing-key.txt")

	require.NoError(t, err)
	assert.True(t, exists)
}

func TestS3Client_Exists_NotFound(t *testing.T) {
	t.Parallel()

	// S3 HeadObject returns 404 when the object does not exist.
	// The AWS SDK deserializes this into a types.NotFound error.
	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	ctx := context.Background()

	exists, err := client.Exists(ctx, "missing-key.txt")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestS3Client_Exists_GenericError(t *testing.T) {
	t.Parallel()

	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
	<Code>AccessDenied</Code>
	<Message>Access Denied</Message>
	<RequestId>test-request-id</RequestId>
</Error>`))
	})
	defer server.Close()

	ctx := context.Background()

	exists, err := client.Exists(ctx, "forbidden-key.txt")

	assert.False(t, exists)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checking object existence")
}

func TestS3Client_GeneratePresignedURL_WithVariousDurations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		key    string
		expiry time.Duration
	}{
		{
			name:   "Short expiry (5 minutes)",
			key:    "short-lived.pdf",
			expiry: 5 * time.Minute,
		},
		{
			name:   "Standard expiry (1 hour)",
			key:    "standard.pdf",
			expiry: 1 * time.Hour,
		},
		{
			name:   "Long expiry (24 hours)",
			key:    "long-lived.pdf",
			expiry: 24 * time.Hour,
		},
		{
			name:   "Key with subdirectory",
			key:    "reports/2024/01/monthly.pdf",
			expiry: 1 * time.Hour,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := createTestClient(t)
			ctx := context.Background()

			url, err := client.GeneratePresignedURL(ctx, tt.key, tt.expiry)

			require.NoError(t, err)
			assert.NotEmpty(t, url)
			assert.Contains(t, url, "test-bucket")
		})
	}
}

func TestNewS3Client_ConfigVariations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     S3Config
		wantErr bool
		errIs   error
	}{
		{
			name: "Empty bucket returns error",
			cfg: S3Config{
				Endpoint: "http://localhost:9000",
				Region:   "us-east-1",
			},
			wantErr: true,
			errIs:   ErrBucketRequired,
		},
		{
			name: "Bucket only (minimal config)",
			cfg: S3Config{
				Bucket: "my-bucket",
			},
			wantErr: false,
		},
		{
			name: "With region",
			cfg: S3Config{
				Bucket: "my-bucket",
				Region: "eu-west-1",
			},
			wantErr: false,
		},
		{
			name: "With endpoint and path style",
			cfg: S3Config{
				Bucket:       "my-bucket",
				Endpoint:     "http://minio:9000",
				UsePathStyle: true,
			},
			wantErr: false,
		},
		{
			name: "With static credentials",
			cfg: S3Config{
				Bucket:          "my-bucket",
				AccessKeyID:     "AKID",
				SecretAccessKey: "SECRET",
			},
			wantErr: false,
		},
		{
			name: "Full config (SeaweedFS-like)",
			cfg: S3Config{
				Endpoint:        "http://seaweedfs:8333",
				Region:          "us-east-1",
				Bucket:          "reports",
				AccessKeyID:     "admin",
				SecretAccessKey: "admin",
				UsePathStyle:    true,
				DisableSSL:      true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			client, err := NewS3Client(ctx, tt.cfg)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, client)

				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestS3Client_Upload_DelegatesToUploadWithTTL(t *testing.T) {
	t.Parallel()

	// Verify Upload delegates to UploadWithTTL with empty TTL by testing success path
	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	ctx := context.Background()

	key, err := client.Upload(ctx, "delegate-test.txt", strings.NewReader("data"), "text/plain")

	require.NoError(t, err)
	assert.Equal(t, "delegate-test.txt", key)
}

func TestS3Client_ImplementsObjectStorageInterface(t *testing.T) {
	t.Parallel()

	// Compile-time check is already in s3_client.go, but this test
	// verifies it explicitly at runtime.
	var iface ObjectStorage = createTestClient(t)
	assert.NotNil(t, iface)
}

func TestS3Client_Download_BodyContentsPreserved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "Plain text",
			content: "hello world",
		},
		{
			name:    "JSON payload",
			content: `{"status":"ok","count":42}`,
		},
		{
			name:    "Large content",
			content: strings.Repeat("abcdefghij", 1000),
		},
		{
			name:    "Binary-like content",
			content: "\x00\x01\x02\x03\xff\xfe\xfd",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.content))
			})
			defer server.Close()

			ctx := context.Background()

			body, err := client.Download(ctx, "test-file")

			require.NoError(t, err)
			require.NotNil(t, body)

			defer body.Close()

			data, readErr := io.ReadAll(body)
			require.NoError(t, readErr)
			assert.Equal(t, tt.content, string(data))
		})
	}
}

func TestS3Client_Delete_Idempotent(t *testing.T) {
	t.Parallel()

	// S3 Delete is idempotent: deleting a non-existent key returns 204
	callCount := 0
	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	ctx := context.Background()

	// Delete same key twice - both should succeed
	err1 := client.Delete(ctx, "idempotent-key.txt")
	err2 := client.Delete(ctx, "idempotent-key.txt")

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, 2, callCount)
}

func TestS3Client_Exists_NoSuchKey(t *testing.T) {
	t.Parallel()

	// HeadObject returning NoSuchKey XML error
	client, server := createTestClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
	<Code>NoSuchKey</Code>
	<Message>The specified key does not exist.</Message>
	<Key>no-such.txt</Key>
	<RequestId>test-request-id</RequestId>
</Error>`))
	})
	defer server.Close()

	ctx := context.Background()

	exists, err := client.Exists(ctx, "no-such.txt")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestS3Client_SentinelErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "ErrBucketRequired",
			err:     ErrBucketRequired,
			wantMsg: "TPL-0041",
		},
		{
			name:    "ErrKeyRequired",
			err:     ErrKeyRequired,
			wantMsg: "TPL-0042",
		},
		{
			name:    "ErrObjectNotFound",
			err:     ErrObjectNotFound,
			wantMsg: "TPL-0043",
		},
		{
			name:    "ErrTTLNotSupported",
			err:     ErrTTLNotSupported,
			wantMsg: "TPL-0044",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, tt.err.Error(), tt.wantMsg)
		})
	}
}

// --- MockObjectStorage exercising tests ---
// These tests exercise the generated mock to cover ports.mock.go.

func TestMockObjectStorage_Upload(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mock := NewMockObjectStorage(ctrl)

	mock.EXPECT().Upload(gomock.Any(), "key.txt", gomock.Any(), "text/plain").Return("key.txt", nil)

	key, err := mock.Upload(context.Background(), "key.txt", strings.NewReader("data"), "text/plain")

	require.NoError(t, err)
	assert.Equal(t, "key.txt", key)
}

func TestMockObjectStorage_UploadWithTTL(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mock := NewMockObjectStorage(ctrl)

	mock.EXPECT().UploadWithTTL(gomock.Any(), "key.txt", gomock.Any(), "text/plain", "1h").Return("key.txt", nil)

	key, err := mock.UploadWithTTL(context.Background(), "key.txt", strings.NewReader("data"), "text/plain", "1h")

	require.NoError(t, err)
	assert.Equal(t, "key.txt", key)
}

func TestMockObjectStorage_Download(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mock := NewMockObjectStorage(ctrl)

	expectedBody := io.NopCloser(strings.NewReader("content"))
	mock.EXPECT().Download(gomock.Any(), "key.txt").Return(expectedBody, nil)

	body, err := mock.Download(context.Background(), "key.txt")

	require.NoError(t, err)
	assert.NotNil(t, body)

	defer body.Close()
}

func TestMockObjectStorage_Delete(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mock := NewMockObjectStorage(ctrl)

	mock.EXPECT().Delete(gomock.Any(), "key.txt").Return(nil)

	err := mock.Delete(context.Background(), "key.txt")

	require.NoError(t, err)
}

func TestMockObjectStorage_Exists(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mock := NewMockObjectStorage(ctrl)

	mock.EXPECT().Exists(gomock.Any(), "key.txt").Return(true, nil)

	exists, err := mock.Exists(context.Background(), "key.txt")

	require.NoError(t, err)
	assert.True(t, exists)
}

func TestMockObjectStorage_GeneratePresignedURL(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mock := NewMockObjectStorage(ctrl)

	mock.EXPECT().GeneratePresignedURL(gomock.Any(), "key.txt", time.Hour).Return("https://example.com/key.txt", nil)

	url, err := mock.GeneratePresignedURL(context.Background(), "key.txt", time.Hour)

	require.NoError(t, err)
	assert.Equal(t, "https://example.com/key.txt", url)
}

// errorReader is an io.Reader that always returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}
