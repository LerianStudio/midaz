package http

import (
	"encoding/json"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libTransction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/pkg"
	cn "github.com/LerianStudio/midaz/pkg/constant"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	en2 "github.com/go-playground/validator/translations/en"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gopkg.in/go-playground/validator.v9"
	"reflect"
	"regexp"
	"strconv"
	"strings"
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
		return BadRequest(c, pkg.ValidateUnmarshallingError(err))
	}

	marshaled, err := json.Marshal(s)
	if err != nil {
		return BadRequest(c, pkg.ValidateUnmarshallingError(err))
	}

	var originalMap, marshaledMap map[string]any

	if err := json.Unmarshal(bodyBytes, &originalMap); err != nil {
		return BadRequest(c, pkg.ValidateUnmarshallingError(err))
	}

	if err := json.Unmarshal(marshaled, &marshaledMap); err != nil {
		return BadRequest(c, pkg.ValidateUnmarshallingError(err))
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

// WithDecode wraps a handler function, providing it with a struct instance created using the provided constructor function.
func WithDecode(c ConstructorFunc, h DecodeHandlerFunc) fiber.Handler {
	d := &decoderHandler{
		handler:     h,
		constructor: c,
	}

	return d.FiberHandlerFunc
}

// WithBody wraps a handler function, providing it with an instance of the specified struct.
func WithBody(s any, h DecodeHandlerFunc) fiber.Handler {
	d := &decoderHandler{
		handler:      h,
		structSource: s,
	}

	return d.FiberHandlerFunc
}

// SetBodyInContext is a higher-order function that wraps a Fiber handler, injecting the decoded body into the request context.
func SetBodyInContext(handler fiber.Handler) DecodeHandlerFunc {
	return func(s any, c *fiber.Ctx) error {
		c.Locals(string(PayloadContextValue("payload")), s)
		return handler(c)
	}
}

// GetPayloadFromContext retrieves the decoded request payload from the Fiber context.
func GetPayloadFromContext(c *fiber.Ctx) any {
	return c.Locals(string(PayloadContextValue("payload")))
}

// ValidateStruct validates a struct against defined validation rules, using the validator package.
func ValidateStruct(s any) error {
	v, trans := newValidator()

	k := reflect.ValueOf(s).Kind()
	if k == reflect.Ptr {
		k = reflect.ValueOf(s).Elem().Kind()
	}

	if k != reflect.Struct {
		return nil
	}

	err := v.Struct(s)
	if err != nil {
		for _, fieldError := range err.(validator.ValidationErrors) {
			switch fieldError.Tag() {
			case "keymax":
				return pkg.ValidateBusinessError(cn.ErrMetadataKeyLengthExceeded, "", fieldError.Translate(trans), fieldError.Param())
			case "valuemax":
				return pkg.ValidateBusinessError(cn.ErrMetadataValueLengthExceeded, "", fieldError.Translate(trans), fieldError.Param())
			case "nonested":
				return pkg.ValidateBusinessError(cn.ErrInvalidMetadataNesting, "", fieldError.Translate(trans))
			case "singletransactiontype":
				return pkg.ValidateBusinessError(cn.ErrInvalidTransactionType, "", fieldError.Translate(trans))
			}
		}

		errPtr := malformedRequestErr(err.(validator.ValidationErrors), trans)

		return &errPtr
	}

	return nil
}

// ParseUUIDPathParameters globally, considering all path parameters are UUIDs
func ParseUUIDPathParameters(c *fiber.Ctx) error {
	params := c.AllParams()

	var invalidUUIDs []string

	validPathParamsMap := make(map[string]any)

	for param, value := range params {
		if !libCommons.Contains[string](cn.UUIDPathParameters, param) {
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

//nolint:ireturn
func newValidator() (*validator.Validate, ut.Translator) {
	locale := en.New()
	uni := ut.New(locale, locale)

	trans, _ := uni.GetTranslator("en")

	v := validator.New()

	if err := en2.RegisterDefaultTranslations(v, trans); err != nil {
		panic(err)
	}

	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}

		return name
	})

	_ = v.RegisterValidation("keymax", validateMetadataKeyMaxLength)
	_ = v.RegisterValidation("nonested", validateMetadataNestedValues)
	_ = v.RegisterValidation("valuemax", validateMetadataValueMaxLength)
	_ = v.RegisterValidation("singletransactiontype", validateSingleTransactionType)
	_ = v.RegisterValidation("prohibitedexternalaccountprefix", validateProhibitedExternalAccountPrefix)

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

	_ = v.RegisterTranslation("singletransactiontype", trans, func(ut ut.Translator) error {
		return ut.Add("singletransactiontype", "{0}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("singletransactiontype", formatErrorFieldName(fe.Namespace()))

		return t
	})

	_ = v.RegisterTranslation("prohibitedexternalaccountprefix", trans, func(ut ut.Translator) error {
		prefix := cn.DefaultExternalAccountAliasPrefix
		return ut.Add("prohibitedexternalaccountprefix", "{0} cannot contain the text '"+prefix+"'", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("prohibitedexternalaccountprefix", formatErrorFieldName(fe.Namespace()))

		return t
	})

	return v, trans
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

// validateSingleTransactionType checks if a transaction has only one type of transaction (amount, share, or remaining)
func validateSingleTransactionType(fl validator.FieldLevel) bool {
	arrField := fl.Field().Interface().([]libTransction.FromTo)
	for _, f := range arrField {
		count := 0
		if f.Amount != nil {
			count++
		}

		if f.Share != nil {
			count++
		}

		if f.Remaining != "" {
			count++
		}

		if count != 1 {
			return false
		}
	}

	return true
}

// validateProhibitedExternalAccountPrefix
func validateProhibitedExternalAccountPrefix(fl validator.FieldLevel) bool {
	f := fl.Field().Interface().(string)

	return !strings.Contains(f, cn.DefaultExternalAccountAliasPrefix)
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

func malformedRequestErr(err validator.ValidationErrors, trans ut.Translator) pkg.ValidationKnownFieldsError {
	invalidFieldsMap := fields(err, trans)

	requiredFields := fieldsRequired(invalidFieldsMap)

	var vErr pkg.ValidationKnownFieldsError
	_ = errors.As(pkg.ValidateBadRequestFieldsError(requiredFields, invalidFieldsMap, "", make(map[string]any)), &vErr)

	return vErr
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
