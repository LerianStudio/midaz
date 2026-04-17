// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// These tests exercise the generated mock to verify:
//   - NewMockRepository wires the recorder correctly
//   - EXPECT returns the recorder
//   - Each mocked method round-trips the programmed return values
//
// They guard against silent breakage when regenerating the mock: if the
// generator output ever drifts (e.g. an argument is renamed and no longer
// compiles against the contract) this file will fail to build.

var (
	errMockCreateFailed = errors.New("create failed")
	errMockDeleteFailed = errors.New("delete failed")
	errMockUpdateFailed = errors.New("update failed")
	errMockFindFailed   = errors.New("find failed")
)

func TestMetadataMockRepository_NewAndEXPECT(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)
	require.NotNil(t, m)

	rec := m.EXPECT()
	require.NotNil(t, rec)
	assert.Same(t, m, rec.mock)
}

func TestMetadataMockRepository_CreateRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	meta := &Metadata{EntityID: "id-1", EntityName: "transaction"}

	m.EXPECT().Create(gomock.Any(), "transaction", meta).Return(nil).Times(1)
	require.NoError(t, m.Create(context.Background(), "transaction", meta))

	m.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(errMockCreateFailed).Times(1)
	err := m.Create(context.Background(), "transaction", meta)
	require.ErrorIs(t, err, errMockCreateFailed)
}

func TestMetadataMockRepository_FindListRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	want := []*Metadata{{EntityID: "a"}, {EntityID: "b"}}

	m.EXPECT().
		FindList(gomock.Any(), "transaction", gomock.Any()).
		Return(want, nil).
		Times(1)

	got, err := m.FindList(context.Background(), "transaction", http.QueryHeader{})
	require.NoError(t, err)
	assert.Equal(t, want, got)

	m.EXPECT().
		FindList(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errMockFindFailed).
		Times(1)
	_, err = m.FindList(context.Background(), "transaction", http.QueryHeader{})
	require.ErrorIs(t, err, errMockFindFailed)
}

func TestMetadataMockRepository_FindByEntityRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	want := &Metadata{EntityID: "id-1"}
	m.EXPECT().FindByEntity(gomock.Any(), "transaction", "id-1").Return(want, nil).Times(1)

	got, err := m.FindByEntity(context.Background(), "transaction", "id-1")
	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestMetadataMockRepository_FindByEntityIDsRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	ids := []string{"a", "b"}
	want := []*Metadata{{EntityID: "a"}, {EntityID: "b"}}

	m.EXPECT().FindByEntityIDs(gomock.Any(), "transaction", ids).Return(want, nil).Times(1)

	got, err := m.FindByEntityIDs(context.Background(), "transaction", ids)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestMetadataMockRepository_UpdateRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	md := map[string]any{"k": "v"}
	m.EXPECT().Update(gomock.Any(), "transaction", "id-1", md).Return(nil).Times(1)
	require.NoError(t, m.Update(context.Background(), "transaction", "id-1", md))

	m.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errMockUpdateFailed).Times(1)
	require.ErrorIs(t, m.Update(context.Background(), "transaction", "id-1", md), errMockUpdateFailed)
}

func TestMetadataMockRepository_DeleteRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	m.EXPECT().Delete(gomock.Any(), "transaction", "id-1").Return(nil).Times(1)
	require.NoError(t, m.Delete(context.Background(), "transaction", "id-1"))

	m.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(errMockDeleteFailed).Times(1)
	require.ErrorIs(t, m.Delete(context.Background(), "transaction", "id-1"), errMockDeleteFailed)
}

func TestMetadataMockRepository_CreateIndexRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	input := &mmodel.CreateMetadataIndexInput{MetadataKey: "customer_id", Unique: true}
	want := &mmodel.MetadataIndex{IndexName: "metadata.customer_id_1", MetadataKey: "customer_id"}

	m.EXPECT().CreateIndex(gomock.Any(), "transaction", input).Return(want, nil).Times(1)

	got, err := m.CreateIndex(context.Background(), "transaction", input)
	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestMetadataMockRepository_FindAllIndexesRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	want := []*mmodel.MetadataIndex{{IndexName: "idx1"}, {IndexName: "idx2"}}

	m.EXPECT().FindAllIndexes(gomock.Any(), "transaction").Return(want, nil).Times(1)

	got, err := m.FindAllIndexes(context.Background(), "transaction")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestMetadataMockRepository_DeleteIndexRoundTrip(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRepository(ctrl)

	m.EXPECT().DeleteIndex(gomock.Any(), "transaction", "idx1").Return(nil).Times(1)
	require.NoError(t, m.DeleteIndex(context.Background(), "transaction", "idx1"))
}

// TestMetadataMockRepository_InterfaceAssignment guards the mock's contract
// with the Repository interface: assigning into a Repository variable fails
// to compile if a method signature drifts in mockgen output.
func TestMetadataMockRepository_InterfaceAssignment(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var repo Repository = NewMockRepository(ctrl)
	assert.NotNil(t, repo)
}
