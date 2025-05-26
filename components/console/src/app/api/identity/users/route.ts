import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { apiErrorHandler } from '../../utils/api-error-handler'
import {
  FetchAllUsers,
  FetchAllUsersUseCase
} from '@/core/application/use-cases/users/fetch-all-users-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { NextResponse } from 'next/server'
import {
  CreateUser,
  CreateUserUseCase
} from '@/core/application/use-cases/users/create-user-use-case'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllUsers',
      method: 'GET'
    })
  ],
  async (request: Request) => {
    try {
      const fetchAllUsersUseCase: FetchAllUsers =
        container.get<FetchAllUsers>(FetchAllUsersUseCase)

      const users = await fetchAllUsersUseCase.execute()

      return NextResponse.json(users)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createUser',
      method: 'POST'
    })
  ],
  async (request: Request) => {
    try {
      const createUserUseCase: CreateUser =
        container.get<CreateUser>(CreateUserUseCase)
      const body = await request.json()
      const userCreated = await createUserUseCase.execute(body)

      return NextResponse.json({ userCreated }, { status: 201 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
