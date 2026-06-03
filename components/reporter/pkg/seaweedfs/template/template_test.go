// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	tmCore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/reporter/pkg/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestStorageRepository_Get_AppendsTplExtension(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)

	mockStorage.EXPECT().
		Download(gomock.Any(), "templates/abc123.tpl").
		Return(io.NopCloser(bytes.NewReader([]byte("tpl-data"))), nil)

	data, err := repo.Get(context.Background(), "abc123")
	require.NoError(t, err)
	assert.Equal(t, "tpl-data", string(data))
}

func TestStorageRepository_Get_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)

	mockStorage.EXPECT().
		Download(gomock.Any(), "templates/abc123.tpl").
		Return(nil, errors.New("download failed"))

	_, err := repo.Get(context.Background(), "abc123")
	require.Error(t, err)
}

func TestStorageRepository_Put_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)

	mockStorage.EXPECT().
		Upload(gomock.Any(), "templates/folder/file.tpl", gomock.Any(), "text/plain").
		DoAndReturn(func(_ context.Context, key string, reader io.Reader, contentType string) (string, error) {
			data, _ := io.ReadAll(reader)
			assert.Equal(t, "content", string(data))
			return key, nil
		})

	err := repo.Put(context.Background(), "folder/file", "text/plain", []byte("content"))
	require.NoError(t, err)
}

func TestStorageRepository_Put_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)

	mockStorage.EXPECT().
		Upload(gomock.Any(), "templates/file.tpl", gomock.Any(), "text/plain").
		Return("", errors.New("upload failed"))

	err := repo.Put(context.Background(), "file", "text/plain", []byte("x"))
	require.Error(t, err)
}

func TestStorageRepository_Get_UsesTenantScopedKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)
	ctx := tmCore.ContextWithTenantID(context.Background(), "tenant-a")

	mockStorage.EXPECT().
		Download(gomock.Any(), "tenant-a/templates/abc123.tpl").
		Return(io.NopCloser(bytes.NewReader([]byte("tpl-data"))), nil)

	data, err := repo.Get(ctx, "abc123")
	require.NoError(t, err)
	assert.Equal(t, "tpl-data", string(data))
}

func TestStorageRepository_Put_RejectsInvalidTenantScopedKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)
	ctx := tmCore.ContextWithTenantID(context.Background(), "tenant/bad")

	err := repo.Put(ctx, "file", "text/plain", []byte("x"))
	require.Error(t, err)
}
