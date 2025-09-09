import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { NextResponse } from 'next/server'
import { FetchTransactionRoutesByIdUseCase } from '../use-cases/transaction-routes/fetch-transaction-routes-use-case'
import { FetchAllTransactionRoutesUseCase } from '../use-cases/transaction-routes/fetch-all-transaction-routes-use-case'
import { CreateTransactionRoutesUseCase } from '../use-cases/transaction-routes/create-transaction-routes-use-case'
import { DeleteTransactionRoutesUseCase } from '../use-cases/transaction-routes/delete-transaction-routes-use-case'
import { UpdateTransactionRoutesUseCase } from '../use-cases/transaction-routes/update-transaction-routes-use-case'
import { BaseController } from '@/lib/http/server/base-controller'
import { FetchAllTransactionRoutesWithOperationRoutesUseCase } from '../use-cases/transaction-operation-routes/fetch-all-transaction-routes-with-operation-routes-use-case'

type TransactionRoutesParams = {
  id: string
  ledgerId: string
  transactionRouteId?: string
}

@LoggerInterceptor()
@Controller()
export class TransactionRoutesController extends BaseController {
  constructor(
    @inject(FetchAllTransactionRoutesUseCase)
    private readonly fetchAllTransactionRoutesUseCase: FetchAllTransactionRoutesUseCase,
    @inject(FetchTransactionRoutesByIdUseCase)
    private readonly fetchTransactionRoutesUseCase: FetchTransactionRoutesByIdUseCase,
    @inject(CreateTransactionRoutesUseCase)
    private readonly createTransactionRoutesUseCase: CreateTransactionRoutesUseCase,
    @inject(UpdateTransactionRoutesUseCase)
    private readonly updateTransactionRoutesUseCase: UpdateTransactionRoutesUseCase,
    @inject(DeleteTransactionRoutesUseCase)
    private readonly deleteTransactionRoutesUseCase: DeleteTransactionRoutesUseCase,
    @inject(FetchAllTransactionRoutesWithOperationRoutesUseCase)
    private readonly fetchAllTransactionRoutesWithOperationRoutesUseCase: FetchAllTransactionRoutesWithOperationRoutesUseCase
  ) {
    super()
  }

  async fetchById(
    request: Request,
    { params }: { params: TransactionRoutesParams }
  ) {
    const { id: organizationId, ledgerId, transactionRouteId } = await params

    const transactionRoute = await this.fetchTransactionRoutesUseCase.execute(
      organizationId,
      ledgerId,
      transactionRouteId!
    )

    return NextResponse.json(transactionRoute)
  }

  async fetchAll(
    request: Request,
    { params }: { params: TransactionRoutesParams }
  ) {
    const { searchParams } = new URL(request.url)
    const { id: organizationId, ledgerId } = await params

    // Check if cursor-based pagination is requested
    const cursor = searchParams.get('cursor')
    const sortOrder = searchParams.get('sort_order') as 'asc' | 'desc'
    const sortBy = searchParams.get('sort_by') as
      | 'id'
      | 'title'
      | 'createdAt'
      | 'updatedAt'
    const id = searchParams.get('id')
    const limit = Number(searchParams.get('limit')) || 10

    // Always use cursor pagination - remove page-based pagination entirely
    const transactionRoutes =
      await this.fetchAllTransactionRoutesWithOperationRoutesUseCase.execute(
        organizationId,
        ledgerId,
        {
          cursor: cursor || undefined,
          limit,
          sortOrder: sortOrder || 'asc',
          sortBy: sortBy || 'createdAt',
          id: id || undefined
        }
      )

    return NextResponse.json(transactionRoutes)
  }

  async create(
    request: Request,
    { params }: { params: TransactionRoutesParams }
  ) {
    const body = await request.json()
    const { id: organizationId, ledgerId } = await params

    const transactionRoute = await this.createTransactionRoutesUseCase.execute(
      organizationId,
      ledgerId,
      body
    )

    return NextResponse.json(transactionRoute)
  }

  async update(
    request: Request,
    { params }: { params: TransactionRoutesParams }
  ) {
    const body = await request.json()
    const { id: organizationId, ledgerId, transactionRouteId } = await params

    const { metadata, ...restBody } = body
    const updateData = metadata === null ? restBody : body

    const transactionRouteUpdated =
      await this.updateTransactionRoutesUseCase.execute(
        organizationId,
        ledgerId,
        transactionRouteId!,
        updateData
      )

    return NextResponse.json(transactionRouteUpdated)
  }

  async delete(
    request: Request,
    { params }: { params: TransactionRoutesParams }
  ) {
    const { id: organizationId, ledgerId, transactionRouteId } = await params

    await this.deleteTransactionRoutesUseCase.execute(
      organizationId,
      ledgerId,
      transactionRouteId!
    )

    return NextResponse.json({}, { status: 200 })
  }
}
