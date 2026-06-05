// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package report

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	tmCore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestStorageRepository_Put_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)

	mockStorage.EXPECT().
		UploadWithTTL(gomock.Any(), "reports/obj.txt", gomock.Any(), "text/plain", "").
		DoAndReturn(func(_ context.Context, key string, reader io.Reader, contentType, ttl string) (string, error) {
			data, _ := io.ReadAll(reader)
			assert.Equal(t, "hello", string(data))
			return key, nil
		})

	err := repo.Put(context.Background(), "obj.txt", "text/plain", []byte("hello"), "")
	require.NoError(t, err)
}

func TestStorageRepository_Put_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)

	mockStorage.EXPECT().
		UploadWithTTL(gomock.Any(), "reports/obj.txt", gomock.Any(), "text/plain", "").
		Return("", errors.New("upload failed"))

	err := repo.Put(context.Background(), "obj.txt", "text/plain", []byte("hello"), "")
	require.Error(t, err)
}

func TestStorageRepository_Put_WithTTL_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)

	mockStorage.EXPECT().
		UploadWithTTL(gomock.Any(), "reports/temp.txt", gomock.Any(), "text/plain", "1m").
		DoAndReturn(func(_ context.Context, key string, reader io.Reader, contentType, ttl string) (string, error) {
			data, _ := io.ReadAll(reader)
			assert.Equal(t, "temporary", string(data))
			return key, nil
		})

	err := repo.Put(context.Background(), "temp.txt", "text/plain", []byte("temporary"), "1m")
	require.NoError(t, err)
}

func TestStorageRepository_Get_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)

	mockStorage.EXPECT().
		Download(gomock.Any(), "reports/obj.txt").
		Return(io.NopCloser(bytes.NewReader([]byte("world"))), nil)

	data, err := repo.Get(context.Background(), "obj.txt")
	require.NoError(t, err)
	assert.Equal(t, "world", string(data))
}

func TestStorageRepository_Get_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)

	mockStorage.EXPECT().
		Download(gomock.Any(), "reports/obj.txt").
		Return(nil, errors.New("download failed"))

	_, err := repo.Get(context.Background(), "obj.txt")
	require.Error(t, err)
}

func TestStorageRepository_Put_UsesTenantScopedKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)
	ctx := tmCore.ContextWithTenantID(context.Background(), "tenant-a")

	mockStorage.EXPECT().
		UploadWithTTL(gomock.Any(), "tenant-a/reports/obj.txt", gomock.Any(), "text/plain", "").
		Return("tenant-a/reports/obj.txt", nil)

	err := repo.Put(ctx, "obj.txt", "text/plain", []byte("hello"), "")
	require.NoError(t, err)
}

func TestStorageRepository_Get_RejectsInvalidTenantScopedKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockObjectStorage(ctrl)
	repo := NewStorageRepository(mockStorage)
	ctx := tmCore.ContextWithTenantID(context.Background(), "tenant/bad")

	err := repo.Put(ctx, "obj.txt", "text/plain", []byte("hello"), "")
	require.Error(t, err)
}
