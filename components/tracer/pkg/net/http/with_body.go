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
	"sync"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en2 "github.com/go-playground/validator/v10/translations/en"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"tracer/pkg"
	cn "tracer/pkg/constant"
)

var UUIDPathParameters = []string{
	"id",
}

var errorFieldNameRegex = regexp.MustCompile(`\.(.+)$`)

// Package-level validator instance (lazy initialized with sync.Once)
var (
	validatorInstance   *validator.Validate
	validatorTranslator ut.Translator
	validatorInitErr    error
	validatorOnce       sync.Once
)

// getValidator returns the singleton validator instance, initializing it on first call.
// This provides better testability than init() as initialization happens on first use.
func getValidator() (*validator.Validate, ut.Translator, error) {
	validatorOnce.Do(func() {
		validatorInstance, validatorTranslator, validatorInitErr = initValidator()
	})

	return validatorInstance, validatorTranslator, validatorInitErr
}

// initValidator creates and configures the validator instance.
// Called once at package initialization.
func initValidator() (*validator.Validate, ut.Translator, error) {
	locale := en.New()
	uni := ut.New(locale, locale)

	trans, found := uni.GetTranslator("en")
	if !found {
		return nil, nil, fmt.Errorf("failed to get translator for locale 'en'")
	}

	v := validator.New()

	if err := en2.RegisterDefaultTranslations(v, trans); err != nil {
		return nil, nil, fmt.Errorf("failed to register default translations: %w", err)
	}

	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}

		return name
	})

	if err := v.RegisterValidation("keymax", validateMetadataKeyMaxLength); err != nil {
		return nil, nil, fmt.Errorf("failed to register keymax validation: %w", err)
	}

	if err := v.RegisterValidation("nonested", validateMetadataNestedValues); err != nil {
		return nil, nil, fmt.Errorf("failed to register nonested validation: %w", err)
	}

	if err := v.RegisterValidation("valuemax", validateMetadataValueMaxLength); err != nil {
		return nil, nil, fmt.Errorf("failed to register valuemax validation: %w", err)
	}

	if err := v.RegisterTranslation("required", trans, func(ut ut.Translator) error {
		return ut.Add("required", "{0} is a required field", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("required", formatErrorFieldName(fe.Namespace()))

		return t
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to register required translation: %w", err)
	}

	if err := v.RegisterTranslation("gte", trans, func(ut ut.Translator) error {
		return ut.Add("gte", "{0} must be {1} or greater", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("gte", formatErrorFieldName(fe.Namespace()), fe.Param())

		return t
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to register gte translation: %w", err)
	}

	if err := v.RegisterTranslation("eq", trans, func(ut ut.Translator) error {
		return ut.Add("eq", "{0} is not equal to {1}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("eq", formatErrorFieldName(fe.Namespace()), fe.Param())

		return t
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to register eq translation: %w", err)
	}

	if err := v.RegisterTranslation("keymax", trans, func(ut ut.Translator) error {
		return ut.Add("keymax", "{0} key exceeds maximum length of {1} characters", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("keymax", formatErrorFieldName(fe.Namespace()), fe.Param())

		return t
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to register keymax translation: %w", err)
	}

	if err := v.RegisterTranslation("valuemax", trans, func(ut ut.Translator) error {
		return ut.Add("valuemax", "{0} value exceeds maximum length of {1} characters", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("valuemax", formatErrorFieldName(fe.Namespace()), fe.Param())

		return t
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to register valuemax translation: %w", err)
	}

	if err := v.RegisterTranslation("nonested", trans, func(ut ut.Translator) error {
		return ut.Add("nonested", "{0} must not contain nested objects or arrays", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("nonested", formatErrorFieldName(fe.Namespace()))

		return t
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to register nonested translation: %w", err)
	}

	return v, trans, nil
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

func newOfType(s any) (any, error) {
	t := reflect.TypeOf(s)
	if t.Kind() != reflect.Pointer {
		return nil, fmt.Errorf("newOfType: expected pointer, got %s", t.Kind())
	}

	v := reflect.New(t.Elem())

	return v.Interface(), nil
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
		var err error

		s, err = newOfType(d.structSource)
		if err != nil {
			return err
		}
	}

	bodyBytes := c.Body() // Get the body bytes

	if err := json.Unmarshal(bodyBytes, s); err != nil {
		return wrapJSONError(err)
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

	parseMetadata(s, originalMap)

	return d.handler(s, c)
}

// findUnknownFields finds fields that are present in the original map but not in the marshaled map.
func findUnknownFields(original, marshaled map[string]any) map[string]any {
	diffFields := make(map[string]any)

	numKinds := libCommons.GetMapNumKinds()

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
	v, trans, initErr := getValidator()
	if initErr != nil {
		return fmt.Errorf("validator not initialized: %w", initErr)
	}

	k := reflect.ValueOf(s).Kind()
	if k == reflect.Pointer {
		k = reflect.ValueOf(s).Elem().Kind()
	}

	if k != reflect.Struct {
		return nil
	}

	err := v.Struct(s)
	if err != nil {
		var validationErrors validator.ValidationErrors

		ok := errors.As(err, &validationErrors)
		if !ok {
			return err
		}

		for _, fieldError := range validationErrors {
			fieldName := formatErrorFieldName(fieldError.Namespace())

			switch fieldError.Tag() {
			case "keymax":
				return pkg.ValidateBusinessError(cn.ErrMetadataKeyLengthExceeded, "", fieldName, fieldError.Param())
			case "valuemax":
				return pkg.ValidateBusinessError(cn.ErrMetadataValueLengthExceeded, "", fieldName, fieldError.Param())
			case "nonested":
				return pkg.ValidateBusinessError(cn.ErrInvalidMetadataNesting, "", fieldName)
			}
		}

		errPtr := malformedRequestErr(validationErrors, trans)

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

// malformedRequestErr creates a validation error from validator errors.
// The type assertion should always succeed since ValidateBadRequestFieldsError
// returns ValidationKnownFieldsError when given non-nil invalidFieldsMap.
// The fallback empty struct is defensive programming for unexpected edge cases.
func malformedRequestErr(err validator.ValidationErrors, trans ut.Translator) pkg.ValidationKnownFieldsError {
	invalidFieldsMap := fields(err, trans)

	requiredFields := fieldsRequired(invalidFieldsMap)

	result := pkg.ValidateBadRequestFieldsError(requiredFields, invalidFieldsMap, "", make(map[string]any))

	var vErr pkg.ValidationKnownFieldsError
	if errors.As(result, &vErr) {
		return vErr
	}

	// Defensive: should not reach here with valid validator.ValidationErrors input
	return pkg.ValidationKnownFieldsError{}
}

// validateMetadataNestedValues checks if there are nested metadata structures
func validateMetadataNestedValues(fl validator.FieldLevel) bool {
	return fl.Field().Kind() != reflect.Map
}

// validateMetadataKeyMaxLength checks if metadata key (always a string) length is allowed
func validateMetadataKeyMaxLength(fl validator.FieldLevel) bool {
	limitParam := fl.Param()

	limit := 100 // default limit if no param configured

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

	limit := 2000 // default limit if no param configured

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
	matches := errorFieldNameRegex.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}

	return text
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

// ParseUUIDPathParameters globally, considering all path parameters are UUIDs
func ParseUUIDPathParameters(c *fiber.Ctx) error {
	params := c.AllParams()

	var invalidUUIDs []string

	validPathParamsMap := make(map[string]any)

	for param, value := range params {
		if !libCommons.Contains[string](UUIDPathParameters, param) {
			validPathParamsMap[param] = value
			continue
		}

		parsedUUID, err := uuid.Parse(value)
		if err != nil {
			invalidUUIDs = append(invalidUUIDs, param)
			continue
		}

		validPathParamsMap[param] = parsedUUID
	}

	for param, value := range validPathParamsMap {
		c.Locals(param, value)
	}

	if len(invalidUUIDs) > 0 {
		err := pkg.ValidateBusinessError(cn.ErrInvalidPathParameter, "", strings.Join(invalidUUIDs, ", "))
		return WithError(c, err)
	}

	return c.Next()
}

// wrapJSONError wraps JSON unmarshal errors with user-friendly messages.
func wrapJSONError(err error) error {
	var (
		e  *json.SyntaxError
		e1 *json.UnmarshalTypeError
	)

	switch {
	case errors.As(err, &e):
		return fmt.Errorf("invalid JSON syntax at position %d: %w", e.Offset, err)
	case errors.As(err, &e1):
		return fmt.Errorf("invalid type for field '%s': expected %s, got %s: %w", e1.Field, e1.Type.String(), e1.Value, err)
	default:
		return fmt.Errorf("invalid JSON: %w", err)
	}
}
