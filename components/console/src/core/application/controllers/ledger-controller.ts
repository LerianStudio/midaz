import { BaseController } from '@/lib/http/server/base-controller'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { Get, Param, Query } from '@/lib/http/server'
import { type LedgerSearchParamDto } from '../dto/ledger-dto'
import { FetchAllLedgersUseCase } from '../use-cases/ledgers/fetch-all-ledgers-use-case'
import { FetchAllLedgersAssetsUseCase } from '../use-cases/ledgers-assets/fetch-ledger-assets-use-case'
import { NextResponse } from 'next/server'
import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'

@LoggerInterceptor()
@Controller()
export class LedgerController extends BaseController {
  constructor(
    @inject(FetchAllLedgersUseCase)
    private readonly fetchAllLedgersUseCase: FetchAllLedgersUseCase,
    @inject(FetchAllLedgersAssetsUseCase)
    private readonly fetchAllLedgersAssetsUseCase: FetchAllLedgersAssetsUseCase
  ) {
    super()
  }

  @Get()
  async fetchAll(
    @Param('id') organizationId: string,
    @Query() query: LedgerSearchParamDto
  ) {
    const ledgers = await this.fetchAllLedgersUseCase.execute(
      organizationId,
      query
    )

    return NextResponse.json(ledgers)
  }

  @Get()
  async fetchWithAssets(
    @Param('id') organizationId: string,
    @Query() query: LedgerSearchParamDto
  ) {
    const ledgers = await this.fetchAllLedgersAssetsUseCase.execute(
      organizationId,
      query
    )

    return NextResponse.json(ledgers)
  }
}
