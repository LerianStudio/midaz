// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	libHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"

	"github.com/stretchr/testify/require"
)

// TestAccountTypeDefaultDirection exercises the real production validation path
// (http.ValidateStruct) over the create and update inputs for the account type
// defaultDirection field. It asserts credit/debit are accepted, absent is
// accepted, and an arbitrary value is rejected with ErrInvalidAccountTypeDirection.
func TestAccountTypeDefaultDirection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		direction *string
		wantErr   bool
	}{
		{name: "accepts credit", direction: testutils.Ptr(cn.DirectionCredit), wantErr: false},
		{name: "accepts debit", direction: testutils.Ptr(cn.DirectionDebit), wantErr: false},
		{name: "accepts absent (nil pointer)", direction: nil, wantErr: false},
		{name: "rejects arbitrary value", direction: testutils.Ptr("sideways"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run("create/"+tt.name, func(t *testing.T) {
			t.Parallel()

			input := mmodel.CreateAccountTypeInput{
				Name:             "Current Assets",
				KeyValue:         "current_assets",
				DefaultDirection: tt.direction,
			}

			err := libHTTP.ValidateStruct(&input)
			assertDirectionErr(t, err, tt.wantErr)
		})

		t.Run("update/"+tt.name, func(t *testing.T) {
			t.Parallel()

			input := mmodel.UpdateAccountTypeInput{
				Name:             "Current Assets",
				DefaultDirection: tt.direction,
			}

			err := libHTTP.ValidateStruct(&input)
			assertDirectionErr(t, err, tt.wantErr)
		})
	}
}

// assertDirectionErr checks the validation outcome: on wantErr it requires a
// ValidationError carrying the ErrInvalidAccountTypeDirection code; otherwise it
// requires no error.
func assertDirectionErr(t *testing.T, err error, wantErr bool) {
	t.Helper()

	if !wantErr {
		require.NoError(t, err)
		return
	}

	require.Error(t, err)

	var vErr pkg.ValidationError

	require.True(t, errors.As(err, &vErr), "expected pkg.ValidationError, got %T", err)
	require.Equal(t, cn.ErrInvalidAccountTypeDirection.Error(), vErr.Code)
	require.NotEmpty(t, vErr.Message, "ValidationError.Message must not be blank")
	require.Contains(t, vErr.Message, cn.DirectionCredit, "message should mention the allowed direction values")
}

// TestAccountTypeDefaultDirectionJSONTag asserts the JSON tag on the
// defaultDirection field is exactly "defaultDirection" (camelCase) across the
// domain, create-input and update-input structs.
func TestAccountTypeDefaultDirectionJSONTag(t *testing.T) {
	t.Parallel()

	const wantTag = "defaultDirection"

	types := []struct {
		name string
		typ  reflect.Type
	}{
		{name: "AccountType", typ: reflect.TypeOf(mmodel.AccountType{})},
		{name: "CreateAccountTypeInput", typ: reflect.TypeOf(mmodel.CreateAccountTypeInput{})},
		{name: "UpdateAccountTypeInput", typ: reflect.TypeOf(mmodel.UpdateAccountTypeInput{})},
	}

	for _, tc := range types {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			field, ok := tc.typ.FieldByName("DefaultDirection")
			require.True(t, ok, "%s must have a DefaultDirection field", tc.name)

			jsonName := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
			require.Equal(t, wantTag, jsonName, "%s.DefaultDirection json tag", tc.name)
		})
	}
}

// TestAccountTypeDefaultDirectionMarshal confirms the domain struct marshals the
// field under the camelCase key when set.
func TestAccountTypeDefaultDirectionMarshal(t *testing.T) {
	t.Parallel()

	at := mmodel.AccountType{DefaultDirection: cn.DirectionCredit}

	b, err := json.Marshal(at)
	require.NoError(t, err)

	var round map[string]any

	require.NoError(t, json.Unmarshal(b, &round))
	require.Equal(t, cn.DirectionCredit, round["defaultDirection"])
}
