// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"

	cn "github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"

	en2 "github.com/go-playground/validator/v10/translations/en"
)

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

	// Validate that body is not empty, whitespace-only, or literally "null"
	trimmedBody := strings.TrimSpace(string(bodyBytes))
	if len(trimmedBody) == 0 || trimmedBody == "null" {
		return BadRequest(c, pkg.ValidateBusinessError(cn.ErrMissingRequiredFields, ""))
	}

	if err := json.Unmarshal(bodyBytes, s); err != nil {
		// Convert JSON unmarshal errors to bad request errors
		fieldName := extractFieldNameFromUnmarshalError(err.Error())
		knownFields := make(map[string]string)

		if fieldName != "" {
			knownFields[fieldName] = fmt.Sprintf("Invalid value for this field: %s", err.Error())
		} else {
			knownFields["body"] = fmt.Sprintf("Invalid request body: %s", err.Error())
		}

		return BadRequest(c, pkg.ValidateBadRequestFieldsError(pkg.FieldValidations{}, knownFields, "", make(map[string]any)))
	}

	// Validate type mismatches before proceeding
	if err := validateTypeMismatches(bodyBytes, s); err != nil {
		return BadRequest(c, err)
	}

	marshaled, err := json.Marshal(s)
	if err != nil {
		return err
	}

	var originalMap, marshaledMap map[string]any

	if err := json.Unmarshal(bodyBytes, &originalMap); err != nil {
		return err
	}

	if err := json.Unmarshal(marshaled, &marshaledMap); err != nil {
		return err
	}

	diffFields := findUnknownFields(originalMap, marshaledMap)

	if len(diffFields) > 0 {
		err := pkg.ValidateBadRequestFieldsError(pkg.FieldValidations{}, pkg.FieldValidations{}, "", diffFields)
		return BadRequest(c, err)
	}

	if err := ValidateStruct(s); err != nil {
		return BadRequest(c, err)
	}

	c.Locals("fields", diffFields)

	parseMetadata(s, originalMap)

	return d.handler(s, c)
}

