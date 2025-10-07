// Package http provides HTTP utilities for the Midaz platform.
// This file contains request body decoding, validation, and middleware functionality.
package http

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	en2 "github.com/go-playground/validator/translations/en"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gopkg.in/go-playground/validator.v9"
)

// Package http provides HTTP utilities and helpers for the Midaz ledger system.
// This file contains request body decoding, validation, and middleware utilities.

// DecodeHandlerFunc is a handler function that receives a decoded and validated request body.
//
// This handler type is used with the WithBody and WithDecode decorators. The decorator
// handles JSON decoding, validation, and unknown field detection before calling this handler.
//
// The flow is: Raw JSON -> Decode -> Validate -> DecodeHandlerFunc
//
// Parameters:
//   - p: Decoded and validated request payload struct
//   - c: Fiber context for the HTTP request
//
// Returns:
//   - error: Handler error (use http.WithError for domain errors)
//
// Example:
//
//	func createAccountHandler(p any, c *fiber.Ctx) error {
//	    input := p.(*mmodel.CreateAccountInput)
//	    account, err := service.CreateAccount(input)
//	    if err != nil {
//	        return http.WithError(c, err)
//	    }
//	    return http.Created(c, account)
//	}
type DecodeHandlerFunc func(p any, c *fiber.Ctx) error

// PayloadContextValue is a type-safe key for storing payloads in Fiber context.
//
// This wrapper type prevents key collisions in the context.Locals map by providing
// a distinct type for payload keys.
type PayloadContextValue string

// ConstructorFunc is a function that creates a new instance of a request payload struct.
//
// This function type is used with WithDecode to create new instances of payload structs
// for each request. Using a constructor function allows for initialization logic beyond
// simple struct instantiation.
//
// Returns:
//   - any: A new instance of the payload struct (typically a pointer)
//
// Example:
//
//	func newCreateAccountInput() any {
//	    return &mmodel.CreateAccountInput{
//	        Status: mmodel.Status{Code: "ACTIVE"}, // Default status
//	    }
//	}
type ConstructorFunc func() any

// decoderHandler is an internal handler that decodes and validates request bodies.
//
// This struct wraps a DecodeHandlerFunc and provides the decoding, validation, and
// unknown field detection logic. It's used internally by WithBody and WithDecode.
type decoderHandler struct {
	handler      DecodeHandlerFunc // The wrapped handler to call after decoding
	constructor  ConstructorFunc   // Optional constructor for creating payload instances
	structSource any               // Optional struct source for reflection-based instantiation
}

// newOfType creates a new instance of a type using reflection.
//
// This function takes a pointer to a struct and creates a new instance of the same type.
// It's used internally by the decoderHandler when no constructor function is provided.
//
// Parameters:
//   - s: Pointer to a struct (e.g., &mmodel.CreateAccountInput{})
//
// Returns:
//   - any: A new pointer to an instance of the same type
//
// Example:
//
//	source := &mmodel.CreateAccountInput{}
//	newInstance := newOfType(source)
//	// newInstance is a new *mmodel.CreateAccountInput
func newOfType(s any) any {
	t := reflect.TypeOf(s)
	v := reflect.New(t.Elem())

	return v.Interface()
}

// FiberHandlerFunc decodes, validates, and processes an HTTP request body.
//
// This method is the core of the request body handling pipeline. It performs the following steps:
// 1. Creates a new instance of the payload struct (via constructor or reflection)
// 2. Unmarshals the JSON request body into the struct
// 3. Detects unknown fields (fields in JSON but not in struct)
// 4. Validates the struct using validation tags
// 5. Parses metadata according to RFC 7396 JSON Merge Patch
// 6. Calls the wrapped handler with the decoded and validated payload
//
// Unknown Field Detection:
//   - Compares original JSON with re-marshaled struct to find extra fields
//   - Returns 400 Bad Request with ValidationUnknownFieldsError if found
//
// Validation:
//   - Uses go-playground/validator for struct validation
//   - Returns 400 Bad Request with ValidationKnownFieldsError if validation fails
//
// Parameters:
//   - c: Fiber context for the HTTP request
//
// Returns:
//   - error: Handler error (validation errors are converted to HTTP responses)
//
// Example Flow:
//
//	// Client sends: {"name": "Account", "extra": "field"}
//	// 1. Decodes to CreateAccountInput
//	// 2. Detects "extra" as unknown field
//	// 3. Returns 400 with ValidationUnknownFieldsError
//
//	// Client sends: {"name": "Account"}
//	// 1. Decodes successfully
//	// 2. No unknown fields
//	// 3. Validates (name is required, passes)
//	// 4. Calls wrapped handler with decoded payload
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

	parseMetadata(s, originalMap)

	return d.handler(s, c)
}

