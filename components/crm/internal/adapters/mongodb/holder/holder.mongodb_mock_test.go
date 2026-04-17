// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// These tests exercise the generated mock to verify:
//   - NewMockRepository wires the recorder correctly
//   - EXPECT returns the recorder
//   - Each mocked method round-trips the programmed return values
//
// They also guard against silent breakage when regenerating the mock: if the
// generator output ever drifts (e.g. an argument is renamed and no longer
// compiles against the contract) this file will fail to build.

func TestMockRepository_NewAndEXPECT(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)
	require.NotNil(t, m)

	rec := m.EXPECT()
	require.NotNil(t, rec)
	assert.Same(t, m, rec.mock)
}

func TestMockRepository_CreateRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	id := uuid.New()
	doc := "doc-1"
	input := &mmodel.Holder{ID: &id, Document: &doc}
	want := &mmodel.Holder{ID: &id, Document: &doc}

	m.EXPECT().Create(gomock.Any(), "org", input).Return(want, nil).Times(1)

	got, err := m.Create(context.Background(), "org", input)

	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestMockRepository_CreateReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	sentinel := errTestCreateFailed
	m.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, sentinel).Times(1)

	got, err := m.Create(context.Background(), "org", &mmodel.Holder{})

	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
	assert.Nil(t, got)
}

func TestMockRepository_FindRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	id := uuid.New()
	want := &mmodel.Holder{ID: &id}

	m.EXPECT().Find(gomock.Any(), "org", id, true).Return(want, nil).Times(1)

	got, err := m.Find(context.Background(), "org", id, true)

	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestMockRepository_FindAllRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	id := uuid.New()
	want := []*mmodel.Holder{{ID: &id}}
	query := pkgHTTP.QueryHeader{Limit: 10, Page: 1}

	m.EXPECT().FindAll(gomock.Any(), "org", query, false).Return(want, nil).Times(1)

	got, err := m.FindAll(context.Background(), "org", query, false)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestMockRepository_UpdateRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	id := uuid.New()
	input := &mmodel.Holder{ID: &id}
	fields := []string{"name"}

	m.EXPECT().Update(gomock.Any(), "org", id, input, fields).Return(input, nil).Times(1)

	got, err := m.Update(context.Background(), "org", id, input, fields)

	require.NoError(t, err)
	assert.Same(t, input, got)
}

func TestMockRepository_DeleteRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	id := uuid.New()

	m.EXPECT().Delete(gomock.Any(), "org", id, true).Return(nil).Times(1)

	require.NoError(t, m.Delete(context.Background(), "org", id, true))
}

func TestMockRepository_DeleteReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	sentinel := errTestDeleteFailed
	m.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(sentinel).Times(1)

	err := m.Delete(context.Background(), "org", uuid.New(), false)

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// TestMockRepository_InterfaceAssignment guards the mock's contract with the
// Repository interface: assigning into a Repository variable fails to compile
// if a method signature drifts in mockgen output.
func TestMockRepository_InterfaceAssignment(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var repo Repository = NewMockRepository(ctrl)
	assert.NotNil(t, repo)
}
