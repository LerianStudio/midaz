// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/LerianStudio/reporter/pkg/ctxutil"
	"github.com/LerianStudio/reporter/pkg/model"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// SendReportQueueReports sends a report to the queue of a generation reports message to a RabbitMQ queue for further processing.
// It uses context for logger and tracer management and handles data serialization and queue message construction.
func (uc *UseCase) SendReportQueueReports(ctx context.Context, reportMessage model.ReportMessage) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.send_queue")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.template_id", reportMessage.TemplateID.String()),
		attribute.String("app.request.report_id", reportMessage.ReportID.String()),
		attribute.String("app.request.output_format", reportMessage.OutputFormat),
		attribute.Int("app.request.filter_datasource_count", len(reportMessage.Filters)),
	)

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctx,
		uc.RabbitMQExchange,
		uc.RabbitMQGenerateReportKey,
		reportMessage,
	); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to send message to queue", err)
		uc.Logger.Log(ctx, log.LevelError, "Failed to send message to queue",
			log.String("report_id", reportMessage.ReportID.String()),
			log.String("template_id", reportMessage.TemplateID.String()),
			log.Err(err),
		)

		return err
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Report sent to generate report queue successfully",
		log.String("report_id", reportMessage.ReportID.String()),
		log.String("template_id", reportMessage.TemplateID.String()),
	)

	return nil
}
