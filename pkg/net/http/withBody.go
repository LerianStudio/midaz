package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	en2 "github.com/go-playground/validator/translations/en"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gopkg.in/go-playground/validator.v9"
)

const (
	// defaultMetadataKeyLimit is the default maximum length for metadata keys
	defaultMetadataKeyLimit = 100
	// defaultMetadataValueLimit is the default maximum length for metadata values
	defaultMetadataValueLimit = 2000
	// cpfLength is the expected length for a CPF document
	cpfLength = 11
	// cnpjLength is the expected length for a CNPJ document
	cnpjLength = 14
	// cpfFirstCheckDigitWeight is the starting weight for CPF first check digit calculation
	cpfFirstCheckDigitWeight = 10
	// cpfSecondCheckDigitWeight is the starting weight for CPF second check digit calculation
	cpfSecondCheckDigitWeight = 11
	// cpfFirstCheckDigitCount is the number of digits used in first check digit calculation
	cpfFirstCheckDigitCount = 9
	// cpfSecondCheckDigitCount is the number of digits used in second check digit calculation
	cpfSecondCheckDigitCount = 10
	// cnpjFirstCheckDigitCount is the number of digits used in first check digit calculation
	cnpjFirstCheckDigitCount = 12
	// cnpjSecondCheckDigitCount is the number of digits used in second check digit calculation
	cnpjSecondCheckDigitCount = 13
	// checkDigitModulo is the modulo used for check digit calculation
	checkDigitModulo = 11
	// checkDigitMultiplier is the multiplier used for check digit calculation
	checkDigitMultiplier = 10
	// maxCheckDigitRemainder is the maximum remainder that gets reset to 0
	maxCheckDigitRemainder = 10
	// minValidCheckDigitRemainder is the minimum remainder for non-zero check digits
	minValidCheckDigitRemainder = 2
	// firstDigitIndex is the index of the first digit
	firstDigitIndex = 1
	// zeroDigit is the ASCII value for '0'
	zeroDigit = '0'
	// nineDigit is the max digit value
	nineDigit = 9
	// jsonTagSplitLimit is the limit for splitting JSON tags
	jsonTagSplitLimit = 2
)

var fieldNameRegex = regexp.MustCompile(`\.(.+)$`)

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

	diffFields := FindUnknownFields(originalMap, marshaledMap)

	if len(diffFields) > 0 {
		err := pkg.ValidateBadRequestFieldsError(pkg.FieldValidations{}, pkg.FieldValidations{}, "", diffFields)
		return BadRequest(c, err)
	}

	if err := ValidateStruct(s); err != nil {
		return BadRequest(c, err)
	}

	c.Locals("fields", diffFields)
	c.Locals("patchRemove", findNilFields(originalMap, ""))

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

	if !isStructType(s) {
		return nil
	}

	err := v.Struct(s)
	if err != nil {
		var validationErrs validator.ValidationErrors
		if errors.As(err, &validationErrs) {
			if businessErr := checkBusinessValidationErrors(validationErrs, trans); businessErr != nil {
				return businessErr
			}

			errPtr := malformedRequestErr(validationErrs, trans)

			return &errPtr
		}
	}

	// Generic null-byte validation across all string fields in the payload
	if violations := validateNoNullBytes(s); len(violations) > 0 {
		return fmt.Errorf("null byte validation failed: %w",
			pkg.ValidateBadRequestFieldsError(pkg.FieldValidations{}, violations, "", map[string]any{}))
	}

	return nil
}

// isStructType checks if the given value is a struct type
func isStructType(s any) bool {
	k := reflect.ValueOf(s).Kind()
	if k == reflect.Ptr {
		k = reflect.ValueOf(s).Elem().Kind()
	}

	return k == reflect.Struct
}

