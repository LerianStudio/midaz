// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package scoperesolver

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// fakeValidations answers the validations seeded, fails on the ids in failing,
// and records every id it was asked for.
type fakeValidations struct {
	stored  map[uuid.UUID]*model.TransactionValidation
	failing map[uuid.UUID]error
	asked   []uuid.UUID
}

func (f *fakeValidations) GetByID(_ context.Context, id uuid.UUID) (*model.TransactionValidation, error) {
	f.asked = append(f.asked, id)

	if err, fails := f.failing[id]; fails {
		return nil, err
	}

	if v, found := f.stored[id]; found {
		return v, nil
	}

	return nil, constant.ErrTransactionValidationNotFound
}

func items(values ...string) []middleware.ResolveItem {
	out := make([]middleware.ResolveItem, 0, len(values))
	for _, v := range values {
		out = append(out, middleware.ResolveItem{Value: v})
	}

	return out
}

func TestRegister_RegistersEveryManifestResolver(t *testing.T) {
	auth := &middleware.AuthClient{}
	require.NoError(t, Register(auth, &fakeValidations{}, nil))

	for _, name := range []string{ValidationAccount, ValidationSegment, ValidationPortfolio, ValidationMerchant} {
		noop := func(context.Context, middleware.ResolveInput) ([][]string, error) { return nil, nil }
		assert.Errorf(t, auth.RegisterScopeResolver(name, noop), "%s must already be registered", name)
	}
}

func TestRegister_RefusesANilClient(t *testing.T) {
	assert.Error(t, Register(nil, &fakeValidations{}, nil))
}

// resolveWith runs the resolver Register registers under name.
func resolveWith(t *testing.T, name string, validations ValidationReader, in middleware.ResolveInput) ([][]string, error) {
	t.Helper()

	return pickers(resolvers{validations: validations})[name](context.Background(), in)
}

func TestValidationResolvers_AnswerTheStoredPlacement(t *testing.T) {
	account, segment, portfolio, merchant := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	placed, bare := uuid.New(), uuid.New()

	validations := &fakeValidations{stored: map[uuid.UUID]*model.TransactionValidation{
		placed: {
			ID: placed, Account: model.AccountContext{ID: account},
			Segment:   &model.SegmentContext{ID: segment},
			Portfolio: &model.PortfolioContext{ID: portfolio},
			Merchant:  &model.MerchantContext{ID: merchant},
		},
		bare: {ID: bare, Account: model.AccountContext{ID: account}},
	}}

	for name, want := range map[string]uuid.UUID{
		ValidationAccount:   account,
		ValidationSegment:   segment,
		ValidationPortfolio: portfolio,
		ValidationMerchant:  merchant,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := resolveWith(t, name, validations, middleware.ResolveInput{Dimension: name, Items: items(placed.String())})
			require.NoError(t, err)
			assert.Equal(t, [][]string{{want.String()}}, got)
		})
	}

	for _, name := range []string{ValidationSegment, ValidationPortfolio, ValidationMerchant} {
		t.Run(name+" of a validation submitted without it names nothing", func(t *testing.T) {
			got, err := resolveWith(t, name, validations, middleware.ResolveInput{Dimension: name, Items: items(bare.String())})
			require.NoError(t, err)
			assert.Equal(t, [][]string{nil}, got)
		})
	}
}

func TestValidationResolvers_AnswerEachItemInOrder(t *testing.T) {
	a1, a2 := uuid.New(), uuid.New()
	ofA1, ofA2, unknown := uuid.New(), uuid.New(), uuid.New()

	validations := &fakeValidations{stored: map[uuid.UUID]*model.TransactionValidation{
		ofA1: {ID: ofA1, Account: model.AccountContext{ID: a1}},
		ofA2: {ID: ofA2, Account: model.AccountContext{ID: a2}},
	}}

	got, err := resolveWith(t, ValidationAccount, validations, middleware.ResolveInput{
		Dimension: "accountId",
		Items:     items(ofA2.String(), "not-a-uuid", unknown.String(), ofA1.String()),
	})
	require.NoError(t, err)

	assert.Equal(t, [][]string{{a2.String()}, nil, nil, {a1.String()}}, got,
		"one entry per item; a value that is not a uuid or names no validation names nothing")
	assert.Equal(t, []uuid.UUID{ofA2, unknown, ofA1}, validations.asked, "a value that is not a uuid is never looked up")
}

func TestValidationResolvers_AFailedLookupIsAnError(t *testing.T) {
	broken := uuid.New()
	cause := errors.New("connection refused")

	_, err := resolveWith(t, ValidationAccount, &fakeValidations{failing: map[uuid.UUID]error{broken: cause}},
		middleware.ResolveInput{Dimension: "accountId", Items: items(broken.String())})

	require.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), "accountId")
}

func TestValidationResolvers_WithoutAReaderIsAnError(t *testing.T) {
	_, err := resolveWith(t, ValidationAccount, nil, middleware.ResolveInput{Dimension: "accountId", Items: items(uuid.NewString())})

	require.ErrorIs(t, err, errNoValidationReader)
}