// WithDecode creates a Fiber handler that decodes and validates request bodies using a constructor.
//
// This function wraps a DecodeHandlerFunc with request body decoding and validation logic.
// It uses the provided constructor function to create new payload instances for each request.
//
// Use WithDecode when:
//   - You need custom initialization logic for payload structs
//   - You want to set default values before decoding
//   - You need to inject dependencies into the payload
//
// Parameters:
//   - c: Constructor function that creates new payload instances
//   - h: Handler function that processes the decoded payload
//
// Returns:
//   - fiber.Handler: A Fiber handler that can be used in route definitions
//
// Example:
//
//	func newCreateAccountInput() any {
//	    return &mmodel.CreateAccountInput{
//	        Status: mmodel.Status{Code: "ACTIVE"}, // Default status
//	    }
//	}
//
//	app.Post("/accounts",
//	    http.WithDecode(newCreateAccountInput, createAccountHandler))
func WithDecode(c ConstructorFunc, h DecodeHandlerFunc) fiber.Handler {
	d := &decoderHandler{
		handler:     h,
		constructor: c,
	}

	return d.FiberHandlerFunc
}

// WithBody creates a Fiber handler that decodes and validates request bodies using reflection.
//
// This function wraps a DecodeHandlerFunc with request body decoding and validation logic.
// It uses reflection to create new instances of the provided struct type for each request.
//
// Use WithBody when:
//   - You don't need custom initialization logic
//   - The payload struct can be instantiated with zero values
//   - You want simpler, more concise code
//
// Parameters:
//   - s: Pointer to a struct instance (used as a type template)
//   - h: Handler function that processes the decoded payload
//
// Returns:
//   - fiber.Handler: A Fiber handler that can be used in route definitions
//
// Example:
//
//	app.Post("/accounts",
//	    http.WithBody(&mmodel.CreateAccountInput{}, createAccountHandler))
//
//	func createAccountHandler(p any, c *fiber.Ctx) error {
//	    input := p.(*mmodel.CreateAccountInput)
//	    // Process input...
//	}
func WithBody(s any, h DecodeHandlerFunc) fiber.Handler {
	d := &decoderHandler{
		handler:      h,
		structSource: s,
	}

	return d.FiberHandlerFunc
}

// SetBodyInContext creates a DecodeHandlerFunc that stores the payload in context.
//
// This higher-order function wraps a standard Fiber handler, allowing it to be used
// with WithBody or WithDecode. The decoded payload is stored in the context under the
// "payload" key and can be retrieved later using GetPayloadFromContext.
//
// This is useful when you want to use standard Fiber handlers that don't accept the
// decoded payload as a parameter.
//
// Parameters:
//   - handler: Standard Fiber handler to wrap
//
// Returns:
//   - DecodeHandlerFunc: A handler that stores payload in context before calling the wrapped handler
//
// Example:
//
//	func standardHandler(c *fiber.Ctx) error {
//	    payload := http.GetPayloadFromContext(c)
//	    input := payload.(*mmodel.CreateAccountInput)
//	    // Process input...
//	}
//
//	app.Post("/accounts",
//	    http.WithBody(&mmodel.CreateAccountInput{},
//	        http.SetBodyInContext(standardHandler)))
func SetBodyInContext(handler fiber.Handler) DecodeHandlerFunc {
	return func(s any, c *fiber.Ctx) error {
		c.Locals(string(PayloadContextValue("payload")), s)
		return handler(c)
	}
}

