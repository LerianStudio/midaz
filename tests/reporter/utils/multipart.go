// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// CreateTemplateMultipart creates a multipart form for template upload
func (c *HTTPClient) CreateTemplateMultipart(ctx context.Context, path string, headers map[string]string, templateFile, description, outputFormat string) (int, []byte, error) {
	var buf bytes.Buffer

	w := multipart.NewWriter(&buf)

	// Add description field
	if err := w.WriteField("description", description); err != nil {
		return 0, nil, fmt.Errorf("write description field: %w", err)
	}

	// Add outputFormat field
	if err := w.WriteField("outputFormat", outputFormat); err != nil {
		return 0, nil, fmt.Errorf("write outputFormat field: %w", err)
	}

	// Add template file
	fw, err := w.CreateFormFile("templateFile", filepath.Base(templateFile))
	if err != nil {
		return 0, nil, fmt.Errorf("create form file: %w", err)
	}

	// Open and stream file content instead of reading it all at once
	// #nosec G304 - this is a test helper; the path is provided by controlled test fixtures
	f, err := os.Open(templateFile)
	if err != nil {
		return 0, nil, fmt.Errorf("open template file %s: %w", templateFile, err)
	}
	defer f.Close()

	if _, err := io.Copy(fw, f); err != nil {
		return 0, nil, fmt.Errorf("write template content: %w", err)
	}

	if err := w.Close(); err != nil {
		return 0, nil, fmt.Errorf("close multipart writer: %w", err)
	}

	// Set content type
	if headers == nil {
		headers = make(map[string]string)
	}

	headers["Content-Type"] = w.FormDataContentType()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, &buf)
	if err != nil {
		return 0, nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

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
