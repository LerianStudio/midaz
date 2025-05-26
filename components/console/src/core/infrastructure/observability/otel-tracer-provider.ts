import {
  Span,
  SpanStatus,
  SpanStatusCode,
  trace,
  Tracer
} from '@opentelemetry/api'
import { injectable } from 'inversify'

export type SpanData = {
  attributes: { [key: string]: any }
  status: SpanStatus
}

@injectable()
export class OtelTracerProvider {
  private otelTracer?: Tracer
  private isTelemetryEnabled: boolean =
    process.env.ENABLE_TELEMETRY === 'true' || false

  private span: Span | undefined

  constructor() {
    if (this.isTelemetryEnabled) {
      this.otelTracer = trace.getTracer('midaz-console')
    }
  }

  public startCustomSpan(spanName: string): void {
    if (this.isTelemetryEnabled && this.otelTracer) {
      this.span = this.otelTracer.startSpan(spanName)
    }
  }

  public endCustomSpan(spanData?: SpanData): void {
    if (this.isTelemetryEnabled && this.otelTracer && this.span) {
      this.setCustomSpanAttributes(spanData?.attributes ?? {})
      this.setCustomSpanStatus(
        spanData?.status ?? { code: SpanStatusCode.UNSET }
      )
      this.endSpan()
    }
  }

  private setCustomSpanAttributes(spanAttributes: Record<string, any>) {
    if (this.isTelemetryEnabled && this.otelTracer) {
      this.span?.setAttributes(spanAttributes)
    }
  }

  private setCustomSpanStatus(spanStatus: SpanStatus) {
    if (this.isTelemetryEnabled && this.otelTracer) {
      this.span?.setStatus(spanStatus)
    }
  }

  private endSpan() {
    if (this.span) {
      this.span.end()
    }
  }
}
