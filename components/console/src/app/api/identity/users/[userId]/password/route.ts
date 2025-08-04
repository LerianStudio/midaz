import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  UpdateUserPassword,
  UpdateUserPasswordUseCase
} from '@/core/application/use-cases/users/update-user-password-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'

export const PATCH = applyMiddleware(
  [loggerMiddleware({ operationName: 'updateUserPassword', method: 'PATCH' })],
  async (request: Request, { params }: { params: { userId: string } }) => {
    try {
      const updateUserPasswordUseCase: UpdateUserPassword =
        container.get<UpdateUserPassword>(UpdateUserPasswordUseCase)
      const { userId } = await params
      const { oldPassword, newPassword } = await request.json()

      await updateUserPasswordUseCase.execute(userId, oldPassword, newPassword)

      return NextResponse.json({}, { status: 200 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
