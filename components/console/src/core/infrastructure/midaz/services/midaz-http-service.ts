import { inject, injectable } from 'inversify'
import { MidazRequestContext } from '@/core/infrastructure/logger/decorators/midaz-id'
import { LoggerAggregator } from '@/core/infrastructure/logger/logger-aggregator'
import { HttpService } from '@/lib/http'
import { getServerSession } from 'next-auth'
import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'
import { OtelTracerProvider } from '@/core/infrastructure/observability/otel-tracer-provider'
import { SpanStatusCode } from '@opentelemetry/api'
import { getIntl } from '@/lib/intl'
import { MidazApiException } from '../exceptions/midaz-exceptions'
import { apiErrorMessages } from '../messages/messages'

@injectable()
export class MidazHttpService extends HttpService {
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

  private midazCustomSpanName: string = 'midaz-request'

  protected async createDefaults() {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Request-Id': this.midazRequestContext.getMidazId()
    }

    if (process.env.PLUGIN_AUTH_ENABLED === 'true') {
      const session = await getServerSession(nextAuthOptions)
      const { access_token } = session?.user
      headers.Authorization = `Bearer ${access_token}`
    }

    return headers
  }

  protected onBeforeFetch(request: Request): void {
    this.logger.info('[INFO] - MidazHttpService', {
      url: request.url,
      method: request.method
    })

    this.otelTracerProvider.startCustomSpan(this.midazCustomSpanName)
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
    this.logger.error('[ERROR] - MidazHttpService', {
      url: request.url,
      method: request.method,
      status: response.status,
      response: error
    })

    const intl = await getIntl()

    if (error?.code) {
      const message =
        apiErrorMessages[error.code as keyof typeof apiErrorMessages]

      if (!message) {
        console.warn('MidazHttpService - Error code not found')
        throw new MidazApiException(
          intl.formatMessage({
            id: 'error.midaz.unknowError',
            defaultMessage: 'Unknown error on Midaz.'
          })
        )
      }

      throw new MidazApiException(intl.formatMessage(message), error.code)
    }

    throw new MidazApiException(
      intl.formatMessage({
        id: 'error.midaz.unknowError',
        defaultMessage: 'Unknown error on Midaz.'
      })
    )
  }
}
