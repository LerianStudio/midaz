// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import (
	"fmt"
	"net/http"

	libStreaming "github.com/LerianStudio/lib-streaming"
)

// ManifestRoutePath is the canonical HTTP path under which the streaming
// manifest document is served by every producing component.
const ManifestRoutePath = "/v1/streaming/manifest"

// NewManifestHTTPHandler builds the lib-streaming manifest handler for a
// producing component from its own catalog, keeping pkg/streaming
// framework-agnostic by returning a stdlib net/http handler. Each component
// wraps the result with its Fiber adaptor at its own layer.
//
// serviceName and sourceBase are the component identity (e.g. "ledger");
// sourceBase is the first segment of every manifest topic. A descriptor whose
// ServiceName or SourceBase is empty fails lib-streaming validation, which is
// surfaced as a wrapped error rather than a nil handler.
func NewManifestHTTPHandler(serviceName, sourceBase string, catalog libStreaming.Catalog) (http.Handler, error) {
	desc := libStreaming.PublisherDescriptor{
		ServiceName: serviceName,
		SourceBase:  sourceBase,
		RoutePath:   ManifestRoutePath,
	}

	h, err := libStreaming.NewStreamingHandler(desc, catalog)
	if err != nil {
		return nil, fmt.Errorf("build streaming manifest handler: %w", err)
	}

	return h, nil
}
