import { OtelTracerProvider } from '@/core/infrastructure/observability/otel-tracer-provider'
import { HttpService } from '@/lib/http'
import { LoggerAggregator, RequestIdRepository } from '@lerianstudio/lib-logs'
import { SpanStatusCode } from '@opentelemetry/api'
import { inject } from 'inversify'
import { InternalServerErrorApiException } from '@/lib/http/api-exception'

export class PluginManifestHttpService extends HttpService {
  constructor(
    @inject(LoggerAggregator)
    private readonly logger: LoggerAggregator,
    @inject(RequestIdRepository)
    private readonly requestIdRepository: RequestIdRepository,
    @inject(OtelTracerProvider)
    private readonly otelTracerProvider: OtelTracerProvider
  ) {
    super()
  }

  private pluginManifestCustomSpanName: string = 'plugin-manifest-request'

  protected async createDefaults() {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Request-Id': this.requestIdRepository.get()!
    }

    return { headers }
  }

  protected onBeforeFetch(request: Request): void {
    this.logger.info('[INFO] - PluginManifestHttpService', {
      url: request.url,
      method: request.method
    })

    this.otelTracerProvider.startCustomSpan(this.pluginManifestCustomSpanName)
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
    this.logger.error('[ERROR] - PluginManifestHttpService', {
      url: request.url,
      method: request.method,
      status: response.status,
      response: error
    })

    throw new InternalServerErrorApiException(error)
  }
}
