// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/deadline"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/template"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

var errDBUnavailable = errors.New("db unavailable")

func TestUseCase_DeleteTemplateByID(t *testing.T) {
	t.Parallel()

	tempID := uuid.New()

	tests := []struct {
		name           string
		tempID         uuid.UUID
		hardDelete     bool
		mockSetup      func(ctrl *gomock.Controller) *UseCase
		expectErr      bool
		expectedResult error
	}{
		{
			name:       "Success - Delete a template without deadlines repo",
			tempID:     tempID,
			hardDelete: true,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTempRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), TemplateRepo: mockTempRepo}
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name:       "Success - Delete template cascades to linked deadlines",
			tempID:     tempID,
			hardDelete: true,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTempRepo.EXPECT().
					Delete(gomock.Any(), tempID, true).
					Return(nil)
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)
				mockDeadlineRepo.EXPECT().
					DeleteByTemplateID(gomock.Any(), tempID).
					Return(int64(3), nil)
				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name:       "Success - Delete template with no linked deadlines",
			tempID:     tempID,
			hardDelete: false,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTempRepo.EXPECT().
					Delete(gomock.Any(), tempID, false).
					Return(nil)
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)
				mockDeadlineRepo.EXPECT().
					DeleteByTemplateID(gomock.Any(), tempID).
					Return(int64(0), nil)
				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name:       "Error - Cascade deadline delete fails before template is deleted",
			tempID:     tempID,
			hardDelete: true,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				// TemplateRepo.Delete must NOT be called when cascade fails first,
				// so the template remains intact and the client can safely retry.
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)
				mockDeadlineRepo.EXPECT().
					DeleteByTemplateID(gomock.Any(), tempID).
					Return(int64(0), errDBUnavailable)
				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      true,
			expectedResult: errDBUnavailable,
		},
		{
			name:       "Error Bad Request - Delete a template",
			tempID:     tempID,
			hardDelete: true,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTempRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(constant.ErrBadRequest)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), TemplateRepo: mockTempRepo}
			},
			expectErr:      true,
			expectedResult: constant.ErrBadRequest,
		},
		{
			name:       "Error Document Not found - Delete a template",
			tempID:     tempID,
			hardDelete: true,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTempRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(mongo.ErrNoDocuments)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), TemplateRepo: mockTempRepo}
			},
			expectErr:      true,
			expectedResult: mongo.ErrNoDocuments,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			tempSvc := tt.mockSetup(ctrl)

			ctx := context.Background()
			err := tempSvc.DeleteTemplateByID(ctx, tt.tempID, tt.hardDelete)

			if tt.expectErr {
				assert.ErrorIs(t, err, tt.expectedResult)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
