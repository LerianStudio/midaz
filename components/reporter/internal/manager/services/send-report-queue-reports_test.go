// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/rabbitmq"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUseCase_SendReportQueueReports(t *testing.T) {
	t.Parallel()

	reportID := uuid.New()
	templateID := uuid.New()

	tests := []struct {
		name          string
		reportMessage model.ReportMessage
		mockSetup     func(ctrl *gomock.Controller) *UseCase
		expectErr     bool
		errContains   string
	}{
		{
			name: "Success - Send report to queue",
			reportMessage: model.ReportMessage{
				ReportID:     reportID,
				TemplateID:   templateID,
				OutputFormat: "pdf",
				MappedFields: map[string]map[string][]string{
					"midaz_onboarding": {
						"asset": {"name", "type", "code"},
					},
				},
				Filters: nil,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)
				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
					Return(nil, nil)
				return &UseCase{
					Logger:                    log.NewNop(),
					Tracer:                    noop.NewTracerProvider().Tracer("test"),
					RabbitMQRepo:              mockRabbitMQ,
					RabbitMQExchange:          "test-exchange",
					RabbitMQGenerateReportKey: "test-key",
				}
			},
		},
		{
			name: "Success - Send report with filters",
			reportMessage: model.ReportMessage{
				ReportID:     reportID,
				TemplateID:   templateID,
				OutputFormat: "xml",
				MappedFields: map[string]map[string][]string{
					"midaz_transaction_metadata": {
						"transaction": {"metadata"},
					},
				},
				Filters: map[string]map[string]map[string]model.FilterCondition{
					"midaz_transaction_metadata": {
						"transaction": {
							"id": {
								Equals: []any{"123"},
							},
						},
					},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)
				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
					Return(nil, nil)
				return &UseCase{
					Logger:                    log.NewNop(),
					Tracer:                    noop.NewTracerProvider().Tracer("test"),
					RabbitMQRepo:              mockRabbitMQ,
					RabbitMQExchange:          "test-exchange",
					RabbitMQGenerateReportKey: "test-key",
				}
			},
		},
		{
			name: "Success - Send report with empty mapped fields",
			reportMessage: model.ReportMessage{
				ReportID:     reportID,
				TemplateID:   templateID,
				OutputFormat: "html",
				MappedFields: map[string]map[string][]string{},
				Filters:      nil,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)
				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
					Return(nil, nil)
				return &UseCase{
					Logger:                    log.NewNop(),
					Tracer:                    noop.NewTracerProvider().Tracer("test"),
					RabbitMQRepo:              mockRabbitMQ,
					RabbitMQExchange:          "test-exchange",
					RabbitMQGenerateReportKey: "test-key",
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := tt.mockSetup(ctrl)

			ctx := context.Background()

			err := svc.SendReportQueueReports(ctx, tt.reportMessage)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUseCase_SendReportQueueReports_WithDifferentOutputFormats(t *testing.T) {
	t.Parallel()

	reportID := uuid.New()
	templateID := uuid.New()

	outputFormats := []string{"pdf", "xml", "html", "txt", "csv"}

	for _, format := range outputFormats {
		format := format
		t.Run("Success - OutputFormat "+format, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)
			mockRabbitMQ.EXPECT().
				ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
				Return(nil, nil)

			svc := &UseCase{
				Logger:                    log.NewNop(),
				Tracer:                    noop.NewTracerProvider().Tracer("test"),
				RabbitMQRepo:              mockRabbitMQ,
				RabbitMQExchange:          "test-exchange",
				RabbitMQGenerateReportKey: "test-key",
			}

			reportMessage := model.ReportMessage{
				ReportID:     reportID,
				TemplateID:   templateID,
				OutputFormat: format,
				MappedFields: map[string]map[string][]string{
					"test_db": {
						"test_table": {"field1", "field2"},
					},
				},
			}

			ctx := context.Background()

			err := svc.SendReportQueueReports(ctx, reportMessage)
			require.NoError(t, err)
		})
	}
}
