import { MidazRequestContext } from '@/core/infrastructure/logger/decorators/midaz-id'
import { LoggerAggregator } from '@/core/infrastructure/logger/logger-aggregator'
import { OtelTracerProvider } from '@/core/infrastructure/observability/otel-tracer-provider'
import { HttpService } from '@/lib/http'
import { SpanStatusCode } from '@opentelemetry/api'
import { inject, injectable } from 'inversify'

@injectable()
export class AuthHttpService extends HttpService {
  constructor(
    @inject(LoggerAggregator)
    private readonly logger: LoggerAggregator,
    @inject(MidazRequestContext)
    private readonly midazRequestContext: MidazRequestContext,
    @inject(OtelTracerProvider)
    private readonly otelTracerProvider: OtelTracerProvider
  ) {
    super()
  }

  private authCustomSpanName: string = 'midaz-auth-request'

  protected async createDefaults() {
    return {
      'Content-Type': 'application/json',
      'X-Request-Id': this.midazRequestContext.getMidazId()
    }
  }

  protected onBeforeFetch(request: Request): void {
    this.logger.info('[INFO] - AuthHttpService', {
      url: request.url,
      method: request.method
    })

    this.otelTracerProvider.startCustomSpan(this.authCustomSpanName)
  }

  protected onAfterFetch(request: Request, response: Response): void {
    this.otelTracerProvider.endCustomSpan({
      attributes: {
        'http.url': request.url,
        'http.method': request.method,
        'http.status_code': response.status
      },
      status: {
        code: response.ok ? SpanStatusCode.OK : SpanStatusCode.ERROR
      }
    })
  }

  protected async catch(request: Request, response: Response, error: any) {
    this.logger.error('[ERROR] - AuthHttpService', {
      url: request.url,
      method: request.method,
      status: response.status,
      response: error
    })
  }
}
