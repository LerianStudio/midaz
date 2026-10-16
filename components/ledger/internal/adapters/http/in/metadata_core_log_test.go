// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// errorRecords returns the captured Log calls made at Error level.
func errorRecords(recorder *logRecorder) []logRecord {
	var out []logRecord

	for _, rec := range recorder.snapshot() {
		if rec.level == int(libLog.LevelError) {
			out = append(out, rec)
		}
	}

	return out
}

// TestCreateMetadataIndex_LogsOnlyInfrastructureErrors locks the documented logging
// exception of metadata_core.go: the handler owns the single ERROR line for an
// infrastructure error from the repository, and logs nothing for a 4xx.
func TestCreateMetadataIndex_LogsOnlyInfrastructureErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entityName string
		repoErr    error
		wantErrors []string
	}{
		{
			name:       "invalid_entity_name_logs_no_error",
			entityName: "not_an_entity",
			wantErrors: nil,
		},
		{
			name:       "index_creation_failure_logs_exactly_one_error",
			entityName: "transaction",
			repoErr:    errors.New("mongo: connection refused"),
			wantErrors: []string{"Failed to create metadata index"},
		},
		{
			name:       "business_error_from_repository_logs_no_error",
			entityName: "transaction",
			repoErr:    pkg.ValidateBusinessError(constant.ErrMetadataIndexAlreadyExists, constant.EntityMetadataIndex, "tier"),
			wantErrors: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			logger, recorder := recordingLogger(t, ctrl)
			handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

			if tt.repoErr != nil {
				transaction.EXPECT().
					CreateIndex(gomock.Any(), "transaction", gomock.Any()).
					Return(nil, tt.repoErr).
					Times(1)
			}

			ctx := libObservability.ContextWithLogger(context.Background(), logger)

			_, err := handler.createMetadataIndex(ctx, tt.entityName, map[string]string{}, &mmodel.CreateMetadataIndexInput{MetadataKey: "tier"})
			require.Error(t, err)

			got := errorRecords(recorder)
			require.Len(t, got, len(tt.wantErrors), "the handler must log exactly once per infrastructure error and never for a 4xx")

			for i, msg := range tt.wantErrors {
				assert.Equal(t, msg, got[i].msg)
			}
		})
	}
}