// GetPayloadFromContext retrieves the decoded request payload from the Fiber context.
//
// This function retrieves the payload that was stored by SetBodyInContext. The payload
// must be type-asserted to the expected type before use.
//
// Parameters:
//   - c: Fiber context containing the stored payload
//
// Returns:
//   - any: The decoded payload (must be type-asserted)
//
// Example:
//
//	func handler(c *fiber.Ctx) error {
//	    payload := http.GetPayloadFromContext(c)
//	    if payload == nil {
//	        return http.BadRequest(c, "No payload found")
//	    }
//	    input := payload.(*mmodel.CreateAccountInput)
//	    // Process input...
//	}
func GetPayloadFromContext(c *fiber.Ctx) any {
	return c.Locals(string(PayloadContextValue("payload")))
}

// ValidateStruct validates a struct using go-playground/validator with custom validation rules.
//
// This function performs comprehensive struct validation including:
//   - Standard validation tags (required, max, min, uuid, etc.)
//   - Custom Midaz validation rules (metadata constraints, alias format, etc.)
//   - Null byte detection in string fields (security)
//
// Custom Validation Rules:
//   - keymax: Maximum length for metadata keys
//   - valuemax: Maximum length for metadata values
//   - nonested: Prevents nested objects in metadata
//   - singletransactiontype: Ensures only one transaction type per entry
//   - prohibitedexternalaccountprefix: Prevents @external/ prefix in aliases
//   - invalidstrings: Prevents specific strings (e.g., "external" in type field)
//   - invalidaliascharacters: Validates alias character set
//   - invalidaccounttype: Validates account type format
//   - nowhitespaces: Prevents whitespace in certain fields
//
// Parameters:
//   - s: Struct to validate (typically a pointer to an input model)
//
// Returns:
//   - error: Validation error with field-level details, or nil if valid
//
// Example:
//
//	input := &mmodel.CreateAccountInput{
//	    Name: "Account",
//	    AssetCode: "USD",
//	}
//	if err := http.ValidateStruct(input); err != nil {
//	    return http.BadRequest(c, err)
//	}
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
			case "invalidstrings":
				return pkg.ValidateBusinessError(cn.ErrInvalidAccountType, "", fieldError.Translate(trans), fieldError.Param())
			case "invalidaliascharacters":
				return pkg.ValidateBusinessError(cn.ErrAccountAliasInvalid, "", fieldError.Translate(trans), fieldError.Param())
			case "invalidaccounttype":
				return pkg.ValidateBusinessError(cn.ErrInvalidAccountTypeKeyValue, "", fieldError.Translate(trans))
			}
		}

		errPtr := malformedRequestErr(err.(validator.ValidationErrors), trans)

		return &errPtr
	}

	// Generic null-byte validation across all string fields in the payload
	if violations := validateNoNullBytes(s); len(violations) > 0 {
		return pkg.ValidateBadRequestFieldsError(pkg.FieldValidations{}, violations, "", map[string]any{})
	}

	return nil
}

