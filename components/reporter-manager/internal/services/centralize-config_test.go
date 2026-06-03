// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/reporter/pkg/model"
	"github.com/LerianStudio/reporter/pkg/rabbitmq"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestUseCase_HasRabbitMQExchangeField verifies that the manager UseCase struct
// has a RabbitMQExchange field for centralized configuration instead of using os.Getenv.
func TestUseCase_HasRabbitMQExchangeField(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:           log.NewNop(),
		Tracer:           noop.NewTracerProvider().Tracer("test"),
		RabbitMQExchange: "reporter.generate-report.exchange",
	}

	assert.Equal(t, "reporter.generate-report.exchange", uc.RabbitMQExchange)
}

// TestUseCase_HasRabbitMQGenerateReportKeyField verifies that the manager UseCase struct
// has a RabbitMQGenerateReportKey field for centralized configuration instead of using os.Getenv.
func TestUseCase_HasRabbitMQGenerateReportKeyField(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:                    log.NewNop(),
		Tracer:                    noop.NewTracerProvider().Tracer("test"),
		RabbitMQGenerateReportKey: "reporter.generate-report.key",
	}

	assert.Equal(t, "reporter.generate-report.key", uc.RabbitMQGenerateReportKey)
}

// TestSendReportQueueReports_UsesUseCaseFields verifies that SendReportQueueReports
// uses UseCase.RabbitMQExchange and UseCase.RabbitMQGenerateReportKey fields
// instead of calling os.Getenv at runtime.
func TestUseCase_SendReportQueueReports_UsesUseCaseFields(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	expectedExchange := "injected-exchange"
	expectedKey := "injected-key"

	// The mock expects the exchange and key to come from UseCase fields,
	// NOT from os.Getenv. We intentionally do NOT set env vars here.
	// If the code still uses os.Getenv, the exchange/key will be empty strings
	// and this test will fail because the mock expects specific values.
	mockRabbitMQ.EXPECT().
		ProducerDefault(gomock.Any(), expectedExchange, expectedKey, gomock.Any()).
		Return(nil, nil)

	svc := &UseCase{
		Logger:                    log.NewNop(),
		Tracer:                    noop.NewTracerProvider().Tracer("test"),
		RabbitMQRepo:              mockRabbitMQ,
		RabbitMQExchange:          expectedExchange,
		RabbitMQGenerateReportKey: expectedKey,
	}

	reportMessage := model.ReportMessage{
		ReportID:     reportID,
		TemplateID:   templateID,
		OutputFormat: "pdf",
		MappedFields: map[string]map[string][]string{
			"test_db": {"test_table": {"field1"}},
		},
	}

	ctx := context.Background()
	err := svc.SendReportQueueReports(ctx, reportMessage)
	require.NoError(t, err)
}

// TestSendReportQueueReports_DoesNotUseOsGetenv verifies that even when env vars
// are set, the function reads from UseCase fields (not environment).
// This proves config centralization is complete.
func TestUseCase_SendReportQueueReports_DoesNotUseOsGetenv(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because t.Setenv is incompatible with parallel execution.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	// Set env vars to DIFFERENT values than UseCase fields
	t.Setenv("RABBITMQ_EXCHANGE", "env-exchange-should-not-be-used")
	t.Setenv("RABBITMQ_GENERATE_REPORT_KEY", "env-key-should-not-be-used")

	useCaseExchange := "usecase-exchange"
	useCaseKey := "usecase-key"

	// Mock expects the UseCase field values, NOT the env var values.
	// If the code reads from os.Getenv, this will fail with mismatched args.
	mockRabbitMQ.EXPECT().
		ProducerDefault(gomock.Any(), useCaseExchange, useCaseKey, gomock.Any()).
		Return(nil, nil)

	svc := &UseCase{
		Logger:                    log.NewNop(),
		Tracer:                    noop.NewTracerProvider().Tracer("test"),
		RabbitMQRepo:              mockRabbitMQ,
		RabbitMQExchange:          useCaseExchange,
		RabbitMQGenerateReportKey: useCaseKey,
	}

	reportMessage := model.ReportMessage{
		ReportID:     reportID,
		TemplateID:   templateID,
		OutputFormat: "html",
		MappedFields: map[string]map[string][]string{
			"test_db": {"test_table": {"field1"}},
		},
	}

	ctx := context.Background()
	err := svc.SendReportQueueReports(ctx, reportMessage)
	require.NoError(t, err)
}
