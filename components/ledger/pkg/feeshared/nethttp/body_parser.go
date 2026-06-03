// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"reflect"
	"regexp"
	"sync"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"

	"github.com/LerianStudio/lib-commons/v5/commons"
	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"
)

// sanitizeTypeCache caches field indices for struct types to avoid repeated reflection
var sanitizeTypeCache sync.Map

// sanitizeCacheEntry holds cached information about a struct type for sanitization
type sanitizeCacheEntry struct {
	stringFields  []int // indices of string fields
	structFields  []int // indices of struct fields
	pointerFields []int // indices of pointer fields
	sliceFields   []int // indices of slice fields
	mapFields     []int // indices of map fields
}

// DecodeHandlerFunc is a handler which works with withBody decorator.
// It receives a struct which was decoded by withBody decorator before.
// Ex: json -> withBody -> DecodeHandlerFunc.
type DecodeHandlerFunc func(p any, c *fiber.Ctx) error

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

// Regex for special characters
// Allow letters, numbers, dash, underscore, space, @, dot, comma, slash and backslash
var specialCharsRegex = regexp.MustCompile(`[^a-zA-Z0-9\\/\-_ @.,]`)

// sanitizeString remove all special characters from a string
func sanitizeString(input string) string {
	return specialCharsRegex.ReplaceAllString(input, "")
}

// getSanitizeCacheEntry returns cached field indices for a struct type, computing them if not cached
func getSanitizeCacheEntry(t reflect.Type) *sanitizeCacheEntry {
	if cached, ok := sanitizeTypeCache.Load(t); ok {
		return cached.(*sanitizeCacheEntry)
	}

	entry := &sanitizeCacheEntry{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		switch field.Type.Kind() {
		case reflect.String:
			entry.stringFields = append(entry.stringFields, i)
		case reflect.Struct:
			entry.structFields = append(entry.structFields, i)
		case reflect.Ptr:
			entry.pointerFields = append(entry.pointerFields, i)
		case reflect.Slice:
			entry.sliceFields = append(entry.sliceFields, i)
		case reflect.Map:
			entry.mapFields = append(entry.mapFields, i)
		}
	}

	sanitizeTypeCache.Store(t, entry)

	return entry
}

// sanitizeStruct remove all special characters from a struct using cached field indices
func sanitizeStruct(s any) {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()
	cache := getSanitizeCacheEntry(t)

	// Process string fields
	for _, i := range cache.stringFields {
		field := v.Field(i)
		if field.CanSet() {
			sanitizeStringField(field)
		}
	}

	// Process struct fields
	for _, i := range cache.structFields {
		field := v.Field(i)
		if field.CanSet() {
			sanitizeStruct(field.Addr().Interface())
		}
	}

	// Process pointer fields
	for _, i := range cache.pointerFields {
		field := v.Field(i)
		if field.CanSet() {
			sanitizePointerField(field)
		}
	}

	// Process slice fields
	for _, i := range cache.sliceFields {
		field := v.Field(i)
		if field.CanSet() {
			sanitizeSliceField(field)
		}
	}

	// Process map fields
	for _, i := range cache.mapFields {
		field := v.Field(i)
		if field.CanSet() {
			sanitizeMapField(field)
			// If the field is a map with struct values, sanitize each value
			valType := field.Type().Elem()
			if valType.Kind() == reflect.Struct {
				for _, key := range field.MapKeys() {
					val := field.MapIndex(key)
					valCopy := reflect.New(valType).Elem()
					valCopy.Set(val)
					sanitizeStruct(valCopy.Addr().Interface())
					field.SetMapIndex(key, valCopy)
				}
			}
		}
	}
}

// sanitizeStringField remove all special characters from a string
func sanitizeStringField(field reflect.Value) {
	sanitized := sanitizeString(field.String())
	field.SetString(sanitized)
}

// sanitizePointerField remove all special characters from a pointer
func sanitizePointerField(field reflect.Value) {
	if field.IsNil() {
		return
	}

	switch field.Type().Elem().Kind() {
	case reflect.String:
		sanitized := sanitizeString(field.Elem().String())
		field.Elem().SetString(sanitized)
	case reflect.Struct:
		sanitizeStruct(field.Interface())
	}
}

// sanitizeSliceField remove all special characters from a slice
func sanitizeSliceField(field reflect.Value) {
	for j := 0; j < field.Len(); j++ {
		elem := field.Index(j)
		switch elem.Kind() {
		case reflect.String:
			sanitized := sanitizeString(elem.String())
			elem.SetString(sanitized)
		case reflect.Struct:
			sanitizeStruct(elem.Addr().Interface())
		}
	}
}

// sanitizeMapField remove all special characters from a map
func sanitizeMapField(field reflect.Value) {
	if field.Type().Key().Kind() == reflect.String && field.Type().Elem().Kind() == reflect.String {
		for _, key := range field.MapKeys() {
			sanitized := sanitizeString(field.MapIndex(key).String())
			field.SetMapIndex(key, reflect.ValueOf(sanitized))
		}
	}
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
func (d *decoderHandler) FiberHandlerFunc(c *fiber.Ctx) error {
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

	sanitizeStruct(s)

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
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
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
