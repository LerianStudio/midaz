import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { GetMidazInfoUseCase } from '../use-cases/midaz-info/get-version'
import { NextResponse } from 'next/server'
import { BaseController } from '@/lib/http/server/base-controller'

@LoggerInterceptor()
@Controller()
export class MidazInfoController extends BaseController {
  constructor(
    @inject(GetMidazInfoUseCase)
    private readonly getMidazInfoUseCase: GetMidazInfoUseCase
  ) {
    super()
  }

  /**
   * Returns the current version of the application.
   * @returns The current version as a string.
   */
  async getVersion() {
    const info = await this.getMidazInfoUseCase.execute()
    return NextResponse.json(info)
  }
}
