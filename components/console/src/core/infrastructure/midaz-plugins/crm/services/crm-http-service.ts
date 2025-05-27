import { inject, injectable } from 'inversify'
import { MidazRequestContext } from '@/core/infrastructure/logger/decorators/midaz-id'
import { LoggerAggregator } from '@/core/infrastructure/logger/logger-aggregator'
import { HttpService } from '@/lib/http'
import { getServerSession } from 'next-auth'
import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'
import { OtelTracerProvider } from '@/core/infrastructure/observability/otel-tracer-provider'
import { SpanStatusCode } from '@opentelemetry/api'
import { getIntl } from '@/lib/intl'
import { CrmApiException } from '../exceptions/crm-exception'
import * as crypto from 'crypto'

@injectable()
export class CrmHttpService extends HttpService {
  private organizationId: string = ''

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

  private crmCustomSpanName: string = 'crm-request'

  setOrganizationId(organizationId: string) {
    this.organizationId = organizationId
    return this
  }

  protected async createDefaults() {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Request-Id': this.midazRequestContext.getMidazId(),
      'x-lerian-id': crypto.randomUUID()
    }

    // Add organization ID if set
    if (this.organizationId) {
      headers['X-Organization-Id'] = this.organizationId
    }

    if (process.env.PLUGIN_AUTH_ENABLED === 'true') {
      const session = await getServerSession(nextAuthOptions)
      const { access_token } = session?.user
      headers.Authorization = `${access_token}`
    }

    return { headers }
  }

  protected onBeforeFetch(request: Request): void {
    this.logger.info('[INFO] - CrmHttpService', {
      url: request.url,
      method: request.method
    })

    this.otelTracerProvider.startCustomSpan(this.crmCustomSpanName)
  }

  protected onAfterFetch(request: Request, response: Response): void {
    this.otelTracerProvider.endCustomSpan({
      attributes: {
        'http.url': request.url,
        'http.method': request.method,
        'http.status_code': response.status,
        'http.request_id': this.midazRequestContext.getMidazId()
      },
      status: {
        code: response.ok ? SpanStatusCode.OK : SpanStatusCode.ERROR
      }
    })
  }

  protected async catch(request: Request, response: Response, error: any) {
    this.logger.error('[ERROR] - CrmHttpService', {
      url: request.url,
      method: request.method,
      status: response.status,
      response: error
    })

    const intl = await getIntl()

    if (response.status === 400) {
      throw new CrmApiException(
        intl.formatMessage({
          id: 'crm.api.error.badRequest',
          defaultMessage: 'Bad request to CRM service'
        }),
        400
      )
    }

    if (response.status === 422) {
      throw new CrmApiException(
        intl.formatMessage({
          id: 'crm.api.error.unprocessableEntity',
          defaultMessage: 'Unprocessable entity error in CRM service'
        }),
        422
      )
    }

    if (response.status === 404) {
      throw new CrmApiException(
        intl.formatMessage({
          id: 'crm.api.error.notFound',
          defaultMessage: 'Resource not found in CRM service'
        }),
        404
      )
    }

    if (response.status >= 500) {
      throw new CrmApiException(
        intl.formatMessage({
          id: 'crm.api.error.serverError',
          defaultMessage: 'Server error in CRM service'
        }),
        response.status
      )
    }

    // Default error
    throw new CrmApiException(
      intl.formatMessage({
        id: 'error.crm.unknownError',
        defaultMessage: 'Unknown error in CRM service'
      }),
      response.status || 500
    )
  }
}
