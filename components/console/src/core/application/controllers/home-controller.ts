import { inject } from 'inversify'
import { FetchHomeMetricsUseCase } from '../use-cases/home/fetch-home-metrics-use-case'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { NextResponse } from 'next/server'
import { BaseController } from '@/lib/http/server/base-controller'

@LoggerInterceptor()
@Controller()
export class HomeController extends BaseController {
  constructor(
    @inject(FetchHomeMetricsUseCase)
    private readonly fetchHomeMetricsUseCase: FetchHomeMetricsUseCase
  ) {
    super()
  }

  async getMetrics(request: Request) {
    const { searchParams } = new URL(request.url)
    const organizationId = searchParams.get('organizationId') || ''
    const ledgerId = searchParams.get('ledgerId') || ''

    const metrics = await this.fetchHomeMetricsUseCase.execute(
      organizationId,
      ledgerId
    )

    return NextResponse.json(metrics)
  }
}