// checkBusinessValidationErrors checks for business validation errors and returns appropriate error
func checkBusinessValidationErrors(errs validator.ValidationErrors, trans ut.Translator) error {
	for _, fieldError := range errs {
		switch fieldError.Tag() {
		case "keymax":
			return fmt.Errorf("metadata key length validation failed: %w",
				pkg.ValidateBusinessError(cn.ErrMetadataKeyLengthExceeded, "", fieldError.Translate(trans), fieldError.Param()))
		case "valuemax":
			return fmt.Errorf("metadata value length validation failed: %w",
				pkg.ValidateBusinessError(cn.ErrMetadataValueLengthExceeded, "", fieldError.Translate(trans), fieldError.Param()))
		case "nonested":
			return fmt.Errorf("metadata nesting validation failed: %w",
				pkg.ValidateBusinessError(cn.ErrInvalidMetadataNesting, "", fieldError.Translate(trans)))
		case "singletransactiontype":
			return fmt.Errorf("transaction type validation failed: %w",
				pkg.ValidateBusinessError(cn.ErrInvalidTransactionType, "", fieldError.Translate(trans)))
		case "invalidstrings":
			return fmt.Errorf("account type validation failed: %w",
				pkg.ValidateBusinessError(cn.ErrInvalidAccountType, "", fieldError.Translate(trans), fieldError.Param()))
		case "invalidaliascharacters":
			return fmt.Errorf("account alias validation failed: %w",
				pkg.ValidateBusinessError(cn.ErrAccountAliasInvalid, "", fieldError.Translate(trans), fieldError.Param()))
		case "invalidaccounttype":
			return fmt.Errorf("account type key value validation failed: %w",
				pkg.ValidateBusinessError(cn.ErrInvalidAccountTypeKeyValue, "", fieldError.Translate(trans)))
		}
	}

	return nil
}

// ParseUUIDPathParameters globally, considering all path parameters are UUIDs and adding them to the span attributes
// entityName is a snake_case string used to identify id name, for example the "organization" entity name will result in "app.request.organization_id"
// otherwise the path parameter "id" in a request for example "/v1/organizations/:id" will be parsed as "app.request.id"
func ParseUUIDPathParameters(entityName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		for param, value := range c.AllParams() {
			if !libCommons.Contains(cn.UUIDPathParameters, param) {
				c.Locals(param, value)
				continue
			}

			libOpentelemetry.SetSpanAttributeForParam(c, param, value, entityName)

			parsedUUID, err := uuid.Parse(value)
			if err != nil {
				err := pkg.ValidateBusinessError(cn.ErrInvalidPathParameter, "", param)
				return WithError(c, err)
			}

			c.Locals(param, parsedUUID)
		}

		return c.Next()
	}
}

