// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// TestValidateStruct_CreatePackageFeesRequired asserts that a CreatePackageInput
// with an empty or missing fees map is rejected by the body validator. A package
// without fees has no billing behavior, so the fees map is required (min=1). All
// other fields are valid so the fees rule is the operative failure.
func TestValidateStruct_CreatePackageFeesRequired(t *testing.T) {
	t.Parallel()

	enable := true

	base := func() *model.CreatePackageInput {
		return &model.CreatePackageInput{
			FeeGroupLabel: "Test Package",
			LedgerID:      "00000000-0000-0000-0000-000000000001",
			MinAmount:     "100.00",
			MaxAmount:     "1000.00",
			Enable:        &enable,
		}
	}

	t.Run("empty fees map fails validation", func(t *testing.T) {
		t.Parallel()

		in := base()
		in.Fee = map[string]model.Fee{}

		if err := ValidateStruct(in); err == nil {
			t.Fatal("expected validation error for empty fees map, got nil")
		}
	})

	t.Run("missing fees map fails validation", func(t *testing.T) {
		t.Parallel()

		in := base() // Fee left nil

		if err := ValidateStruct(in); err == nil {
			t.Fatal("expected validation error for nil fees map, got nil")
		}
	})
}
