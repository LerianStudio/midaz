// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"maps"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// flattenLegacyFeeExemption rewrites a feeExemption stored as an object to the JSON
// string the fee engine writes, so records made before that contract stay flat. Any
// other value, or one that cannot be encoded, is left for the flat-metadata check.
func flattenLegacyFeeExemption(metadata map[string]any) map[string]any {
	var exemption map[string]any

	switch value := metadata["feeExemption"].(type) {
	case map[string]any:
		exemption = value
	case bson.D:
		exemption = make(map[string]any, len(value))
		for _, element := range value {
			exemption[element.Key] = element.Value
		}
	default:
		return metadata
	}

	encoded, err := json.Marshal(exemption)
	if err != nil {
		return metadata
	}

	flat := maps.Clone(metadata)
	flat["feeExemption"] = string(encoded)

	return flat
}
