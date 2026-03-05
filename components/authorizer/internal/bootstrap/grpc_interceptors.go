// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// wrappedServerStream wraps a grpc.ServerStream with a custom context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context, falling back to context.Background
// when the receiver or its context is nil.
func (w *wrappedServerStream) Context() context.Context {
	if w == nil || w.ctx == nil {
		return context.Background()
	}

	return w.ctx
}

func streamTelemetryInterceptor(telemetry *libOpentelemetry.Telemetry) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if telemetry == nil {
			return handler(srv, ss)
		}

		streamCtx := ss.Context()
		traceCtx := libOpentelemetry.ExtractGRPCContext(streamCtx)
		tracer := otel.Tracer(telemetry.LibraryName)

		streamCtx, span := tracer.Start(traceCtx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
		span.SetAttributes(
			attribute.String("grpc.method", info.FullMethod),
			attribute.Bool("grpc.client_stream", info.IsClientStream),
			attribute.Bool("grpc.server_stream", info.IsServerStream),
		)

		wrapped := &wrappedServerStream{ServerStream: ss, ctx: streamCtx}

		err := handler(srv, wrapped)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
		} else {
			span.SetStatus(otelcodes.Ok, "")
		}

		span.End()

		return err
	}
}

func streamLoggingInterceptor(logger libLog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)
		duration := time.Since(start)

		if logger == nil {
			return err
		}

		grpcStatus := status.Code(err)
		if err != nil && grpcStatus != grpcCodes.Canceled {
			logger.Warnf(
				"Authorizer stream rpc failed: method=%s client_stream=%t server_stream=%t status=%s duration_ms=%d err=%v",
				info.FullMethod,
				info.IsClientStream,
				info.IsServerStream,
				grpcStatus.String(),
				duration.Milliseconds(),
				err,
			)

			return err
		}

		logger.Infof(
			"Authorizer stream rpc completed: method=%s client_stream=%t server_stream=%t status=%s duration_ms=%d",
			info.FullMethod,
			info.IsClientStream,
			info.IsServerStream,
			grpcStatus.String(),
			duration.Milliseconds(),
		)

		return err
	}
}
