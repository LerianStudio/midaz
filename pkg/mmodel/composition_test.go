// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// holderIDFieldName is the single CreateAccountInput field the composite does
// NOT mirror: the holder is path-sourced, never body-supplied.
const holderIDFieldName = "HolderID"

// TestCompositionMirrorsCreateAccountInput is the F4-local-1 field-drift guard.
// It reflects over CreateAccountInput and asserts every field (except the
// path-sourced HolderID) is mirrored on CreateHolderAccountInput with an
// identical type and JSON tag. A future field added to CreateAccountInput will
// break this test instead of being silently dropped from the composite wire.
func TestCompositionMirrorsCreateAccountInput(t *testing.T) {
	t.Parallel()

	accountType := reflect.TypeOf(CreateAccountInput{})
	compositeType := reflect.TypeOf(CreateHolderAccountInput{})

	compositeFields := make(map[string]reflect.StructField, compositeType.NumField())
	for i := 0; i < compositeType.NumField(); i++ {
		f := compositeType.Field(i)
		compositeFields[f.Name] = f
	}

	sawHolderID := false

	for i := 0; i < accountType.NumField(); i++ {
		af := accountType.Field(i)

		if af.Name == holderIDFieldName {
			sawHolderID = true

			_, present := compositeFields[holderIDFieldName]
			assert.False(t, present,
				"CreateHolderAccountInput must NOT carry HolderID: the holder is path-sourced, not body-supplied")

			continue
		}

		cf, present := compositeFields[af.Name]
		assert.Truef(t, present,
			"CreateAccountInput.%s is not mirrored on CreateHolderAccountInput (field drift: add it or update the guard)", af.Name)

		if !present {
			continue
		}

		assert.Equalf(t, af.Type, cf.Type,
			"CreateHolderAccountInput.%s type %s diverged from CreateAccountInput.%s type %s",
			cf.Name, cf.Type, af.Name, af.Type)
		assert.Equalf(t, af.Tag.Get("json"), cf.Tag.Get("json"),
			"CreateHolderAccountInput.%s json tag diverged from CreateAccountInput.%s", cf.Name, af.Name)
		assert.Equalf(t, af.Tag.Get("validate"), cf.Tag.Get("validate"),
			"CreateHolderAccountInput.%s validate tag diverged from CreateAccountInput.%s", cf.Name, af.Name)
	}

	assert.True(t, sawHolderID,
		"CreateAccountInput no longer has a HolderID field; the composition holder-source guard is stale and must be revisited")
}

// TestCompositionHasNoHolderIDField pins the no-new-null-semantics contract at
// the type level: the composite body never exposes a holder field, so the
// holder cannot be set from the body (F4-T08 / R14+R40 enforced at the contract).
func TestCompositionHasNoHolderIDField(t *testing.T) {
	t.Parallel()

	compositeType := reflect.TypeOf(CreateHolderAccountInput{})

	_, hasHolderID := compositeType.FieldByName("HolderID")
	assert.False(t, hasHolderID, "CreateHolderAccountInput must not declare a HolderID field")

	for i := 0; i < compositeType.NumField(); i++ {
		assert.NotEqual(t, "holderId", compositeType.Field(i).Tag.Get("json"),
			"CreateHolderAccountInput must not expose a holderId json field")
	}
}

func TestCreateHolderAccountInput_ToCreateAccountInput(t *testing.T) {
	t.Parallel()

	parent := "11111111-1111-1111-1111-111111111111"
	entity := "EXT-ACC-1"
	portfolio := "22222222-2222-2222-2222-222222222222"
	segment := "33333333-3333-3333-3333-333333333333"
	alias := "@treasury"
	blocked := true
	holderID := "44444444-4444-4444-4444-444444444444"

	in := &CreateHolderAccountInput{
		Name:            "Corp Checking",
		ParentAccountID: &parent,
		EntityID:        &entity,
		AssetCode:       "USD",
		PortfolioID:     &portfolio,
		SegmentID:       &segment,
		Status:          Status{Code: "ACTIVE"},
		Alias:           &alias,
		Type:            "deposit",
		Blocked:         &blocked,
		Metadata:        map[string]any{"region": "Global"},
		// Instrument fields must NOT leak onto the account input.
		BankingDetails:   &BankingDetails{},
		RegulatoryFields: &RegulatoryFields{},
		RelatedParties:   []*RelatedParty{{Document: "doc", Name: "n", Role: "PRIMARY_HOLDER"}},
	}

	got := in.ToCreateAccountInput(holderID)

	// HolderID is sourced from the path argument, never from the body.
	if assert.NotNil(t, got.HolderID) {
		assert.Equal(t, holderID, *got.HolderID)
	}

	// Every mirrored account field carries across unchanged.
	assert.Equal(t, in.Name, got.Name)
	assert.Equal(t, in.ParentAccountID, got.ParentAccountID)
	assert.Equal(t, in.EntityID, got.EntityID)
	assert.Equal(t, in.AssetCode, got.AssetCode)
	assert.Equal(t, in.PortfolioID, got.PortfolioID)
	assert.Equal(t, in.SegmentID, got.SegmentID)
	assert.Equal(t, in.Status, got.Status)
	assert.Equal(t, in.Alias, got.Alias)
	assert.Equal(t, in.Type, got.Type)
	assert.Equal(t, in.Blocked, got.Blocked)
	assert.Equal(t, in.Metadata, got.Metadata)
}

// TestCreateHolderAccountInput_ToCreateAccountInput_MinimalBody covers the
// all-optional-fields-absent case: only required fields set, optionals stay nil.
func TestCreateHolderAccountInput_ToCreateAccountInput_MinimalBody(t *testing.T) {
	t.Parallel()

	holderID := "44444444-4444-4444-4444-444444444444"

	in := &CreateHolderAccountInput{
		AssetCode: "BRL",
		Type:      "deposit",
	}

	got := in.ToCreateAccountInput(holderID)

	if assert.NotNil(t, got.HolderID) {
		assert.Equal(t, holderID, *got.HolderID)
	}

	assert.Equal(t, "BRL", got.AssetCode)
	assert.Equal(t, "deposit", got.Type)
	assert.Nil(t, got.ParentAccountID)
	assert.Nil(t, got.EntityID)
	assert.Nil(t, got.PortfolioID)
	assert.Nil(t, got.SegmentID)
	assert.Nil(t, got.Alias)
	assert.Nil(t, got.Blocked)
	assert.Nil(t, got.Metadata)
}
