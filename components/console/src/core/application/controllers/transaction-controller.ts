import { inject, injectable } from 'inversify'
import { FetchAllTransactionsUseCase } from '../use-cases/transactions/fetch-all-transactions-use-case'
import { Controller, Get, Param, Query } from '@/lib/http/server'
import { type TransactionSearchDto } from '../dto/transaction-dto'
import { NextResponse } from 'next/server'
import { BaseController } from '@/lib/http/server/base-controller'

@Controller()
export class TransactionController extends BaseController {
  constructor(
    @inject(FetchAllTransactionsUseCase)
    private readonly fetchAllTransactionsUseCase: FetchAllTransactionsUseCase
  ) {
    super()
  }

  @Get()
  async fetchAll(
    @Param('id') organizationId: string,
    @Param('ledgerId') ledgerId: string,
    @Query() query: TransactionSearchDto
  ) {
    const transactions = await this.fetchAllTransactionsUseCase.execute(
      organizationId,
      ledgerId,
      query
    )

    return NextResponse.json(transactions)
  }
}
