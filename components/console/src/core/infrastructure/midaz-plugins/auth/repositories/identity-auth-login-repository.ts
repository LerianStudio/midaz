import { LoggerAggregator } from '@lerianstudio/lib-logs'
import {
  AuthEntity,
  AuthResponseEntity,
  AuthSessionEntity
} from '@/core/domain/entities/auth-entity'
import { AuthLoginRepository } from '@/core/domain/repositories/auth/auth-login-repository'
import { inject, injectable } from 'inversify'
import * as jwt from 'jsonwebtoken'
import { JwtPayload } from 'jsonwebtoken'
import { UnauthorizedApiException } from '@/lib/http'
import { getIntl } from '@/lib/intl'
import { AuthHttpService } from '../services/auth-http-service'

@injectable()
export class IdentityAuthLoginRepository implements AuthLoginRepository {
  constructor(
    @inject(AuthHttpService)
    private readonly httpService: AuthHttpService,
    @inject(LoggerAggregator)
    private readonly midazLogger: LoggerAggregator
  ) {}

  private readonly authBaseUrl: string = process.env
    .PLUGIN_AUTH_BASE_PATH as string
  private readonly authClientId: string = process.env
    .PLUGIN_AUTH_CLIENT_ID as string
  private readonly authClientSecret: string = process.env
    .PLUGIN_AUTH_CLIENT_SECRET as string

  async login(loginData: AuthEntity): Promise<AuthSessionEntity> {
    const intl = await getIntl()

    this.midazLogger.audit('[AUDIT] - Login ', {
      username: loginData.username,
      event: 'Login attempt'
    })

    const url = `${this.authBaseUrl}/login/oauth/access_token`

    const loginDataWithClient = {
      ...loginData,
      clientId: this.authClientId,
      clientSecret: this.authClientSecret,
      grantType: 'password'
    }

    try {
      const authResponse: AuthResponseEntity =
        await this.httpService.login<AuthResponseEntity>(url, {
          body: JSON.stringify(loginDataWithClient)
        })

      const jwtPauload: JwtPayload = jwt.decode(
        authResponse.accessToken
      ) as JwtPayload

      const authSession: AuthSessionEntity = {
        id: jwtPauload.sub as string,
        username: jwtPauload.name,
        name: jwtPauload.displayName,
        idToken: authResponse.idToken,
        accessToken: authResponse.accessToken,
        refreshToken: authResponse.refreshToken
      }

      this.midazLogger.audit('[AUDIT] - Login ', {
        username: loginData.username,
        event: 'login successful'
      })

      return authSession
    } catch (error: any) {
      // TODO - handle unauthorized error

      this.midazLogger.error('[ERROR] - Login ', {
        username: loginData.username,
        event: 'login failed',
        error: error.message
      })

      throw new UnauthorizedApiException(
        intl.formatMessage({
          id: 'error.login',
          defaultMessage: 'Invalid credentials'
        })
      )
    }
  }
}
