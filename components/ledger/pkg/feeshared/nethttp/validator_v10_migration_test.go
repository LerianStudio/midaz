// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"reflect"
	"testing"

	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testFeeCalculate mirrors testFeeCalculate to avoid import cycle (model -> http)
type testFeeCalculate struct {
	SegmentID   *uuid.UUID              `json:"segmentId"`
	LedgerID    uuid.UUID               `json:"ledgerId" validate:"required"`
	Transaction transaction.Transaction `json:"transaction"`
}

// testFeeEstimate mirrors testFeeEstimate to avoid import cycle
type testFeeEstimate struct {
	PackageID   uuid.UUID               `json:"packageId" validate:"required"`
	LedgerID    uuid.UUID               `json:"ledgerId" validate:"required"`
	Transaction transaction.Transaction `json:"transaction"`
}

// ---------------------------------------------------------------------------
// AC-1: All validator imports use v10 — no v9 imports remain
// ---------------------------------------------------------------------------

func TestValidatorV10_ImportVersion_IsV10(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "validator.New returns v10 instance",
			check: func(t *testing.T) {
				t.Parallel()
				v := validator.New()
				require.NotNil(t, v)

				// v10 types live in "github.com/go-playground/validator/v10"
				typeName := reflect.TypeOf(v).String()
				assert.Equal(t, "*validator.Validate", typeName,
					"validator.New() should return *validator.Validate from v10")
			},
		},
		{
			name: "validator package path contains v10",
			check: func(t *testing.T) {
				t.Parallel()
				v := validator.New()
				pkgPath := reflect.TypeOf(v).Elem().PkgPath()
				assert.Contains(t, pkgPath, "validator/v10",
					"validator package path must contain v10")
			},
		},
		{
			name: "newValidator returns v10 singleton",
			check: func(t *testing.T) {
				t.Parallel()
				v, trans := newValidator()
				require.NotNil(t, v, "validator singleton must not be nil")
				require.NotNil(t, trans, "translator must not be nil")

				pkgPath := reflect.TypeOf(v).Elem().PkgPath()
				assert.Contains(t, pkgPath, "validator/v10",
					"newValidator must return a v10 validator")
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t)
		})
	}
}

// ---------------------------------------------------------------------------
// AC-2: Validation behavior is preserved — same validation rules work with v10
// ---------------------------------------------------------------------------

func TestValidatorV10_ValidationRules_Preserved(t *testing.T) {
	t.Parallel()

	v, _ := newValidator()
	require.NotNil(t, v)

	tests := []struct {
		name      string
		value     any
		tag       string
		wantValid bool
	}{
		// required
		{name: "required_valid_string", value: "hello", tag: "required", wantValid: true},
		{name: "required_empty_string", value: "", tag: "required", wantValid: false},
		{name: "required_zero_int", value: 0, tag: "required", wantValid: false},
		{name: "required_nonzero_int", value: 42, tag: "required", wantValid: true},

		// gt (greater than)
		{name: "gt_valid", value: 10, tag: "gt=5", wantValid: true},
		{name: "gt_equal", value: 5, tag: "gt=5", wantValid: false},
		{name: "gt_less", value: 3, tag: "gt=5", wantValid: false},
		{name: "gt_zero", value: 0, tag: "gt=0", wantValid: false},

		// gte (greater than or equal)
		{name: "gte_valid", value: 10, tag: "gte=5", wantValid: true},
		{name: "gte_equal", value: 5, tag: "gte=5", wantValid: true},
		{name: "gte_less", value: 3, tag: "gte=5", wantValid: false},
		{name: "gte_zero", value: 0, tag: "gte=0", wantValid: true},
		{name: "gte_negative", value: -1, tag: "gte=0", wantValid: false},

		// min (minimum length for strings)
		{name: "min_valid", value: "hello", tag: "min=3", wantValid: true},
		{name: "min_exact", value: "abc", tag: "min=3", wantValid: true},
		{name: "min_less", value: "ab", tag: "min=3", wantValid: false},
		{name: "min_empty", value: "", tag: "min=1", wantValid: false},

		// max (maximum length for strings)
		{name: "max_valid", value: "hi", tag: "max=5", wantValid: true},
		{name: "max_exact", value: "hello", tag: "max=5", wantValid: true},
		{name: "max_over", value: "toolong", tag: "max=5", wantValid: false},
		{name: "max_empty", value: "", tag: "max=5", wantValid: true},

		// len (exact length)
		{name: "len_valid", value: "abc", tag: "len=3", wantValid: true},
		{name: "len_short", value: "ab", tag: "len=3", wantValid: false},
		{name: "len_long", value: "abcd", tag: "len=3", wantValid: false},
		{name: "len_empty", value: "", tag: "len=0", wantValid: true},

		// oneof
		{name: "oneof_valid", value: "red", tag: "oneof=red green blue", wantValid: true},
		{name: "oneof_invalid", value: "yellow", tag: "oneof=red green blue", wantValid: false},
		{name: "oneof_empty", value: "", tag: "oneof=red green blue", wantValid: false},

		// email
		{name: "email_valid", value: "test@example.com", tag: "email", wantValid: true},
		{name: "email_invalid", value: "not-email", tag: "email", wantValid: false},
		{name: "email_empty", value: "", tag: "email", wantValid: false},
		{name: "email_at_only", value: "@", tag: "email", wantValid: false},
		{name: "email_no_domain", value: "user@", tag: "email", wantValid: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := v.Var(tt.value, tt.tag)
			if tt.wantValid {
				assert.NoError(t, err,
					"expected value %v to pass tag %q", tt.value, tt.tag)
			} else {
				assert.Error(t, err,
					"expected value %v to fail tag %q", tt.value, tt.tag)
			}
		})
	}
}

