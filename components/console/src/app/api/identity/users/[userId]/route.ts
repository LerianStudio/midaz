import {
  FetchUserById,
  FetchUserByIdUseCase
} from '@/core/application/use-cases/users/fetch-user-by-id-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../../../utils/api-error-handler'
import {
  DeleteUser,
  DeleteUserUseCase
} from '@/core/application/use-cases/users/delete-user-use-case'
import {
  UpdateUser,
  UpdateUserUseCase
} from '@/core/application/use-cases/users/update-user-use-case'
import { UpdateAccountUseCase } from '@/core/application/use-cases/accounts/update-account-use-case'

export const GET = applyMiddleware(
  [loggerMiddleware({ operationName: 'fetchUserById', method: 'GET' })],
  async (request: Request, { params }: { params: { userId: string } }) => {
    try {
      const fetchUserByIdUseCase: FetchUserById =
        container.get<FetchUserById>(FetchUserByIdUseCase)
      const { userId } = await params

      const user = await fetchUserByIdUseCase.execute(userId)

      return NextResponse.json(user)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const DELETE = applyMiddleware(
  [loggerMiddleware({ operationName: 'deleteUser', method: 'DELETE' })],
  async (request: Request, { params }: { params: { userId: string } }) => {
    try {
      const deleteUserUseCase: DeleteUser =
        container.get<DeleteUser>(DeleteUserUseCase)
      const { userId } = await params

      await deleteUserUseCase.execute(userId)

      return NextResponse.json({}, { status: 200 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const PATCH = applyMiddleware(
  [loggerMiddleware({ operationName: 'updateUser', method: 'PATCH' })],
  async (request: Request, { params }: { params: { userId: string } }) => {
    try {
      const updateUserUseCase: UpdateUser =
        container.get<UpdateUser>(UpdateUserUseCase)
      const { userId } = await params
      const body = await request.json()

      const userUpdated = await updateUserUseCase.execute(userId, body)

      return NextResponse.json({ userUpdated })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
