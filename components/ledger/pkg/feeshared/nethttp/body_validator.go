// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"

	modelTransaction "github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	en2 "github.com/go-playground/validator/v10/translations/en"
)

var (
	validatorInstance  *validator.Validate
	translatorInstance ut.Translator
	validatorOnce      sync.Once
	validatorInitErr   error
)

// ValidateStruct validates a struct against defined validation rules, using the validator pack.
func ValidateStruct(s any) error {
	v, trans := newValidator()

	// If validator initialization failed, reject the request to prevent unvalidated input
	// TODO(review): Redundant nil check - fail-secure at line 407 already returns - ring:code-reviewer on 2026-02-21
	if v == nil {
		return pkg.ValidateInternalError(validatorInitErr, "validator")
	}

	k := reflect.ValueOf(s).Kind()
	if k == reflect.Ptr {
		k = reflect.ValueOf(s).Elem().Kind()
	}

	if k != reflect.Struct {
		return nil
	}

	err := v.Struct(s)
	if err != nil {
		validationErrors, ok := err.(validator.ValidationErrors)
		if !ok {
			return pkg.ValidateInternalError(err, "validator")
		}

		for _, fieldError := range validationErrors {
			if fieldError.Tag() == "singletransactiontype" {
				return pkg.ValidateBusinessError(constant.ErrInvalidTransactionType, "", fieldError.Translate(trans))
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

func malformedRequestErr(err validator.ValidationErrors, trans ut.Translator) pkg.ValidationKnownFieldsError {
	invalidFieldsMap := fields(err, trans)

	requiredFields := fieldsRequired(invalidFieldsMap)

	var vErr pkg.ValidationKnownFieldsError

	_ = errors.As(pkg.ValidateBadRequestFieldsError(requiredFields, invalidFieldsMap, "", make(map[string]any)), &vErr)

	return vErr
}

// initValidator initializes the validator singleton. Called once via sync.Once.
//
//nolint:gocyclo // Validator initialization registers many custom validations sequentially
func initValidator() {
	locale := en.New()
	uni := ut.New(locale, locale)

	trans, _ := uni.GetTranslator("en")

	v := validator.New()

	if err := en2.RegisterDefaultTranslations(v, trans); err != nil {
		validatorInitErr = err

		return
	}

	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}

		return name
	})

	var registrationErrors []error

	if err := v.RegisterValidation("keymax", validateMetadataKeyMaxLength); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterValidation("nonested", validateMetadataNestedValues); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterValidation("valuemax", validateMetadataValueMaxLength); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterValidation("singletransactiontype", validateSingleTransactionType); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterValidation("uuid", validateUUID); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterTranslation("required", trans, func(ut ut.Translator) error {
		return ut.Add("required", "{0} is a required field", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("required", formatErrorFieldName(fe.Namespace()))
		return t
	}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterTranslation("gte", trans, func(ut ut.Translator) error {
		return ut.Add("gte", "{0} must be {1} or greater", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("gte", formatErrorFieldName(fe.Namespace()), fe.Param())
		return t
	}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterTranslation("eq", trans, func(ut ut.Translator) error {
		return ut.Add("eq", "{0} is not equal to {1}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("eq", formatErrorFieldName(fe.Namespace()), fe.Param())
		return t
	}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterTranslation("keymax", trans, func(ut ut.Translator) error {
		return ut.Add("keymax", "{0}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("keymax", formatErrorFieldName(fe.Namespace()))
		return t
	}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterTranslation("valuemax", trans, func(ut ut.Translator) error {
		return ut.Add("valuemax", "{0}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("valuemax", formatErrorFieldName(fe.Namespace()))
		return t
	}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterTranslation("nonested", trans, func(ut ut.Translator) error {
		return ut.Add("nonested", "{0}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("nonested", formatErrorFieldName(fe.Namespace()))
		return t
	}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterTranslation("singletransactiontype", trans, func(ut ut.Translator) error {
		return ut.Add("singletransactiontype", "{0}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("singletransactiontype", formatErrorFieldName(fe.Namespace()))
		return t
	}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if err := v.RegisterTranslation("uuid", trans, func(ut ut.Translator) error {
		return ut.Add("uuid", "{0} must be a valid UUID", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("uuid", formatErrorFieldName(fe.Namespace()))
		return t
	}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}

	if len(registrationErrors) > 0 {
		validatorInitErr = errors.Join(registrationErrors...)

		return
	}

	validatorInstance = v
	translatorInstance = trans
}

//nolint:ireturn
func newValidator() (*validator.Validate, ut.Translator) {
	validatorOnce.Do(initValidator)

	if validatorInitErr != nil {
		return nil, nil
	}

	return validatorInstance, translatorInstance
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

	return utf8.RuneCountInString(fl.Field().String()) <= limit
}

// validateSingleTransactionType checks if a transaction has only one type of transaction (amount, share, or remaining)
func validateSingleTransactionType(fl validator.FieldLevel) bool {
	arrField, ok := fl.Field().Interface().([]modelTransaction.FromTo)
	if !ok {
		return false
	}

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

	return utf8.RuneCountInString(value) <= limit
}

// formatErrorFieldName formats metadata field error names for error messages
// TODO(review): Pre-compile regex as package-level var for performance - ring:code-reviewer on 2026-02-21
// TODO(review): Evaluate regex for ReDoS risk with untrusted input - ring:security-reviewer on 2026-02-21
func formatErrorFieldName(text string) string {
	re := regexp.MustCompile(`\.(.+)$`)

	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	} else {
		return text
	}
}

// validateUUID validates if a string is a valid UUID
func validateUUID(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value == "" {
		return true
	}

	_, err := uuid.Parse(value)

	return err == nil
}