func TestValidatorV10_CustomValidators_Preserved(t *testing.T) {
	t.Parallel()

	v, _ := newValidator()
	require.NotNil(t, v)

	tests := []struct {
		name      string
		value     any
		tag       string
		wantValid bool
	}{
		// uuid custom validator
		{name: "uuid_valid", value: uuid.New().String(), tag: "uuid", wantValid: true},
		{name: "uuid_invalid", value: "not-a-uuid", tag: "uuid", wantValid: false},
		{name: "uuid_empty", value: "", tag: "uuid", wantValid: true},
		{name: "uuid_partial", value: "00000000-0000-0000", tag: "uuid", wantValid: false},
		{name: "uuid_zeroed", value: "00000000-0000-0000-0000-000000000000", tag: "uuid", wantValid: true},

		// keymax custom validator
		{name: "keymax_within_limit", value: "short", tag: "keymax=10", wantValid: true},
		{name: "keymax_at_limit", value: "1234567890", tag: "keymax=10", wantValid: true},
		{name: "keymax_over_limit", value: "12345678901", tag: "keymax=10", wantValid: false},
		{name: "keymax_empty", value: "", tag: "keymax=10", wantValid: true},

		// valuemax custom validator
		{name: "valuemax_within_limit", value: "test", tag: "valuemax=10", wantValid: true},
		{name: "valuemax_at_limit", value: "1234567890", tag: "valuemax=10", wantValid: true},
		{name: "valuemax_over_limit", value: "12345678901", tag: "valuemax=10", wantValid: false},
		{name: "valuemax_empty", value: "", tag: "valuemax=10", wantValid: true},

		// nonested custom validator
		{name: "nonested_string", value: "value", tag: "nonested", wantValid: true},
		{name: "nonested_int", value: 42, tag: "nonested", wantValid: true},
		{name: "nonested_map", value: map[string]any{"k": "v"}, tag: "nonested", wantValid: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := v.Var(tt.value, tt.tag)
			if tt.wantValid {
				assert.NoError(t, err,
					"expected value %v to pass tag %q", tt.value, tt.tag)
			} else {
				assert.Error(t, err,
					"expected value %v to fail tag %q", tt.value, tt.tag)
			}
		})
	}
}

