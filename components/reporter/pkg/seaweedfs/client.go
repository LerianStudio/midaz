// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seaweedfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/LerianStudio/reporter/pkg/constant"
)

// SeaweedFSClient provides direct HTTP access to SeaweedFS
type SeaweedFSClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewSeaweedFSClient creates a new simple HTTP client for SeaweedFS
func NewSeaweedFSClient(baseURL string) *SeaweedFSClient {
	return &SeaweedFSClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: constant.SeaweedFSHTTPTimeout,
		},
	}
}

// UploadFile uploads a file to SeaweedFS
func (c *SeaweedFSClient) UploadFile(ctx context.Context, path string, data []byte) error {
	return c.UploadFileWithTTL(ctx, path, data, "")
}

// UploadFileWithTTL uploads a file to SeaweedFS with optional TTL
func (c *SeaweedFSClient) UploadFileWithTTL(ctx context.Context, path string, data []byte, ttl string) error {
	url := fmt.Sprintf("%s%s", c.baseURL, path)
	if ttl != "" {
		url = fmt.Sprintf("%s?ttl=%s", url, ttl)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DownloadFile downloads a file from SeaweedFS
func (c *SeaweedFSClient) DownloadFile(ctx context.Context, path string) ([]byte, error) {
	url := fmt.Sprintf("%s%s", c.baseURL, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return data, nil
}

// DeleteFile deletes a file from SeaweedFS
func (c *SeaweedFSClient) DeleteFile(ctx context.Context, path string) error {
	url := fmt.Sprintf("%s%s", c.baseURL, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// HealthCheck checks if SeaweedFS is accessible
func (c *SeaweedFSClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/status", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// GetBaseURL returns the base URL of the SeaweedFS server
func (c *SeaweedFSClient) GetBaseURL() string {
	return c.baseURL
}
