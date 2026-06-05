// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/template"
	httpUtils "github.com/LerianStudio/midaz/v4/pkg/reporter/net/http"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUseCase_GetAllTemplates(t *testing.T) {
	t.Parallel()

	tempID := uuid.New()
	resultEntity := []*template.Template{
		{
			ID:           tempID,
			Description:  "Template Financeiro",
			OutputFormat: "html",
			FileName:     "019672b1-9d50-7360-9df5-5099dd166709_1745680964.tpl",
		},
	}

	filter := httpUtils.QueryHeader{
		Limit: 10,
		Page:  1,
	}

	tests := []struct {
		name           string
		filter         httpUtils.QueryHeader
		mockSetup      func(ctrl *gomock.Controller) *UseCase
		expectErr      bool
		expectedErr    error
		expectedResult []*template.Template
	}{
		{
			name:   "Success - Get all templates",
			filter: filter,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTempRepo.EXPECT().
					FindList(gomock.Any(), filter).
					Return(resultEntity, nil)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), TemplateRepo: mockTempRepo}
			},
			expectErr: false,
			expectedResult: []*template.Template{
				{
					ID:           tempID,
					Description:  "Template Financeiro",
					OutputFormat: "html",
					FileName:     "019672b1-9d50-7360-9df5-5099dd166709_1745680964.tpl",
				},
			},
		},
		{
			name:   "Error - Get all templates",
			filter: filter,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTempRepo.EXPECT().
					FindList(gomock.Any(), filter).
					Return(nil, constant.ErrBadRequest)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), TemplateRepo: mockTempRepo}
			},
			expectErr:      true,
			expectedErr:    constant.ErrBadRequest,
			expectedResult: nil,
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
			result, err := tempSvc.GetAllTemplates(ctx, tt.filter)

			if tt.expectErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}