func TestValidatorV10_SingleTransactionType_Preserved(t *testing.T) {
	t.Parallel()

	v, _ := newValidator()
	require.NotNil(t, v)

	type singleTypeStruct struct {
		FromTo []transaction.FromTo `validate:"singletransactiontype"`
	}

	tests := []struct {
		name      string
		fromTo    []transaction.FromTo
		wantValid bool
	}{
		{
			name: "only_amount",
			fromTo: []transaction.FromTo{
				{Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)}},
			},
			wantValid: true,
		},
		{
			name: "only_share",
			fromTo: []transaction.FromTo{
				{Share: &transaction.Share{Percentage: 50}},
			},
			wantValid: true,
		},
		{
			name: "only_remaining",
			fromTo: []transaction.FromTo{
				{Remaining: "remaining"},
			},
			wantValid: true,
		},
		{
			name: "amount_and_share_invalid",
			fromTo: []transaction.FromTo{
				{
					Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
					Share:  &transaction.Share{Percentage: 50},
				},
			},
			wantValid: false,
		},
		{
			name: "all_three_invalid",
			fromTo: []transaction.FromTo{
				{
					Amount:    &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
					Share:     &transaction.Share{Percentage: 50},
					Remaining: "remaining",
				},
			},
			wantValid: false,
		},
		{
			name:      "none_specified_invalid",
			fromTo:    []transaction.FromTo{{}},
			wantValid: false,
		},
		{
			name:      "empty_slice",
			fromTo:    []transaction.FromTo{},
			wantValid: true,
		},
		{
			name: "multiple_items_all_valid",
			fromTo: []transaction.FromTo{
				{Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)}},
				{Share: &transaction.Share{Percentage: 50}},
			},
			wantValid: true,
		},
		{
			name: "multiple_items_one_invalid",
			fromTo: []transaction.FromTo{
				{Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)}},
				{
					Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(50)},
					Share:  &transaction.Share{Percentage: 25},
				},
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := singleTypeStruct{FromTo: tt.fromTo}
			err := v.Struct(s)
			if tt.wantValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AC-3: No incompatible v9 tags cause panics — dive on non-slice/map must not panic
// ---------------------------------------------------------------------------

func TestValidatorV10_DiveTag_NoPanicOnNonSliceMap(t *testing.T) {
	t.Parallel()

	v := validator.New()

	tests := []struct {
		name   string
		value  any
		panics bool
	}{
		{
			name: "dive_on_slice_no_panic",
			value: struct {
				Items []string `validate:"dive,required"`
			}{Items: []string{"a", "b"}},
			panics: false,
		},
		{
			name: "dive_on_map_no_panic",
			value: struct {
				Data map[string]string `validate:"dive,keys,required,endkeys,required"`
			}{Data: map[string]string{"key": "val"}},
			panics: false,
		},
		{
			name: "dive_on_empty_slice_no_panic",
			value: struct {
				Items []string `validate:"dive,required"`
			}{Items: []string{}},
			panics: false,
		},
		{
			name: "dive_on_nil_slice_no_panic",
			value: struct {
				Items []string `validate:"dive,required"`
			}{Items: nil},
			panics: false,
		},
		{
			name: "dive_on_nil_map_no_panic",
			value: struct {
				Data map[string]string `validate:"dive,keys,required,endkeys,required"`
			}{Data: nil},
			panics: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.NotPanics(t, func() {
				_ = v.Struct(tt.value)
			}, "validator.Struct must not panic")
		})
	}
}

func TestValidatorV10_DiveTag_OnStructFieldPanicsInV10(t *testing.T) {
	t.Parallel()

	// This test documents the v10 behavior change:
	// In v9, dive on a struct field was silently ignored.
	// In v10, dive on a non-slice/map type PANICS.
	// The migration removed dive from Transaction.Send and Transaction.Respond fields.
	v := validator.New()

	type innerStruct struct {
		Name string `validate:"required"`
	}

	type structWithDiveOnStruct struct {
		Inner innerStruct `validate:"dive"`
	}

	// v10 panics when dive is used on a struct (non-slice/map) field
	assert.Panics(t, func() {
		_ = v.Struct(structWithDiveOnStruct{Inner: innerStruct{Name: "test"}})
	}, "v10 should panic when dive is applied to a struct field")
}

