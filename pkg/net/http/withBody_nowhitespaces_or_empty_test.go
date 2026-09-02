// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
)

// clearableKeyProbe is a minimal stand-in for a PATCH input carrying an optional,
// clearable string field. It is declared here rather than reusing a published mmodel
// type so this test pins the TAG's behaviour and cannot be invalidated by a future
// change to any particular DTO.
type clearableKeyProbe struct {
	Key *string `json:"balanceKey,omitempty" validate:"omitempty,nowhitespacesorempty,max=100"`
}

// strictKeyProbe is the same field under the pre-existing nowhitespaces tag. It is the
// control: it proves the two tags differ ONLY on the empty string, so introducing
// nowhitespacesorempty changed nothing for the fields already using nowhitespaces.
type strictKeyProbe struct {
	Key *string `json:"balanceKey,omitempty" validate:"omitempty,nowhitespaces,max=100"`
}

// TestValidateStruct_NoWhitespacesOrEmpty pins the tag registered for PATCH inputs that
// use "" as an explicit clear sentinel: the empty string passes, every other
// whitespace-bearing value is still rejected, and the length bound still applies.
//
// The nil case matters on its own: omitempty skips a nil pointer, but does NOT skip a
// non-nil pointer to "" (go-playground's hasValue returns true for any non-nil pointer),
// which is the whole reason this tag exists.
func TestValidateStruct_NoWhitespacesOrEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     *string
		wantErr bool
	}{
		{name: "absent key is skipped by omitempty", key: nil, wantErr: false},
		{name: "empty string is accepted as the clear sentinel", key: ptr(""), wantErr: false},
		{name: "ordinary key is accepted", key: ptr("asset-freeze"), wantErr: false},
		{name: "key at exactly 100 chars is accepted", key: ptr(strings.Repeat("k", 100)), wantErr: false},
		{name: "key over 100 chars is rejected", key: ptr(strings.Repeat("k", 101)), wantErr: true},
		{name: "key containing a space is rejected", key: ptr("asset freeze"), wantErr: true},
		{name: "key containing a tab is rejected", key: ptr("asset\tfreeze"), wantErr: true},
		{name: "key containing a newline is rejected", key: ptr("asset\nfreeze"), wantErr: true},
		{name: "whitespace-only key is rejected", key: ptr("   "), wantErr: true},
		{name: "leading whitespace is rejected", key: ptr(" asset-freeze"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateStruct(&clearableKeyProbe{Key: tt.key})

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

// TestValidateStruct_NoWhitespacesTagUnchanged is the regression guard on the existing
// tag: nowhitespaces MUST keep rejecting the empty string, so no field that already uses
// it silently loosened when nowhitespacesorempty was registered alongside it.
func TestValidateStruct_NoWhitespacesTagUnchanged(t *testing.T) {
	t.Parallel()

	require.Error(t, ValidateStruct(&strictKeyProbe{Key: ptr("")}),
		"nowhitespaces must still reject the empty string")
	require.Error(t, ValidateStruct(&strictKeyProbe{Key: ptr("asset freeze")}),
		"nowhitespaces must still reject an embedded space")
	require.NoError(t, ValidateStruct(&strictKeyProbe{Key: ptr("asset-freeze")}),
		"nowhitespaces must still accept an ordinary key")
	require.NoError(t, ValidateStruct(&strictKeyProbe{Key: nil}),
		"omitempty must still skip a nil pointer")
}

// TestValidateStruct_NoWhitespacesOrEmptyMessage asserts the rejection carries a
// TRANSLATED, field-named message rather than go-playground's raw fallback, which leaks
// the Go struct name into a client-facing 400 body.
func TestValidateStruct_NoWhitespacesOrEmptyMessage(t *testing.T) {
	t.Parallel()

	err := ValidateStruct(&clearableKeyProbe{Key: ptr("asset freeze")})
	require.Error(t, err)

	knownFields, ok := err.(*pkg.ValidationKnownFieldsError)
	require.Truef(t, ok, "expected *pkg.ValidationKnownFieldsError, got %T", err)

	message, present := knownFields.Fields["balanceKey"]
	require.True(t, present, "the rejected field must be reported under its json name")
	require.Equal(t, "balanceKey cannot contain whitespaces", message)
}
