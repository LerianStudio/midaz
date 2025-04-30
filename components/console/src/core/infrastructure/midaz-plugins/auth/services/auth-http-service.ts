import { MidazRequestContext } from '@/core/infrastructure/logger/decorators/midaz-id'
import { LoggerAggregator } from '@/core/infrastructure/logger/logger-aggregator'
import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'
import { OtelTracerProvider } from '@/core/infrastructure/observability/otel-tracer-provider'
import {
  FetchModuleOptions,
  HttpMethods,
  HttpService,
  InternalServerErrorApiException
} from '@/lib/http'
import { getIntl } from '@/lib/intl'
import { SpanStatusCode } from '@opentelemetry/api'
import { inject, injectable } from 'inversify'
import { getServerSession } from 'next-auth'

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

  async login<T>(url: string, options: FetchModuleOptions): Promise<T> {
    const headers = {
      'Content-Type': 'application/json',
      'X-Request-Id': this.midazRequestContext.getMidazId()
    }

    return await this.request<T>(
      new Request(url, { ...options, method: HttpMethods.POST, headers })
    )
  }

  protected async createDefaults() {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Request-Id': this.midazRequestContext.getMidazId()
    }

    if (process.env.PLUGIN_AUTH_ENABLED === 'true') {
      const session = await getServerSession(nextAuthOptions)
      const { access_token } = session?.user
      headers.Authorization = `${access_token}`
    }

    return { headers }
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
    const intl = await getIntl()

    this.logger.error('[ERROR] - AuthHttpService', {
      url: request.url,
      method: request.method,
      status: response.status,
      response: error
    })

    throw new InternalServerErrorApiException(
      intl.formatMessage({
        id: 'error.midaz.unknowError',
        defaultMessage: 'Unknown error on Midaz.'
      })
    )
  }
}
