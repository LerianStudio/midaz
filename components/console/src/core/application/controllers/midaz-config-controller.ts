import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { GetMidazConfigValidationUseCase } from '../use-cases/midaz-config/get-config-validation'
import { NextResponse } from 'next/server'
import { BaseController } from '@/lib/http/server/base-controller'

@LoggerInterceptor()
@Controller()
export class MidazConfigController extends BaseController {
  constructor(
    @inject(GetMidazConfigValidationUseCase)
    private readonly getMidazConfigValidationUseCase: GetMidazConfigValidationUseCase
  ) {
    super()
  }

  async getConfigValidation() {
    const config = await this.getMidazConfigValidationUseCase.execute()
    return NextResponse.json(config)
  }
}
