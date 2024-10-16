package http

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/LerianStudio/midaz/common"

	"github.com/gofiber/fiber/v2"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	en2 "github.com/go-playground/validator/translations/en"

	"gopkg.in/go-playground/validator.v9"
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
		return err
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

	// Generate a map that only contains fields that are present in the original payload but not recognized by the Go struct.
	diffFields := make(map[string]any)

	for key, value := range originalMap {
		if _, ok := marshaledMap[key]; !ok {
			diffFields[key] = value
		}
	}

	if len(diffFields) > 0 {
		err := common.ValidateBadRequestFieldsError(common.FieldValidations{}, common.FieldValidations{}, "", diffFields)
		return BadRequest(c, err)
	}

	if err := ValidateStruct(s); err != nil {
		return BadRequest(c, err)
	}

	c.Locals(string(PayloadContextValue("fields")), diffFields)

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
		errPtr := malformedRequestErr(err.(validator.ValidationErrors), trans)
		return &errPtr
	}

	return nil
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

	return v, trans
}

func malformedRequestErr(err validator.ValidationErrors, trans ut.Translator) common.ValidationKnownFieldsError {
	invalidFieldsMap := fields(err, trans)

	var requiredFields = fieldsRequired(invalidFieldsMap)

	var vErr common.ValidationKnownFieldsError
	_ = errors.As(common.ValidateBadRequestFieldsError(requiredFields, invalidFieldsMap, "", make(map[string]any)), &vErr)

	return vErr
}

func fields(errs validator.ValidationErrors, trans ut.Translator) common.FieldValidations {
	l := len(errs)
	if l > 0 {
		fields := make(common.FieldValidations, l)
		for _, e := range errs {
			fields[e.Field()] = e.Translate(trans)
		}

		return fields
	}

	return nil
}

func fieldsRequired(myMap common.FieldValidations) common.FieldValidations {
	result := make(common.FieldValidations)

	for key, value := range myMap {
		if strings.Contains(value, "required") {
			result[key] = value
		}
	}

	return result
}
