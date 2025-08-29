import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { NextResponse } from 'next/server'
import { FetchOperationRoutesByIdUseCase } from '../use-cases/operation-routes/fetch-operation-routes-use-case'
import { FetchAllOperationRoutesUseCase } from '../use-cases/operation-routes/fetch-all-operation-routes-use-case'
import { CreateOperationRoutesUseCase } from '../use-cases/operation-routes/create-operation-routes-use-case'
import { DeleteOperationRoutesUseCase } from '../use-cases/operation-routes/delete-operation-routes-use-case'
import { UpdateOperationRoutesUseCase } from '../use-cases/operation-routes/update-operation-routes-use-case'
import { BaseController } from '@/lib/http/server/base-controller'

type OperationRoutesParams = {
  id: string
  ledgerId: string
  operationRouteId?: string
}

@LoggerInterceptor()
@Controller()
export class OperationRoutesController extends BaseController {
  constructor(
    @inject(FetchAllOperationRoutesUseCase)
    private readonly fetchAllOperationRoutesUseCase: FetchAllOperationRoutesUseCase,
    @inject(FetchOperationRoutesByIdUseCase)
    private readonly fetchOperationRoutesUseCase: FetchOperationRoutesByIdUseCase,
    @inject(CreateOperationRoutesUseCase)
    private readonly createOperationRoutesUseCase: CreateOperationRoutesUseCase,
    @inject(UpdateOperationRoutesUseCase)
    private readonly updateOperationRoutesUseCase: UpdateOperationRoutesUseCase,
    @inject(DeleteOperationRoutesUseCase)
    private readonly deleteOperationRoutesUseCase: DeleteOperationRoutesUseCase
  ) {
    super()
  }

  async fetchById(
    request: Request,
    { params }: { params: OperationRoutesParams }
  ) {
    const { id: organizationId, ledgerId, operationRouteId } = await params

    const operationRoute = await this.fetchOperationRoutesUseCase.execute(
      organizationId,
      ledgerId,
      operationRouteId!
    )

    return NextResponse.json(operationRoute)
  }

  async fetchAll(
    request: Request,
    { params }: { params: OperationRoutesParams }
  ) {
    const { searchParams } = new URL(request.url)
    const { id: organizationId, ledgerId } = await params
    const limit = Number(searchParams.get('limit')) || 10
    const page = Number(searchParams.get('page')) || 1

    const operationRoutes = await this.fetchAllOperationRoutesUseCase.execute(
      organizationId,
      ledgerId,
      {
        limit,
        page
      }
    )

    return NextResponse.json(operationRoutes)
  }

  async create(
    request: Request,
    { params }: { params: OperationRoutesParams }
  ) {
    const body = await request.json()
    const { id: organizationId, ledgerId } = await params

    const operationRoute = await this.createOperationRoutesUseCase.execute(
      organizationId,
      ledgerId,
      body
    )

    return NextResponse.json(operationRoute)
  }

  async update(
    request: Request,
    { params }: { params: OperationRoutesParams }
  ) {
    const body = await request.json()
    const { id: organizationId, ledgerId, operationRouteId } = await params

    const { metadata, ...restBody } = body
    const updateData = metadata === null ? restBody : body

    const operationRouteUpdated =
      await this.updateOperationRoutesUseCase.execute(
        organizationId,
        ledgerId,
        operationRouteId!,
        updateData
      )

    return NextResponse.json(operationRouteUpdated)
  }

  async delete(
    request: Request,
    { params }: { params: OperationRoutesParams }
  ) {
    const { id: organizationId, ledgerId, operationRouteId } = await params

    await this.deleteOperationRoutesUseCase.execute(
      organizationId,
      ledgerId,
      operationRouteId!
    )

    return NextResponse.json({}, { status: 200 })
  }
}
