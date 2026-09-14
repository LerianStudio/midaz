// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"reflect"

	"github.com/LerianStudio/midaz/v4/pkg"

	"github.com/LerianStudio/lib-commons/v7/commons"
	commonsHttp "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	"github.com/gofiber/fiber/v3"
	"github.com/shopspring/decimal"
)

// DecodeHandlerFunc is a handler which works with withBody decorator.
// It receives a struct which was decoded by withBody decorator before.
// Ex: json -> withBody -> DecodeHandlerFunc.
type DecodeHandlerFunc func(p any, c fiber.Ctx) error

// PayloadContextValue is a wrapper type used to keep Context.Locals safe.
type PayloadContextValue string

// ConstructorFunc representing a constructor of any type.
type ConstructorFunc func() any

// decoderHandler decodes payload coming from requests.
type decoderHandler struct {
	handler      DecodeHandlerFunc
	constructor  ConstructorFunc
	structSource any
}

func newOfType(s any) any {
	t := reflect.TypeOf(s)
	v := reflect.New(t.Elem())

	return v.Interface()
}

func WithBody(s any, h DecodeHandlerFunc) fiber.Handler {
	d := &decoderHandler{
		handler:      h,
		structSource: s,
	}

	return d.FiberHandlerFunc
}

// FiberHandlerFunc is a method on the decoderHandler struct. It decodes the incoming request's body to a Go struct,
// validates it, checks for any extraneous fields not defined in the struct, and finally calls the wrapped handler function.
func (d *decoderHandler) FiberHandlerFunc(c fiber.Ctx) error {
	var s any

	if d.constructor != nil {
		s = d.constructor()
	} else {
		s = newOfType(d.structSource)
	}

	bodyBytes := c.Body() // Get the body bytes

	if err := json.Unmarshal(bodyBytes, s); err != nil {
		return commonsHttp.Respond(c, fiber.StatusBadRequest, pkg.ValidateUnmarshallingError(err))
	}

	marshaled, err := json.Marshal(s)
	if err != nil {
		return commonsHttp.Respond(c, fiber.StatusBadRequest, pkg.ValidateUnmarshallingError(err))
	}

	var originalMap, marshaledMap map[string]any

	if err := json.Unmarshal(bodyBytes, &originalMap); err != nil {
		return commonsHttp.Respond(c, fiber.StatusBadRequest, pkg.ValidateUnmarshallingError(err))
	}

	if err := json.Unmarshal(marshaled, &marshaledMap); err != nil {
		return commonsHttp.Respond(c, fiber.StatusBadRequest, pkg.ValidateUnmarshallingError(err))
	}

	diffFields := findUnknownFields(originalMap, marshaledMap)

	if len(diffFields) > 0 {
		err := pkg.ValidateBadRequestFieldsError(pkg.FieldValidations{}, pkg.FieldValidations{}, "", diffFields)
		return commonsHttp.Respond(c, fiber.StatusBadRequest, err)
	}

	if err := ValidateStruct(s); err != nil {
		return commonsHttp.Respond(c, fiber.StatusBadRequest, err)
	}

	c.Locals("fields", diffFields)

	parseMetadata(s, originalMap)

	return d.handler(s, c)
}

// findUnknownFields checks if the marshaled value is different from the original value
func findUnknownFields(original, marshaled map[string]any) map[string]any {
	diffFields := make(map[string]any)

	for key, value := range original {
		if isZeroFloat(value) {
			continue
		}

		marshaledValue, ok := marshaled[key]
		if !ok {
			diffFields[key] = value
			continue
		}

		if nestedDiff := handleNestedDifferences(value, marshaledValue); nestedDiff != nil {
			diffFields[key] = nestedDiff
		}
	}

	return diffFields
}

// isZeroFloat checks if the value is zero for numeric types.
// Returns false for nil values to handle missing fields gracefully.
func isZeroFloat(value any) bool {
	if value == nil {
		return false
	}

	numKinds := commons.GetMapNumKinds()

	return numKinds[reflect.ValueOf(value).Kind()] && value == 0.0
}

// handleNestedDifferences checks if the marshaled value is different from the original value
func handleNestedDifferences(originalVal, marshaledVal any) any {
	switch v := originalVal.(type) {
	case map[string]any:
		return handleMapDifference(v, marshaledVal)
	case []any:
		return handleSliceDifference(v, marshaledVal)
	case string:
		if isStringNumeric(v) {
			return nil
		}
	}

	if !reflect.DeepEqual(originalVal, marshaledVal) {
		return originalVal
	}

	return nil
}

// handleMapDifference checks if the marshaled map is different from the original map
func handleMapDifference(originalMap map[string]any, marshaledVal any) any {
	if marshaledMap, ok := marshaledVal.(map[string]any); ok {
		nestedDiff := findUnknownFields(originalMap, marshaledMap)
		if len(nestedDiff) > 0 {
			return nestedDiff
		}
	} else {
		return originalMap
	}

	return nil
}

// handleSliceDifference checks if the marshaled slice is different from the original slice
func handleSliceDifference(originalSlice []any, marshaledVal any) any {
	if marshaledSlice, ok := marshaledVal.([]any); ok {
		arrayDiff := compareSlices(originalSlice, marshaledSlice)
		if len(arrayDiff) > 0 {
			return arrayDiff
		}
	} else {
		return originalSlice
	}

	return nil
}

// isStringNumeric checks if a string is numeric
func isStringNumeric(s string) bool {
	_, err := decimal.NewFromString(s)
	return err == nil
}

// compareSlices compares two slices and returns differences.
func compareSlices(original, marshaled []any) []any {
	var diff []any

	// Iterate through the original slice and check differences
	for i, item := range original {
		if i >= len(marshaled) {
			// If marshaled slice is shorter, the original item is missing
			diff = append(diff, item)
		} else {
			tmpMarshaled := marshaled[i]
			// Compare individual items at the same index
			if originalMap, ok := item.(map[string]any); ok {
				if marshaledMap, ok := tmpMarshaled.(map[string]any); ok {
					nestedDiff := findUnknownFields(originalMap, marshaledMap)
					if len(nestedDiff) > 0 {
						diff = append(diff, nestedDiff)
					}
				}
			} else if !reflect.DeepEqual(item, tmpMarshaled) {
				diff = append(diff, item)
			}
		}
	}

	// Check if marshaled slice is longer
	for i := len(original); i < len(marshaled); i++ {
		diff = append(diff, marshaled[i])
	}

	return diff
}

// parseMetadata For compliance with RFC7396 JSON Merge Patch
func parseMetadata(s any, originalMap map[string]any) {
	val := reflect.ValueOf(s)
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return
	}

	val = val.Elem()

	metadataField := val.FieldByName("Metadata")
	if !metadataField.IsValid() || !metadataField.CanSet() {
		return
	}

	if _, exists := originalMap["metadata"]; !exists {
		metadataField.Set(reflect.ValueOf(make(map[string]any)))
	}
}
