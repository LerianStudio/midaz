// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// metadataNullPolicy is what "metadata": null in a PATCH body means on the contract that
// received it. Every other metadata body means the same on both contracts.
type metadataNullPolicy int

const (
	// metadataNullClearsV1 is the /v1 RFC 7396 reading: null deletes every key the client wrote.
	metadataNullClearsV1 metadataNullPolicy = iota

	// metadataNullKeepsV2 leaves the stored metadata as it is: /v2 deletes metadata key by key only.
	metadataNullKeepsV2
)

// patchMetadataDocV1 and patchMetadataDocV2 describe how a PATCH applies metadata on each contract.
const (
	patchMetadataDoc = "Metadata is applied as an RFC 7396 merge patch: a body without metadata, or with an empty metadata object, " +
		"leaves the stored metadata as it is; a key sent as null is deleted; any other key is added or replaced; "
	patchMetadataDocV1 = patchMetadataDoc + "metadata sent as null deletes every key the client wrote."
	patchMetadataDocV2 = patchMetadataDoc + "metadata sent as null also leaves the stored metadata as it is, so keys are deleted one at a time."
)

// patchMetadataFor is the null policy and the PATCH description of the contract whose operation IDs
// carry opSuffix.
func patchMetadataFor(opSuffix string) (metadataNullPolicy, string) {
	if opSuffix == v1OpSuffix {
		return metadataNullClearsV1, patchMetadataDocV1
	}

	return metadataNullKeepsV2, patchMetadataDocV2
}

// withMetadataNull binds a PATCH handler shared by both contracts to the policy of the one it is
// registered on.
func withMetadataNull[I, O any](policy metadataNullPolicy, handle func(context.Context, *I, metadataNullPolicy) (*O, error)) func(context.Context, *I) (*O, error) {
	return func(ctx context.Context, in *I) (*O, error) {
		return handle(ctx, in, policy)
	}
}

// decodePatchBody decodes and validates a PATCH body. Under metadataNullKeepsV2 an explicit null
// metadata becomes an empty map and leaves the returned body map, so it writes nothing.
func decodePatchBody(body []byte, payload any, metadata *map[string]any, policy metadataNullPolicy) (map[string]any, error) {
	originalMap, err := pkgHTTP.DecodeAndValidate(body, payload)
	if err != nil || policy != metadataNullKeepsV2 {
		return originalMap, err
	}

	if value, sent := originalMap["metadata"]; sent && value == nil {
		delete(originalMap, "metadata")

		*metadata = map[string]any{}
	}

	return originalMap, nil
}
