// Package helpers provides multipart form utilities for Midaz integration tests.
//
// # Purpose
//
// This file extends HTTPClient with multipart/form-data request capabilities
// for testing file upload endpoints.
//
// # Features
//
//   - RequestMultipart: Generic multipart form with fields and files
//   - PostDSL: Convenience method for DSL file uploads
//
// # Usage
//
//	// Upload a file
//	files := map[string]struct{ Field, Filename string; Content []byte }{
//	    "doc": {Field: "document", Filename: "test.pdf", Content: pdfBytes},
//	}
//	status, body, headers, err := client.RequestMultipart(ctx, "POST", "/upload",
//	    authHeaders, nil, files)
//
//	// Upload DSL file
//	status, body, headers, err := client.PostDSL(ctx, "/api/v1/dsl",
//	    authHeaders, dslContent)
package helpers

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
)

// RequestMultipart sends a multipart/form-data request with optional text fields and file parts.
//
// # Parameters
//
//   - ctx: Context for request cancellation
//   - method: HTTP method (typically POST or PUT)
//   - path: URL path appended to base URL
//   - headers: Additional request headers (Content-Type is auto-set)
//   - fields: Text form fields (field name -> value)
//   - files: File parts (key -> {Field, Filename, Content})
//
// # Process
//
//	Step 1: Create multipart writer
//	Step 2: Write all text fields
//	Step 3: Create form file parts and write content
//	Step 4: Close multipart writer to finalize boundary
//	Step 5: Set Content-Type with boundary
//	Step 6: Execute request
//
// # Returns
//
//   - int: HTTP status code
//   - []byte: Response body
//   - http.Header: Response headers
//   - error: Request or multipart encoding error
func (c *HTTPClient) RequestMultipart(ctx context.Context, method, path string, headers map[string]string, fields map[string]string, files map[string]struct {
	Field, Filename string
	Content         []byte
}) (int, []byte, http.Header, error) {
	var buf bytes.Buffer

	mw := multipart.NewWriter(&buf)

	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}

	for _, f := range files {
		fw, err := mw.CreateFormFile(f.Field, f.Filename)
		if err != nil {
			return 0, nil, nil, err
		}

		if _, err = io.Copy(fw, bytes.NewReader(f.Content)); err != nil {
			return 0, nil, nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return 0, nil, nil, err
	}

	if headers == nil {
		headers = map[string]string{}
	}

	headers["Content-Type"] = mw.FormDataContentType()

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, &buf)
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

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, resp.Header, err
	}

	return resp.StatusCode, b, resp.Header, nil
}

// PostDSL is a convenience for uploading a DSL file required by the DSL endpoint.
//
// This method wraps RequestMultipart for the common case of uploading DSL content
// to the DSL processing endpoint.
//
// # Parameters
//
//   - ctx: Context for request cancellation
//   - path: URL path for DSL endpoint (e.g., "/api/v1/dsl")
//   - headers: Request headers (typically from AuthHeaders)
//   - dsl: DSL content as string
//
// # File Details
//
// The DSL is uploaded as:
//   - Field name: "dsl"
//   - Filename: "test.gold"
//   - Content: UTF-8 encoded dsl string
//
// # Returns
//
//   - int: HTTP status code
//   - []byte: Response body
//   - http.Header: Response headers
//   - error: Request error
func (c *HTTPClient) PostDSL(ctx context.Context, path string, headers map[string]string, dsl string) (int, []byte, http.Header, error) {
	files := map[string]struct {
		Field, Filename string
		Content         []byte
	}{
		"dsl": {Field: "dsl", Filename: "test.gold", Content: []byte(dsl)},
	}

	return c.RequestMultipart(ctx, http.MethodPost, path, headers, nil, files)
}
