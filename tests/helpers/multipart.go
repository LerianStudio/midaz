package helpers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// RequestMultipart sends a multipart/form-data request with optional text fields and file parts.
func (c *HTTPClient) RequestMultipart(ctx context.Context, method, path string, headers map[string]string, fields map[string]string, files map[string]struct {
	Field, Filename string
	Content         []byte
},
) (int, []byte, http.Header, error) {
	buf, contentType, err := buildMultipartBody(fields, files)
	if err != nil {
		return 0, nil, nil, err
	}

	if headers == nil {
		headers = map[string]string{}
	}

	headers["Content-Type"] = contentType

	return c.sendMultipartRequest(ctx, method, path, headers, buf)
}

// buildMultipartBody constructs the multipart body and returns the buffer and content type
func buildMultipartBody(fields map[string]string, files map[string]struct {
	Field, Filename string
	Content         []byte
},
) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer

	mw := multipart.NewWriter(&buf)

	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}

	for _, f := range files {
		fw, err := mw.CreateFormFile(f.Field, f.Filename)
		if err != nil {
			//nolint:wrapcheck // Error already wrapped with context for test helpers
			return nil, "", fmt.Errorf("failed to create form file: %w", err)
		}

		if _, err = io.Copy(fw, bytes.NewReader(f.Content)); err != nil {
			//nolint:wrapcheck // Error already wrapped with context for test helpers
			return nil, "", fmt.Errorf("failed to copy file content: %w", err)
		}
	}

	if err := mw.Close(); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return &buf, mw.FormDataContentType(), nil
}

// sendMultipartRequest sends the multipart request with the provided buffer
func (c *HTTPClient) sendMultipartRequest(ctx context.Context, method, path string, headers map[string]string, buf *bytes.Buffer) (int, []byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, buf)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, nil, nil, fmt.Errorf("failed to create multipart request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, nil, nil, fmt.Errorf("failed to execute multipart request: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return resp.StatusCode, nil, resp.Header, fmt.Errorf("failed to read multipart response body: %w", err)
	}

	return resp.StatusCode, b, resp.Header, nil
}

// PostDSL is a convenience for uploading a DSL file required by the DSL endpoint.
func (c *HTTPClient) PostDSL(ctx context.Context, path string, headers map[string]string, dsl string) (int, []byte, http.Header, error) {
	files := map[string]struct {
		Field, Filename string
		Content         []byte
	}{
		"dsl": {Field: "dsl", Filename: "test.gold", Content: []byte(dsl)},
	}

	return c.RequestMultipart(ctx, http.MethodPost, path, headers, nil, files)
}
