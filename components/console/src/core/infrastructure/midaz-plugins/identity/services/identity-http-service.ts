import { LoggerAggregator, RequestIdRepository } from '@lerianstudio/lib-logs'
import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'
import { OtelTracerProvider } from '@/core/infrastructure/observability/otel-tracer-provider'
import { HttpService, InternalServerErrorApiException } from '@/lib/http'
import { getIntl } from '@/lib/intl'
import { SpanStatusCode } from '@opentelemetry/api'
import { inject } from 'inversify'
import { getServerSession } from 'next-auth'
import { IdentityApiException } from '../exceptions/identity-exceptions'
import { identityApiMessages } from '../messages/messages'

export class IdentityHttpService extends HttpService {
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

  private identityCustomSpanName: string = 'identity-request'

  protected async createDefaults() {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Request-Id': this.requestIdRepository.get()!
    }

    if (process.env.PLUGIN_AUTH_ENABLED === 'true') {
      const session = await getServerSession(nextAuthOptions)
      const { access_token } = session?.user
      headers.Authorization = `Bearer ${access_token}`
    }

    return { headers }
  }

  protected onBeforeFetch(request: Request): void {
    this.logger.info('[INFO] - IdentityHttpService', {
      url: request.url,
      method: request.method
    })

    this.otelTracerProvider.startCustomSpan(this.identityCustomSpanName)
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

    this.logger.error('[ERROR] - IdentityHttpService', {
      url: request.url,
      method: request.method,
      status: response.status,
      response: error
    })

    if (error?.code && error.code.includes('AUT')) {
      const message =
        identityApiMessages[error.code as keyof typeof identityApiMessages]

      if (!message) {
        this.logger.warn('[ERROR] - AuthHttpService - Error code not found', {
          url: request.url,
          method: request.method,
          status: response.status,
          response: error
        })
        throw new IdentityApiException(
          intl.formatMessage({
            id: 'error.midaz.unknowError',
            defaultMessage: 'Unknown error on Midaz.'
          })
        )
      }

      throw new IdentityApiException(
        intl.formatMessage(message),
        error.code,
        response.status
      )
    }

    if (error?.code) {
      const message =
        identityApiMessages[error.code as keyof typeof identityApiMessages]

      if (!message) {
        this.logger.warn('[ERROR] - MidazHttpService - Error code not found', {
          url: request.url,
          method: request.method,
          status: response.status,
          response: error
        })
        throw new IdentityApiException(
          intl.formatMessage({
            id: 'error.midaz.unknowError',
            defaultMessage: 'Unknown error on Midaz.'
          })
        )
      }

      throw new IdentityApiException(
        intl.formatMessage(message),
        error.code,
        response.status
      )
    }

    throw new InternalServerErrorApiException(
      intl.formatMessage({
        id: 'error.midaz.unknowError',
        defaultMessage: 'Unknown error on Midaz.'
      })
    )
  }
}