// ParseUUIDPathParameters creates a middleware that parses and validates UUID path parameters.
//
// This middleware automatically validates that path parameters defined in constant.UUIDPathParameters
// are valid UUIDs. It also adds the parameters to OpenTelemetry span attributes for tracing.
//
// The middleware:
// 1. Iterates through all path parameters
// 2. Checks if the parameter name is in constant.UUIDPathParameters
// 3. Validates the parameter value is a valid UUID
// 4. Stores the parsed UUID in context.Locals for use by handlers
// 5. Adds the parameter to OpenTelemetry span attributes
//
// Parameters:
//   - entityName: Snake_case entity name for span attributes (e.g., "organization", "account")
//     This is used to create span attribute names like "app.request.organization_id"
//
// Returns:
//   - fiber.Handler: Middleware function that can be used in route definitions
//
// Example:
//
//	app.Get("/v1/organizations/:organization_id/ledgers/:id",
//	    http.ParseUUIDPathParameters("ledger"),
//	    getLedgerHandler)
//
//	func getLedgerHandler(c *fiber.Ctx) error {
//	    orgID := c.Locals("organization_id").(uuid.UUID)
//	    ledgerID := c.Locals("id").(uuid.UUID)
//	    // Use parsed UUIDs...
//	}
//
// Error Handling:
//   - Returns 400 Bad Request with ErrInvalidPathParameter if UUID is invalid
//   - Includes the parameter name in the error message
func ParseUUIDPathParameters(entityName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		for param, value := range c.AllParams() {
			if !libCommons.Contains[string](cn.UUIDPathParameters, param) {
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
	_ = v.RegisterValidation("invalidstrings", validateInvalidStrings)
	_ = v.RegisterValidation("invalidaliascharacters", validateInvalidAliasCharacters)
	_ = v.RegisterValidation("invalidaccounttype", validateAccountType)
	_ = v.RegisterValidation("nowhitespaces", validateNoWhitespaces)

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

// validateNoNullBytes recursively checks for null bytes in string fields (security validation).
//
// This function walks through the entire struct (including nested structs, slices, and pointers)
// and validates that no string field contains a null byte (\x00). Null bytes can cause issues
// with C-based libraries, database drivers, and can be used in injection attacks.
//
// The function uses reflection to traverse:
//   - Pointers (follows to pointed value)
//   - Structs (checks all exported fields)
//   - Slices/Arrays (checks all elements)
//   - Strings (validates no \x00 present)
//
// Parameters:
//   - s: Struct to validate (any type)
//
// Returns:
//   - pkg.FieldValidations: Map of field names to error messages (nil if no violations)
//
// Example:
//
//	input := &mmodel.CreateAccountInput{
//	    Name: "Account\x00WithNull",
//	}
//	violations := validateNoNullBytes(input)
//	// Returns: {"name": "name cannot contain null byte (\\x00)"}
//
// Security Note:
//   - This validation prevents null byte injection attacks
//   - Protects against issues with PostgreSQL and other databases
//   - Ensures data integrity in C-based libraries
func validateNoNullBytes(s any) pkg.FieldValidations {
	out := make(pkg.FieldValidations)

	var walk func(rv reflect.Value, jsonPath string)
	walk = func(rv reflect.Value, jsonPath string) {
		if !rv.IsValid() {
			return
		}

		switch rv.Kind() {
		case reflect.Ptr:
			if rv.IsNil() {
				return
			}
			walk(rv.Elem(), jsonPath)
		case reflect.Struct:
			rt := rv.Type()
			for i := 0; i < rv.NumField(); i++ {
				f := rt.Field(i)
				// Skip unexported
				if f.PkgPath != "" {
					continue
				}
				tag := f.Tag.Get("json")
				name := strings.Split(tag, ",")[0]
				if name == "-" {
					continue
				}
				if name == "" {
					name = f.Name
				}
				walk(rv.Field(i), name)
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i), jsonPath)
			}
		case reflect.String:
			if strings.ContainsRune(rv.String(), '\x00') {
				key := jsonPath
				if key == "" {
					key = "value"
				}
				out[key] = key + " cannot contain null byte (\\x00)"
			}
		default:
			// primitives: no-op
		}
	}

	rv := reflect.ValueOf(s)
	walk(rv, "")
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseMetadata handles metadata field parsing according to RFC 7396 JSON Merge Patch.
//
// This function implements special handling for the metadata field to support JSON Merge Patch
// semantics. If the metadata field is not present in the original JSON, it's initialized to
// an empty map rather than nil. This allows distinguishing between:
//   - Metadata not provided (empty map)
//   - Metadata explicitly set to null (nil)
//
// RFC 7396 JSON Merge Patch allows:
//   - Omitting a field: no change
//   - Setting a field to null: delete the field
//   - Setting a field to a value: update the field
//
// Parameters:
//   - s: Decoded struct (must be a pointer to a struct with a Metadata field)
//   - originalMap: Original JSON as a map
//
// Example:
//
//	// Request: {"name": "Account"}
//	// After parseMetadata: input.Metadata = map[string]any{}
//
//	// Request: {"name": "Account", "metadata": null}
//	// After parseMetadata: input.Metadata = nil
//
//	// Request: {"name": "Account", "metadata": {"key": "value"}}
//	// After parseMetadata: input.Metadata = map[string]any{"key": "value"}
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

// FindUnknownFields identifies fields in the original JSON that are not in the struct definition.
//
// This function performs a deep comparison between the original JSON (as a map) and the
// re-marshaled struct (as a map) to detect fields that were present in the JSON but not
// defined in the struct. This is used to enforce strict API contracts and prevent clients
// from sending unexpected data.
//
// The function handles:
//   - Nested maps (recursively checks nested objects)
//   - Arrays (compares array elements)
//   - Decimal values (special handling for numeric strings)
//   - Type mismatches (detects when JSON type differs from struct type)
//   - Zero values (ignores numeric zeros that weren't in original JSON)
//
// Parameters:
//   - original: Map representation of the original JSON request body
//   - marshaled: Map representation of the struct after unmarshaling and re-marshaling
//
// Returns:
//   - map[string]any: Map of unknown fields (empty if no unknown fields found)
//
// Example:
//
//	original := map[string]any{"name": "Account", "extra": "field"}
//	marshaled := map[string]any{"name": "Account"}
//	unknown := FindUnknownFields(original, marshaled)
//	// Returns: {"extra": "field"}
//
//nolint:gocognit
func FindUnknownFields(original, marshaled map[string]any) map[string]any {
	diffFields := make(map[string]any)

	numKinds := libCommons.GetMapNumKinds()

	for key, value := range original {
		if numKinds[reflect.ValueOf(value).Kind()] && value == 0.0 {
			continue
		}

		marshaledValue, ok := marshaled[key]
		if !ok {
			diffFields[key] = value
			continue
		}

		switch originalValue := value.(type) {
		case map[string]any:
			if marshaledMap, ok := marshaledValue.(map[string]any); ok {
				nestedDiff := FindUnknownFields(originalValue, marshaledMap)
				if len(nestedDiff) > 0 {
					diffFields[key] = nestedDiff
				}
			} else if !reflect.DeepEqual(originalValue, marshaledValue) {
				diffFields[key] = value
			}

		case []any:
			if marshaledArray, ok := marshaledValue.([]any); ok {
				arrayDiff := compareSlices(originalValue, marshaledArray)
				if len(arrayDiff) > 0 {
					diffFields[key] = arrayDiff
				}
			} else if !reflect.DeepEqual(originalValue, marshaledValue) {
				diffFields[key] = value
			}
		case string:
			if isStringNumeric(originalValue) {
				if isDecimalEqual(originalValue, marshaledValue) {
					continue
				}
			}

			if !reflect.DeepEqual(value, marshaledValue) {
				diffFields[key] = value
			}
		default:
			if !reflect.DeepEqual(value, marshaledValue) {
				diffFields[key] = value
			}
		}
	}

	return diffFields
}

func isDecimalEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}

	var decimalA, decimalB decimal.Decimal

	var err error

	switch valA := a.(type) {
	case string:
		decimalA, err = decimal.NewFromString(valA)
		if err != nil {
			return false
		}
	case decimal.Decimal:
		decimalA = valA
	default:
		return false
	}

	switch valB := b.(type) {
	case string:
		decimalB, err = decimal.NewFromString(valB)
		if err != nil {
			return false
		}
	case decimal.Decimal:
		decimalB = valB
	default:
		return false
	}

	return decimalA.Equal(decimalB)
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
		} else {
			tmpMarshaled := marshaled[i]
			// Compare individual items at the same index
			if originalMap, ok := item.(map[string]any); ok {
				if marshaledMap, ok := tmpMarshaled.(map[string]any); ok {
					nestedDiff := FindUnknownFields(originalMap, marshaledMap)
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