func TestValidatorV10_FeesModels_NoDiveOnStructFields(t *testing.T) {
	t.Parallel()

	// Verify that FeeCalculate and FeeEstimate models with Transaction
	// can be validated without panicking, proving dive tags were properly removed
	// from the Send field in the Transaction struct embedded in fees.go models.
	v, _ := newValidator()
	require.NotNil(t, v)

	tests := []struct {
		name  string
		value any
	}{
		{
			name: "FeeCalculate_with_valid_transaction_no_panic",
			value: &testFeeCalculate{
				LedgerID: uuid.New(),
				Transaction: transaction.Transaction{
					Send: transaction.Send{
						Asset: "BRL",
						Value: decimal.NewFromInt(100),
						Source: transaction.Source{
							From: []transaction.FromTo{
								{
									AccountAlias: "@test",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
								},
							},
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{
								{
									AccountAlias: "@dest",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "FeeEstimate_with_valid_transaction_no_panic",
			value: &testFeeEstimate{
				PackageID: uuid.New(),
				LedgerID:  uuid.New(),
				Transaction: transaction.Transaction{
					Send: transaction.Send{
						Asset: "BRL",
						Value: decimal.NewFromInt(200),
						Source: transaction.Source{
							From: []transaction.FromTo{
								{
									AccountAlias: "@src",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(200)},
								},
							},
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{
								{
									AccountAlias: "@dst",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(200)},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "FeeCalculate_empty_transaction_no_panic",
			value: &testFeeCalculate{
				LedgerID:    uuid.New(),
				Transaction: transaction.Transaction{},
			},
		},
		{
			name: "FeeCalculate_nil_segment_no_panic",
			value: &testFeeCalculate{
				SegmentID:   nil,
				LedgerID:    uuid.New(),
				Transaction: transaction.Transaction{},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.NotPanics(t, func() {
				_ = v.Struct(tt.value)
			}, "validating %s must not panic (no dive on struct fields)", tt.name)
		})
	}
}

// ---------------------------------------------------------------------------
// AC-4: Struct validation works end-to-end — Transaction and model structs
// ---------------------------------------------------------------------------

func TestValidatorV10_StructValidation_EndToEnd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{
			name: "FeeCalculate_valid_full",
			input: &testFeeCalculate{
				LedgerID: uuid.New(),
				Transaction: transaction.Transaction{
					Send: transaction.Send{
						Asset: "BRL",
						Value: decimal.NewFromInt(1000),
						Source: transaction.Source{
							From: []transaction.FromTo{
								{
									AccountAlias: "@user1",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
								},
							},
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{
								{
									AccountAlias: "@user2",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "FeeCalculate_missing_ledgerID",
			input: &testFeeCalculate{
				LedgerID:    uuid.Nil,
				Transaction: transaction.Transaction{},
			},
			wantErr: true,
		},
		{
			name: "FeeEstimate_missing_packageID",
			input: &testFeeEstimate{
				PackageID:   uuid.Nil,
				LedgerID:    uuid.New(),
				Transaction: transaction.Transaction{},
			},
			wantErr: true,
		},
		{
			name: "FeeEstimate_valid",
			input: &testFeeEstimate{
				PackageID: uuid.New(),
				LedgerID:  uuid.New(),
				Transaction: transaction.Transaction{
					Send: transaction.Send{
						Asset: "USD",
						Value: decimal.NewFromInt(500),
						Source: transaction.Source{
							From: []transaction.FromTo{
								{
									AccountAlias: "@src",
									Amount:       &transaction.Amount{Asset: "USD", Value: decimal.NewFromInt(500)},
								},
							},
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{
								{
									AccountAlias: "@dst",
									Amount:       &transaction.Amount{Asset: "USD", Value: decimal.NewFromInt(500)},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "FeeEstimate_missing_send_asset",
			input: &testFeeEstimate{
				LedgerID: uuid.New(),
				Transaction: transaction.Transaction{
					Send: transaction.Send{
						Asset: "", // required but empty
						Value: decimal.NewFromInt(100),
						Source: transaction.Source{
							From: []transaction.FromTo{
								{
									AccountAlias: "@src",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
								},
							},
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{
								{
									AccountAlias: "@dst",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "FeeCalculate_with_metadata",
			input: &testFeeCalculate{
				LedgerID: uuid.New(),
				Transaction: transaction.Transaction{
					Metadata: map[string]any{"key": "value"},
					Send: transaction.Send{
						Asset: "BRL",
						Value: decimal.NewFromInt(100),
						Source: transaction.Source{
							From: []transaction.FromTo{
								{
									AccountAlias: "@src",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
								},
							},
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{
								{
									AccountAlias: "@dst",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "FeeCalculate_with_optional_segmentID",
			input: &testFeeCalculate{
				SegmentID: func() *uuid.UUID { u := uuid.New(); return &u }(),
				LedgerID:  uuid.New(),
				Transaction: transaction.Transaction{
					Send: transaction.Send{
						Asset: "EUR",
						Value: decimal.NewFromInt(250),
						Source: transaction.Source{
							From: []transaction.FromTo{
								{
									AccountAlias: "@src",
									Amount:       &transaction.Amount{Asset: "EUR", Value: decimal.NewFromInt(250)},
								},
							},
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{
								{
									AccountAlias: "@dst",
									Amount:       &transaction.Amount{Asset: "EUR", Value: decimal.NewFromInt(250)},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "FeeCalculate_empty_source_from_passes_v10",
			input: &testFeeCalculate{
				LedgerID: uuid.New(),
				Transaction: transaction.Transaction{
					Send: transaction.Send{
						Asset: "BRL",
						Value: decimal.NewFromInt(100),
						Source: transaction.Source{
							From: []transaction.FromTo{},
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{
								{
									AccountAlias: "@dst",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
								},
							},
						},
					},
				},
			},
			wantErr: false, // v10: empty slice is non-nil, passes required; singletransactiontype returns true for empty
		},
		{
			name: "FeeCalculate_nil_source_from",
			input: &testFeeCalculate{
				LedgerID: uuid.New(),
				Transaction: transaction.Transaction{
					Send: transaction.Send{
						Asset: "BRL",
						Value: decimal.NewFromInt(100),
						Source: transaction.Source{
							From: nil,
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{
								{
									AccountAlias: "@dst",
									Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
								},
							},
						},
					},
				},
			},
			wantErr: true, // nil slice fails required
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// First: must not panic (AC-3 related)
			var err error
			assert.NotPanics(t, func() {
				err = ValidateStruct(tt.input)
			}, "ValidateStruct must not panic")

			if tt.wantErr {
				assert.Error(t, err,
					"expected validation error for %s", tt.name)
			} else {
				assert.NoError(t, err,
					"expected no validation error for %s", tt.name)
			}
		})
	}
}

func TestValidatorV10_ValidateStruct_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{
			name:    "nil_pointer",
			input:   (*testFeeCalculate)(nil),
			wantErr: false, // non-struct returns nil
		},
		{
			name:    "non_struct_string",
			input:   "not a struct",
			wantErr: false,
		},
		{
			name:    "non_struct_int",
			input:   42,
			wantErr: false,
		},
		{
			name: "pointer_to_valid_struct",
			input: &testStructWithValidation{
				Name:  "test",
				Email: "test@example.com",
				Age:   25,
			},
			wantErr: false,
		},
		{
			name: "pointer_to_invalid_struct",
			input: &testStructWithValidation{
				Name:  "",
				Email: "bad",
				Age:   -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var err error
			assert.NotPanics(t, func() {
				err = ValidateStruct(tt.input)
			})
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatorV10_TranslationErrors_V10Format(t *testing.T) {
	t.Parallel()

	// Verify that v10 translation registrations work correctly
	v, trans := newValidator()
	require.NotNil(t, v)
	require.NotNil(t, trans)

	tests := []struct {
		name         string
		value        any
		expectFields []string
	}{
		{
			name: "required_field_translation",
			value: &testStructWithValidation{
				Name:  "",
				Email: "test@example.com",
				Age:   0,
			},
			expectFields: []string{"name"},
		},
		{
			name: "email_field_translation",
			value: &testStructWithValidation{
				Name:  "test",
				Email: "invalid",
				Age:   0,
			},
			expectFields: []string{"email"},
		},
		{
			name: "gte_field_translation",
			value: &testStructWithValidation{
				Name:  "test",
				Email: "test@example.com",
				Age:   -1,
			},
			expectFields: []string{"age"},
		},
		{
			name: "multiple_errors_translation",
			value: &testStructWithValidation{
				Name:  "",
				Email: "bad",
				Age:   -1,
			},
			expectFields: []string{"name", "email", "age"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := v.Struct(tt.value)
			require.Error(t, err)

			validationErrors, ok := err.(validator.ValidationErrors)
			require.True(t, ok, "error should be ValidationErrors type")

			fieldMap := fields(validationErrors, trans)
			require.NotNil(t, fieldMap)

			for _, expectedField := range tt.expectFields {
				assert.Contains(t, fieldMap, expectedField,
					"translation should contain field %q", expectedField)
				msg, exists := fieldMap[expectedField]
				if exists {
					assert.NotEmpty(t, msg,
						"translation for field %q should not be empty", expectedField)
				}
			}
		})
	}
}
