// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import "time"

const (
	TemplateBucketName = "templates"
	ReportBucketName   = "reports"
)

// SeaweedFS HTTP client configuration.
const (
	// SeaweedFSHTTPTimeout is the timeout for HTTP requests to the SeaweedFS server.
	SeaweedFSHTTPTimeout = 30 * time.Second
)