//nolint:ireturn
func newValidator() (*validator.Validate, ut.Translator) {
	locale := en.New()
	uni := ut.New(locale, locale)

	trans, _ := uni.GetTranslator("en")

	v := validator.New()

	err := en2.RegisterDefaultTranslations(v, trans)
	assert.NoError(err, "validator translations registration required",
		"package", "http",
		"function", "newValidator")

	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", jsonTagSplitLimit)[0]
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
	_ = v.RegisterValidation("invalidstrings", validateInvalidStrings)
	_ = v.RegisterValidation("invalidaliascharacters", validateInvalidAliasCharacters)
	_ = v.RegisterValidation("invalidaccounttype", validateAccountType)
	_ = v.RegisterValidation("nowhitespaces", validateNoWhitespaces)
	_ = v.RegisterValidation("cpfcnpj", validateCPFCNPJ)
	_ = v.RegisterValidation("cpf", validateCPF)
	_ = v.RegisterValidation("cnpj", validateCNPJ)

	_ = v.RegisterTranslation("required", trans, func(ut ut.Translator) error {
		return ut.Add("required", "{0} is a required field", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("required", formatErrorFieldName(fe.Namespace()))

		return t
	})

	_ = v.RegisterTranslation("cpfcnpj", trans, func(ut ut.Translator) error {
		return ut.Add("cpfcnpj", "{0} must be a valid CPF or CNPJ", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("cpfcnpj", formatErrorFieldName(fe.Namespace()))

		return t
	})

	_ = v.RegisterTranslation("cpf", trans, func(ut ut.Translator) error {
		return ut.Add("cpf", "{0} must be a valid CPF", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("cpf", formatErrorFieldName(fe.Namespace()))
		return t
	})

	_ = v.RegisterTranslation("cnpj", trans, func(ut ut.Translator) error {
		return ut.Add("cnpj", "{0} must be a valid CNPJ", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("cnpj", formatErrorFieldName(fe.Namespace()))
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

	_ = v.RegisterTranslation("invalidstrings", trans, func(ut ut.Translator) error {
		return ut.Add("invalidstrings", "{0} cannot contain any of these invalid strings: {1}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("invalidstrings", formatErrorFieldName(fe.Namespace()), fe.Param())
		return t
	})

	_ = v.RegisterTranslation("invalidaliascharacters", trans, func(ut ut.Translator) error {
		return ut.Add("invalidaliascharacters", "{0}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("invalidaliascharacters", formatErrorFieldName(fe.Namespace()))

		return t
	})

	_ = v.RegisterTranslation("invalidaccounttype", trans, func(ut ut.Translator) error {
		return ut.Add("invalidaccounttype", "{0}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("invalidaccounttype", formatErrorFieldName(fe.Namespace()))

		return t
	})

	_ = v.RegisterTranslation("nowhitespaces", trans, func(ut ut.Translator) error {
		return ut.Add("nowhitespaces", "{0} cannot contain whitespaces", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("nowhitespaces", formatErrorFieldName(fe.Namespace()))

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

	limit := defaultMetadataKeyLimit

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

	limit := defaultMetadataValueLimit

	if limitParam != "" {
		if parsedParam, err := strconv.Atoi(limitParam); err == nil {
			limit = parsedParam
		}
	}

	value := convertFieldToString(fl.Field())
	if value == "" && fl.Field().Kind() != reflect.String {
		return false
	}

	return len(value) <= limit
}

// convertFieldToString converts a reflect.Value to string based on its kind
func convertFieldToString(field reflect.Value) string {
	switch field.Kind() {
	case reflect.Int:
		return strconv.Itoa(int(field.Int()))
	case reflect.Float64:
		return strconv.FormatFloat(field.Float(), 'f', -1, 64)
	case reflect.String:
		return field.String()
	case reflect.Bool:
		return strconv.FormatBool(field.Bool())
	default:
		return ""
	}
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

// validateInvalidAliasCharacters validate if it has invalid characters on alias. only permit a-zA-Z0-9@:_-
func validateInvalidAliasCharacters(fl validator.FieldLevel) bool {
	f := fl.Field().Interface().(string)

	validChars := regexp.MustCompile(cn.AccountAliasAcceptedChars)

	return validChars.MatchString(f)
}

// validateAccountType checks if the string contains only alphanumeric characters, _ or -, and no spaces.
func validateAccountType(fl validator.FieldLevel) bool {
	f, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}

	match, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, f)

	return match
}

// validateNoWhitespaces ensures the provided string does not contain any whitespace characters. Return false if input is invalid.
func validateNoWhitespaces(fl validator.FieldLevel) bool {
	f, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}

	match, _ := regexp.MatchString(`^\S+$`, f)

	return match
}

// formatErrorFieldName formats metadata field error names for error messages
func formatErrorFieldName(text string) string {
	matches := fieldNameRegex.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}

	return text
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

// validateNoNullBytes walks through the struct payload and ensures no string value contains a null byte (\x00).
// Returns a map of invalid field names to error messages when violations are found.
func validateNoNullBytes(s any) pkg.FieldValidations {
	out := make(pkg.FieldValidations)

	rv := reflect.ValueOf(s)

	collectNullByteViolations(rv, "", out)

	if len(out) == 0 {
		return nil
	}

	return out
}

// collectNullByteViolations recursively traverses values and records fields that contain null bytes.
func collectNullByteViolations(rv reflect.Value, jsonPath string, out pkg.FieldValidations) {
	if !rv.IsValid() {
		return
	}

	switch rv.Kind() {
	case reflect.Ptr:
		handlePtrNullByteViolations(rv, jsonPath, out)
	case reflect.Struct:
		handleStructNullByteViolations(rv, out)
	case reflect.Slice, reflect.Array:
		handleSliceNullByteViolations(rv, jsonPath, out)
	case reflect.String:
		handleStringNullByteViolation(rv, jsonPath, out)
	default:
		// primitives: no-op
	}
}

// handlePtrNullByteViolations handles null byte violations in pointer types
func handlePtrNullByteViolations(rv reflect.Value, jsonPath string, out pkg.FieldValidations) {
	if rv.IsNil() {
		return
	}

	collectNullByteViolations(rv.Elem(), jsonPath, out)
}

// handleStructNullByteViolations handles null byte violations in struct types
func handleStructNullByteViolations(rv reflect.Value, out pkg.FieldValidations) {
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		f := rt.Field(i)

		// Skip unexported fields
		if f.PkgPath != "" {
			continue
		}

		name := jsonFieldName(f)
		if name == "-" {
			continue
		}

		collectNullByteViolations(rv.Field(i), name, out)
	}
}

// handleSliceNullByteViolations handles null byte violations in slice/array types
func handleSliceNullByteViolations(rv reflect.Value, jsonPath string, out pkg.FieldValidations) {
	for i := 0; i < rv.Len(); i++ {
		collectNullByteViolations(rv.Index(i), jsonPath, out)
	}
}

// handleStringNullByteViolation handles null byte violations in string types
func handleStringNullByteViolation(rv reflect.Value, jsonPath string, out pkg.FieldValidations) {
	if strings.ContainsRune(rv.String(), '\x00') {
		key := jsonPath
		if key == "" {
			key = "value"
		}

		out[key] = key + " cannot contain null byte (\\x00)"
	}
}

// jsonFieldName returns the effective JSON field name for a struct field.
func jsonFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("json")

	name := strings.Split(tag, ",")[0]

	if name == "" {
		return f.Name
	}

	return name
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

// FindUnknownFields finds fields that are present in the original map but not in the marshaled map.
func FindUnknownFields(original, marshaled map[string]any) map[string]any {
	diffFields := make(map[string]any)
	numKinds := libCommons.GetMapNumKinds()

	for key, value := range original {
		if shouldSkipZeroNumeric(value, numKinds) {
			continue
		}

		marshaledValue, ok := marshaled[key]
		if !ok {
			diffFields[key] = value
			continue
		}

		checkFieldDifference(key, value, marshaledValue, diffFields)
	}

	return diffFields
}

// shouldSkipZeroNumeric checks if a value is a zero numeric value that should be skipped
func shouldSkipZeroNumeric(value any, numKinds map[reflect.Kind]bool) bool {
	return numKinds[reflect.ValueOf(value).Kind()] && value == 0.0
}

// checkFieldDifference checks if there's a difference between original and marshaled values
func checkFieldDifference(key string, originalValue, marshaledValue any, diffFields map[string]any) {
	switch original := originalValue.(type) {
	case map[string]any:
		handleMapDifference(key, original, marshaledValue, originalValue, diffFields)
	case []any:
		handleSliceDifference(key, original, marshaledValue, originalValue, diffFields)
	case string:
		handleStringDifference(key, original, marshaledValue, originalValue, diffFields)
	default:
		handleDefaultDifference(key, originalValue, marshaledValue, diffFields)
	}
}

// handleMapDifference handles differences in map values
func handleMapDifference(key string, originalMap map[string]any, marshaledValue, originalValue any, diffFields map[string]any) {
	if marshaledMap, ok := marshaledValue.(map[string]any); ok {
		nestedDiff := FindUnknownFields(originalMap, marshaledMap)
		if len(nestedDiff) > 0 {
			diffFields[key] = nestedDiff
		}
	} else if !reflect.DeepEqual(originalMap, marshaledValue) {
		diffFields[key] = originalValue
	}
}

// handleSliceDifference handles differences in slice values
func handleSliceDifference(key string, originalSlice []any, marshaledValue, originalValue any, diffFields map[string]any) {
	if marshaledArray, ok := marshaledValue.([]any); ok {
		arrayDiff := compareSlices(originalSlice, marshaledArray)
		if len(arrayDiff) > 0 {
			diffFields[key] = arrayDiff
		}
	} else if !reflect.DeepEqual(originalSlice, marshaledValue) {
		diffFields[key] = originalValue
	}
}

// handleStringDifference handles differences in string values
func handleStringDifference(key, originalString string, marshaledValue, originalValue any, diffFields map[string]any) {
	if isStringNumeric(originalString) && isDecimalEqual(originalString, marshaledValue) {
		return
	}

	if !reflect.DeepEqual(originalValue, marshaledValue) {
		diffFields[key] = originalValue
	}
}

// handleDefaultDifference handles differences in default value types
func handleDefaultDifference(key string, originalValue, marshaledValue any, diffFields map[string]any) {
	if !reflect.DeepEqual(originalValue, marshaledValue) {
		diffFields[key] = originalValue
	}
}

func isDecimalEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}

	decimalA, okA := parseDecimalValue(a)
	if !okA {
		return false
	}

	decimalB, okB := parseDecimalValue(b)
	if !okB {
		return false
	}

	return decimalA.Equal(decimalB)
}

// parseDecimalValue converts any value to decimal.Decimal
func parseDecimalValue(val any) (decimal.Decimal, bool) {
	switch v := val.(type) {
	case string:
		d, err := decimal.NewFromString(v)
		if err != nil {
			return decimal.Decimal{}, false
		}

		return d, true
	case decimal.Decimal:
		return v, true

	default:
		return decimal.Decimal{}, false
	}
}

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
			continue
		}

		itemDiff := compareSliceItem(item, marshaled[i])
		if itemDiff != nil {
			diff = append(diff, itemDiff)
		}
	}

	// Check if marshaled slice is longer
	for i := len(original); i < len(marshaled); i++ {
		diff = append(diff, marshaled[i])
	}

	return diff
}

// compareSliceItem compares a single item from two slices
func compareSliceItem(original, marshaled any) any {
	originalMap, okOrig := original.(map[string]any)
	if !okOrig {
		if !reflect.DeepEqual(original, marshaled) {
			return original
		}

		return nil
	}

	marshaledMap, okMarsh := marshaled.(map[string]any)
	if !okMarsh {
		return original
	}

	nestedDiff := FindUnknownFields(originalMap, marshaledMap)
	if len(nestedDiff) > 0 {
		return nestedDiff
	}

	return nil
}

// validateInvalidStrings checks if a string contains any of the invalid strings (case-insensitive)
func validateInvalidStrings(fl validator.FieldLevel) bool {
	f := strings.ToLower(fl.Field().Interface().(string))

	invalidStrings := strings.Split(fl.Param(), ",")

	for _, str := range invalidStrings {
		if strings.Contains(f, strings.ToLower(str)) {
			return false
		}
	}

	return true
}

// findNilFields recursively traverses the map and returns the paths
// of the fields whose value is nil.
// The prefix parameter is used to build the complete path (e.g., "object.field").
func findNilFields(data map[string]any, prefix string) []string {
	var nilFields []string

	for key, value := range data {
		var fullPath string
		if prefix == "" {
			fullPath = key
		} else {
			fullPath = prefix + "." + key
		}

		if value == nil {
			nilFields = append(nilFields, fullPath)
		} else {
			if nestedMap, ok := value.(map[string]any); ok {
				nilFields = append(nilFields, findNilFields(nestedMap, fullPath)...)
			}
		}
	}

	return nilFields
}

func validateCPFCNPJ(fl validator.FieldLevel) bool {
	value := fl.Field().Interface().(string)
	if value == "" {
		return true
	}

	if len(value) != cpfLength && len(value) != cnpjLength {
		return false
	}

	if len(value) == cpfLength {
		return validateCPF(fl)
	}

	return validateCNPJ(fl)
}

func validateCPF(fl validator.FieldLevel) bool {
	cpf := fl.Field().Interface().(string)
	if cpf == "" {
		return true
	}

	if len(cpf) != cpfLength {
		return false
	}

	if hasAllEqualDigits(cpf) {
		return false
	}

	// Validate first check digit
	if !validateCPFCheckDigit(cpf, cpfFirstCheckDigitCount, cpfFirstCheckDigitWeight, cpfFirstCheckDigitCount) {
		return false
	}

	// Validate second check digit
	return validateCPFCheckDigit(cpf, cpfSecondCheckDigitCount, cpfSecondCheckDigitWeight, cpfSecondCheckDigitCount)
}

// hasAllEqualDigits checks if all digits in a string are the same
func hasAllEqualDigits(s string) bool {
	for i := firstDigitIndex; i < len(s); i++ {
		if s[i] != s[0] {
			return false
		}
	}

	return true
}

// validateCPFCheckDigit validates a single CPF check digit
func validateCPFCheckDigit(cpf string, digitCount, weight, checkPosition int) bool {
	sum := 0

	for i := 0; i < digitCount; i++ {
		digit := int(cpf[i] - zeroDigit)
		if digit < 0 || digit > nineDigit {
			return false
		}

		sum += digit * (weight - i)
	}

	remainder := (sum * checkDigitMultiplier) % checkDigitModulo
	if remainder == maxCheckDigitRemainder {
		remainder = 0
	}

	return remainder == int(cpf[checkPosition]-zeroDigit)
}

func validateCNPJ(fl validator.FieldLevel) bool {
	cnpj := fl.Field().Interface().(string)
	if cnpj == "" {
		return true
	}

	if len(cnpj) != cnpjLength {
		return false
	}

	if hasAllEqualDigits(cnpj) {
		return false
	}

	// Weights for first check digit
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	// Weights for second check digit
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}

	// Validate first check digit
	if !validateCNPJCheckDigit(cnpj, cnpjFirstCheckDigitCount, weights1, cnpjFirstCheckDigitCount) {
		return false
	}

	// Validate second check digit
	return validateCNPJCheckDigit(cnpj, cnpjSecondCheckDigitCount, weights2, cnpjSecondCheckDigitCount)
}

// validateCNPJCheckDigit validates a single CNPJ check digit
func validateCNPJCheckDigit(cnpj string, digitCount int, weights []int, checkPosition int) bool {
	sum := 0

	for i := 0; i < digitCount; i++ {
		digit := int(cnpj[i] - zeroDigit)
		if digit < 0 || digit > nineDigit {
			return false
		}

		sum += digit * weights[i]
	}

	remainder := sum % checkDigitModulo
	expectedDigit := 0

	if remainder >= minValidCheckDigitRemainder {
		expectedDigit = checkDigitModulo - remainder
	}

	return expectedDigit == int(cnpj[checkPosition]-zeroDigit)
}
