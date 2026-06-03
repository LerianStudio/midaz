// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

const (
	healthCheckTickerInterval = 2 * time.Second
	defaultBaseBackoff        = 200 * time.Millisecond
)

type HTTPClient struct {
	base   string
	client *http.Client
}

func NewHTTPClient(base string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{base: base, client: &http.Client{Timeout: timeout}}
}

func (c *HTTPClient) RequestFull(ctx context.Context, method, path string, headers map[string]string, body any) (int, []byte, http.Header, error) {
	var rdr io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("marshal body: %w", err)
		}

		rdr = bytes.NewReader(b)

		if headers == nil {
			headers = map[string]string{}
		}

		if _, ok := headers["Content-Type"]; !ok {
			headers["Content-Type"] = "application/json"
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return 0, nil, nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, resp.Header, err
	}

	return resp.StatusCode, data, resp.Header, nil
}

func (c *HTTPClient) Request(ctx context.Context, method, path string, headers map[string]string, body any) (int, []byte, error) {
	code, b, _, err := c.RequestFull(ctx, method, path, headers, body)
	return code, b, err
}

func (c *HTTPClient) RequestWithHeaderValues(ctx context.Context, method, path string, headers map[string][]string, body any) (int, []byte, http.Header, error) {
	var rdr io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("marshal body: %w", err)
		}

		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return 0, nil, nil, err
	}

	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, resp.Header, err
	}

	return resp.StatusCode, data, resp.Header, nil
}

func (c *HTTPClient) RequestFullWithRetry(ctx context.Context, method, path string, headers map[string]string, body any, attempts int, baseBackoff time.Duration) (int, []byte, http.Header, error) {
	if attempts <= 0 {
		attempts = 1
	}

	if baseBackoff <= 0 {
		baseBackoff = defaultBaseBackoff
	}

	var (
		lastCode int
		lastBody []byte
		lastHdr  http.Header
		lastErr  error
	)

	for i := 0; i < attempts; i++ {
		code, b, hdr, err := c.RequestFull(ctx, method, path, headers, body)

		lastCode, lastBody, lastHdr, lastErr = code, b, hdr, err
		if err == nil && code != 429 && code != 502 && code != 503 && code != 504 {
			return code, b, hdr, nil
		}

		time.Sleep(time.Duration(1<<i) * baseBackoff)
	}

	return lastCode, lastBody, lastHdr, lastErr
}

// ReportStatus represents the status of a report
type ReportStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// GetReportStatus retrieves the status of a specific report
func (c *HTTPClient) GetReportStatus(ctx context.Context, reportID string, headers map[string]string) (*ReportStatus, error) {
	code, body, err := c.Request(ctx, "GET", fmt.Sprintf("/v1/reports/%s", reportID), headers, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", code, string(body))
	}

	var report ReportStatus
	if err := json.Unmarshal(body, &report); err != nil {
		return nil, fmt.Errorf("failed to unmarshal report: %w", err)
	}

	return &report, nil
}

// WaitForReportStatus waits for a report to reach a specific status with timeout
func (c *HTTPClient) WaitForReportStatus(ctx context.Context, reportID string, headers map[string]string, expectedStatus string, timeout time.Duration) (*ReportStatus, error) {
	deadline := time.Now().Add(timeout)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for report %s to reach status %s", reportID, expectedStatus)
			}

			report, err := c.GetReportStatus(ctx, reportID, headers)
			if err != nil {
				continue
			}

			if report.Status == expectedStatus {
				return report, nil
			}
		}
	}
}

// ListReports retrieves a list of reports with optional filters
func (c *HTTPClient) ListReports(ctx context.Context, headers map[string]string, queryParams string) ([]ReportStatus, error) {
	path := "/v1/reports"
	if queryParams != "" {
		path += "?" + queryParams
	}

	code, body, err := c.Request(ctx, "GET", path, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}

	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", code, string(body))
	}

	var response struct {
		Items []ReportStatus `json:"items"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal reports list: %w", err)
	}

	return response.Items, nil
}

// UploadMultipartForm uploads a multipart form with files and form data
func (c *HTTPClient) UploadMultipartForm(ctx context.Context, method, path string, headers map[string]string, formData map[string]string, files map[string][]byte) (int, []byte, error) {
	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	// Add form fields
	for key, value := range formData {
		if err := writer.WriteField(key, value); err != nil {
			return 0, nil, fmt.Errorf("failed to write form field %s: %w", key, err)
		}
	}

	// Add files
	for fieldName, fileData := range files {
		// Use .tpl extension for template files
		filename := fieldName
		if fieldName == "template" {
			filename = "template.tpl"
		}

		part, err := writer.CreateFormFile(fieldName, filename)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to create form file %s: %w", fieldName, err)
		}

		if _, err := part.Write(fileData); err != nil {
			return 0, nil, fmt.Errorf("failed to write file data for %s: %w", fieldName, err)
		}
	}

	// Important: Get the content type BEFORE closing the writer
	contentType := writer.FormDataContentType()

	if err := writer.Close(); err != nil {
		return 0, nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, &body)
	if err != nil {
		return 0, nil, err
	}

	// Add custom headers FIRST (but skip Content-Type)
	for k, v := range headers {
		// Don't override Content-Type - multipart boundary is critical
		if k != "Content-Type" {
			req.Header.Set(k, v)
		}
	}

	// Set Content-Type with boundary LAST (this is critical for multipart)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	return resp.StatusCode, data, nil
}

// WaitForSystemHealth waits for the system to be healthy by checking health endpoint
func WaitForSystemHealth(ctx context.Context, cli *HTTPClient, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	ticker := time.NewTicker(healthCheckTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for system health after %v", timeout)
			}

			// Try to check health endpoint
			code, _, err := cli.Request(ctx, "GET", "/health", nil, nil)
			if err == nil && code == http.StatusOK {
				return nil
			}

			// Also try a simple API call to verify system is responsive
			code, _, err = cli.Request(ctx, "GET", "/v1/templates?limit=1", nil, nil)
			if err == nil && (code == http.StatusOK || code == http.StatusUnauthorized) { // 401 is OK, means auth is working
				return nil
			}
		}
	}
}
