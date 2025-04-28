import { AuthSessionDto } from '@/core/application/dto/auth-dto'
import { LoggerAggregator } from '@/core/application/logger/logger-aggregator'
import {
  AuthLogin,
  AuthLoginUseCase
} from '@/core/application/use-cases/auth/auth-login-use-case'
import { AuthEntity } from '@/core/domain/entities/auth-entity'
import { LoggerRepository } from '@/core/domain/repositories/logger/logger-repository'
import { NextAuthOptions } from 'next-auth'
import CredentialsProvider from 'next-auth/providers/credentials'
import { log } from 'console'
import { container } from '../container-registry/container-registry'
import { MidazRequestContext } from '../logger/decorators/midaz-id'

export const nextAuthOptions: NextAuthOptions = {
  session: {
    strategy: 'jwt',
    maxAge: 30 * 60,
    updateAge: 24 * 60 * 60
  },
  jwt: {
    maxAge: 30 * 60
  },
  debug: false,
  logger: {
    error(code, metadata) {
      console.error(code, metadata)
    },
    warn(code) {
      console.warn(code)
    },
    debug(code, metadata) {
      console.debug(code, metadata)
    }
  },

  providers: [
    CredentialsProvider({
      name: 'credentials',
      credentials: {
        username: { label: 'username', type: 'text' },
        password: { label: 'password', type: 'password' }
      },
      type: 'credentials',

      async authorize(credentials, req) {
        const midazLogger = container.get(LoggerAggregator)
        const midazRequestContext: MidazRequestContext =
          container.get<MidazRequestContext>(MidazRequestContext)
        try {
          const authResponse = await midazLogger.runWithContext(
            'authLogin',
            'POST',
            {
              operationName: 'next-auth-provider',
              action: 'authorize',
              midazId: midazRequestContext.getMidazId()
            },
            async () => {
              const authLoginUseCase: AuthLogin =
                container.get<AuthLogin>(AuthLoginUseCase)

              const username = credentials?.username
              const password = credentials?.password

              if (!username || !password) {
                midazLogger.error('Error on authorize', {
                  message: 'Username or password not provided'
                })
                return null
              }

              const loginEntity: AuthEntity = {
                username,
                password
              }

              const authLoginResponse: AuthSessionDto =
                await authLoginUseCase.execute(loginEntity)

              return authLoginResponse
            }
          )

          return authResponse
        } catch (error: any) {
          midazLogger.error('Error on authorize', error)
          return null
        }
      }
    })
  ],

  pages: {
    signIn: '/signin'
  },
  callbacks: {
    jwt: async ({ token, user }) => {
      if (user) {
        token = { ...token, ...user }
      }
      return token
    },
    session: async ({ session, token }) => {
      session.user = token
      return session
    }
  }
}