// findUnknownFields finds fields that are present in the original map but not in the marshaled map.
func findUnknownFields(original, marshaled map[string]any) map[string]any {
	diffFields := make(map[string]any)

	numKinds := pkg.GetMapNumKinds()

	for key, value := range original {
		if numKinds[reflect.ValueOf(value).Kind()] && value == 0.0 {
			continue
		}

		marshaledValue, ok := marshaled[key]
		if !ok {
			// If the key is not present in the marshaled map, marking as difference
			diffFields[key] = value
			continue
		}

		// Check for nested structures and direct value comparison
		switch originalValue := value.(type) {
		case map[string]any:
			if marshaledMap, ok := marshaledValue.(map[string]any); ok {
				nestedDiff := findUnknownFields(originalValue, marshaledMap)
				if len(nestedDiff) > 0 {
					diffFields[key] = nestedDiff
				}
			} else if !reflect.DeepEqual(originalValue, marshaledValue) {
				// If types mismatch (map vs non-map), marking as difference
				diffFields[key] = value
			}

		case []any:
			if marshaledArray, ok := marshaledValue.([]any); ok {
				arrayDiff := compareSlices(originalValue, marshaledArray)
				if len(arrayDiff) > 0 {
					diffFields[key] = arrayDiff
				}
			} else if !reflect.DeepEqual(originalValue, marshaledValue) {
				// If types mismatch (slice vs non-slice), marking as difference
				diffFields[key] = value
			}

		default:
			// Using reflect.DeepEqual for simple types (strings, ints, etc.)
			if !reflect.DeepEqual(value, marshaledValue) {
				diffFields[key] = value
			}
		}
	}

	return diffFields
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

// ValidateStruct validates a struct against defined validation rules, using the validator package.
func ValidateStruct(s any) error {
	v, trans, err := newValidator()
	if err != nil {
		return fmt.Errorf("failed to create validator: %w", err)
	}

	k := reflect.ValueOf(s).Kind()
	if k == reflect.Pointer {
		k = reflect.ValueOf(s).Elem().Kind()
	}

	if k != reflect.Struct {
		return nil
	}

	validationErr := v.Struct(s)
	if validationErr != nil {
		for _, fieldError := range validationErr.(validator.ValidationErrors) {
			switch fieldError.Tag() {
			case "keymax":
				return pkg.ValidateBusinessError(cn.ErrMetadataKeyLengthExceeded, "", fieldError.Translate(trans), fieldError.Param())
			case "valuemax":
				return pkg.ValidateBusinessError(cn.ErrMetadataValueLengthExceeded, "", fieldError.Translate(trans), fieldError.Param())
			case "nonested":
				return pkg.ValidateBusinessError(cn.ErrInvalidMetadataNesting, "", fieldError.Translate(trans))
			}
		}

		errPtr := malformedRequestErr(validationErr.(validator.ValidationErrors), trans)

		return &errPtr
	}

	return nil
}

func fields(errs validator.ValidationErrors, trans ut.Translator) pkg.FieldValidations {
	l := len(errs)
	if l > 0 {
		fields := make(pkg.FieldValidations, l)
		for _, e := range errs {
			fields[e.Field()] = e.Translate(trans)
		}

		return fields
	}

	return nil
}

func fieldsRequired(myMap pkg.FieldValidations) pkg.FieldValidations {
	result := make(pkg.FieldValidations)

	for key, value := range myMap {
		if strings.Contains(value, "required") {
			result[key] = value
		}
	}

	return result
}

func malformedRequestErr(err validator.ValidationErrors, trans ut.Translator) pkg.ValidationKnownFieldsError {
	invalidFieldsMap := fields(err, trans)

	requiredFields := fieldsRequired(invalidFieldsMap)

	var vErr pkg.ValidationKnownFieldsError

	_ = errors.As(pkg.ValidateBadRequestFieldsError(requiredFields, invalidFieldsMap, "", make(map[string]any)), &vErr)

	return vErr
}

//nolint:ireturn
func newValidator() (*validator.Validate, ut.Translator, error) {
	locale := en.New()
	uni := ut.New(locale, locale)

	trans, _ := uni.GetTranslator("en")

	v := validator.New()

	if err := en2.RegisterDefaultTranslations(v, trans); err != nil {
		return nil, nil, fmt.Errorf("failed to register default translations: %w", err)
	}

	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", cn.SplitKeyValueParts)[0]
		if name == "-" {
			return ""
		}

		return name
	})

	_ = v.RegisterValidation("keymax", validateMetadataKeyMaxLength)
	_ = v.RegisterValidation("nonested", validateMetadataNestedValues)
	_ = v.RegisterValidation("valuemax", validateMetadataValueMaxLength)

	_ = v.RegisterTranslation("required", trans, func(ut ut.Translator) error {
		return ut.Add("required", "{0} is a required field", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("required", formatErrorFieldName(fe.Namespace()))

		return t
	})

	_ = v.RegisterTranslation("gte", trans, func(ut ut.Translator) error {
		return ut.Add("gte", "{0} must be {1} or greater", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("gte", formatErrorFieldName(fe.Namespace()), fe.Param())

		return t
	})

	_ = v.RegisterTranslation("eq", trans, func(ut ut.Translator) error {
		return ut.Add("eq", "{0} is not equal to {1}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("eq", formatErrorFieldName(fe.Namespace()), fe.Param())

		return t
	})

	_ = v.RegisterTranslation("keymax", trans, func(ut ut.Translator) error {
		return ut.Add("keymax", "{0}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("keymax", formatErrorFieldName(fe.Namespace()))

		return t
	})

	_ = v.RegisterTranslation("valuemax", trans, func(ut ut.Translator) error {
		return ut.Add("valuemax", "{0}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("valuemax", formatErrorFieldName(fe.Namespace()))

		return t
	})

	_ = v.RegisterTranslation("nonested", trans, func(ut ut.Translator) error {
		return ut.Add("nonested", "{0}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("nonested", formatErrorFieldName(fe.Namespace()))

		return t
	})

	return v, trans, nil
}

// validateMetadataNestedValues checks if there are nested metadata structures
func validateMetadataNestedValues(fl validator.FieldLevel) bool {
	return fl.Field().Kind() != reflect.Map
}

// validateMetadataKeyMaxLength checks if metadata key (always a string) length is allowed
func validateMetadataKeyMaxLength(fl validator.FieldLevel) bool {
	limitParam := fl.Param()

	limit := cn.DefaultMetadataKeyMaxLength // default limit if no param configured

	if limitParam != "" {
		if parsedParam, err := strconv.Atoi(limitParam); err == nil {
			limit = parsedParam
		}
	}

	return len(fl.Field().String()) <= limit
}

// validateMetadataValueMaxLength checks metadata value max length
func validateMetadataValueMaxLength(fl validator.FieldLevel) bool {
	limitParam := fl.Param()

	limit := cn.DefaultMetadataValueMaxLength // default limit if no param configured

	if limitParam != "" {
		if parsedParam, err := strconv.Atoi(limitParam); err == nil {
			limit = parsedParam
		}
	}

	var value string

	switch fl.Field().Kind() {
	case reflect.Int:
		value = strconv.Itoa(int(fl.Field().Int()))
	case reflect.Float64:
		value = strconv.FormatFloat(fl.Field().Float(), 'f', -1, 64)
	case reflect.String:
		value = fl.Field().String()
	case reflect.Bool:
		value = strconv.FormatBool(fl.Field().Bool())
	default:
		return false
	}

	return len(value) <= limit
}

// formatErrorFieldName formats metadata field error names for error messages
func formatErrorFieldName(text string) string {
	re, _ := regexp.Compile(`\.(.+)$`)

	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	} else {
		return text
	}
}

// validateTypeMismatches checks if the JSON payload has type mismatches with the struct definition
func validateTypeMismatches(bodyBytes []byte, s any) error {
	var originalMap map[string]any
	if err := json.Unmarshal(bodyBytes, &originalMap); err != nil {
		return err
	}

	val := reflect.ValueOf(s)
	if val.Kind() != reflect.Pointer {
		return nil
	}

	val = val.Elem()

	if val.Kind() != reflect.Struct {
		return nil
	}

	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Get JSON tag name
		jsonTag := fieldType.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Remove omitempty and other options
		jsonName := strings.Split(jsonTag, ",")[0]

		// Check if field exists in original JSON
		if originalValue, exists := originalMap[jsonName]; exists {
			// Check type compatibility
			if err := validateFieldType(originalValue, field, fieldType); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateFieldType validates if the original JSON value is compatible with the struct field type
func validateFieldType(originalValue any, field reflect.Value, fieldType reflect.StructField) error {
	fieldKind := field.Kind()
	jsonTag := fieldType.Tag.Get("json")
	jsonName := strings.Split(jsonTag, ",")[0]

	// Define type compatibility rules
	typeMismatch := getTypeMismatch(originalValue, fieldKind)
	if typeMismatch != nil {
		return pkg.ValidateBusinessError(cn.ErrBadRequest, "", fmt.Sprintf("field '%s' expects %s but received %s", jsonName, fieldKind.String(), typeMismatch.receivedType))
	}

	return nil
}

// typeMismatchInfo holds information about a type mismatch
type typeMismatchInfo struct {
	receivedType string
	value        any
}

type mismatchRule struct {
	receivedType string
	isIncompat   func(reflect.Kind) bool
	preserveVal  bool
}

var mismatchRules = map[reflect.Type]mismatchRule{
	reflect.TypeOf(""):               {receivedType: "string", isIncompat: func(k reflect.Kind) bool { return k == reflect.Map || k == reflect.Slice }, preserveVal: true},
	reflect.TypeOf(map[string]any{}): {receivedType: "object", isIncompat: isSimpleType, preserveVal: false},
	reflect.TypeOf([]any{}):          {receivedType: "array", isIncompat: isSimpleType, preserveVal: false},
	reflect.TypeOf(float64(0)):       {receivedType: "number", isIncompat: func(k reflect.Kind) bool { return k == reflect.String || k == reflect.Map || k == reflect.Slice }, preserveVal: true},
	reflect.TypeOf(false):            {receivedType: "boolean", isIncompat: func(k reflect.Kind) bool { return k == reflect.String || k == reflect.Map || k == reflect.Slice }, preserveVal: true},
}

func getTypeMismatch(originalValue any, fieldKind reflect.Kind) *typeMismatchInfo {
	if originalValue == nil {
		return nil
	}

	rule, exists := mismatchRules[reflect.TypeOf(originalValue)]
	if !exists || !rule.isIncompat(fieldKind) {
		return nil
	}

	var val any
	if rule.preserveVal {
		val = originalValue
	}

	return &typeMismatchInfo{receivedType: rule.receivedType, value: val}
}

// isSimpleType checks if the field kind is a simple type
func isSimpleType(fieldKind reflect.Kind) bool {
	return fieldKind == reflect.String || fieldKind == reflect.Int || fieldKind == reflect.Float64 || fieldKind == reflect.Bool
}

// extractFieldNameFromUnmarshalError extracts the field name from a JSON unmarshal error
func extractFieldNameFromUnmarshalError(errorMsg string) string {
	// Error format: "json: cannot unmarshal string into Go struct field CreateReportInput.filters of type map[string]map[string]map[string][]string"
	// Look for the pattern "struct field PackageName.FieldName"
	re := regexp.MustCompile(`struct field \w+\.(\w+)`)
	matches := re.FindStringSubmatch(errorMsg)

	if len(matches) > 1 {
		return matches[1] // Return the field name (e.g., "filters")
	}

	// Fallback: try to extract just the field name if the pattern doesn't match
	re2 := regexp.MustCompile(`field (\w+) of type`)
	matches2 := re2.FindStringSubmatch(errorMsg)

	if len(matches2) > 1 {
		return matches2[1]
	}

	return ""
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
