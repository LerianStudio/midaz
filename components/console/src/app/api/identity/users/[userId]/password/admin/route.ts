import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  ResetUserPassword,
  ResetUserPasswordUseCase
} from '@/core/application/use-cases/users/reset-user-password-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'

export const PATCH = applyMiddleware(
  [loggerMiddleware({ operationName: 'resetUserPassword', method: 'PATCH' })],
  async (request: Request, { params }: { params: { userId: string } }) => {
    try {
      const resetUserPasswordUseCase: ResetUserPassword =
        container.get<ResetUserPassword>(ResetUserPasswordUseCase)
      const { userId } = await params
      const { newPassword } = await request.json()

      await resetUserPasswordUseCase.execute(userId, newPassword)

      return NextResponse.json({}, { status: 200 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
