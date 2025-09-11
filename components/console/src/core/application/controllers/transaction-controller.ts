import { inject } from 'inversify'
import { FetchAllTransactionsUseCase } from '../use-cases/transactions/fetch-all-transactions-use-case'
import { Controller } from '@/lib/http/server'
import { NextResponse } from 'next/server'
import { BaseController } from '@/lib/http/server/base-controller'
import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators/logger-interceptor-decorator'

@LoggerInterceptor()
@Controller()
export class TransactionController extends BaseController {
  constructor(
    @inject(FetchAllTransactionsUseCase)
    private readonly fetchAllTransactionsUseCase: FetchAllTransactionsUseCase
  ) {
    super()
  }

  async fetchAll(
    request: Request,
    { params }: { params: { id: string; ledgerId: string } }
  ) {
    const { searchParams } = new URL(request.url)
    const { id: organizationId, ledgerId } = await params

    // Check if cursor-based pagination is requested
    const cursor = searchParams.get('cursor')
    const sortOrder = searchParams.get('sort_order') as 'asc' | 'desc'
    const sortBy = searchParams.get('sort_by') as
      | 'id'
      | 'createdAt'
      | 'updatedAt'
    const id = searchParams.get('id')
    const limit = Number(searchParams.get('limit')) || 10

    // Always use cursor pagination - remove page-based pagination entirely
    const transactions = await this.fetchAllTransactionsUseCase.execute(
      organizationId,
      ledgerId,
      {
        cursor: cursor || undefined,
        limit,
        sortOrder: sortOrder || 'desc',
        sortBy: sortBy || 'createdAt',
        id: id || undefined
      }
    )

    return NextResponse.json(transactions)
  }
}
