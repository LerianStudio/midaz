import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject, injectable } from 'inversify'
import { GetMidazInfoUseCase } from '../use-cases/midaz-info/get-version'
import { NextResponse } from 'next/server'

@injectable()
@LoggerInterceptor()
@Controller()
export class MidazInfoController {
  constructor(
    @inject(GetMidazInfoUseCase)
    private readonly getMidazInfoUseCase: GetMidazInfoUseCase
  ) {}

  /**
   * Returns the current version of the application.
   * @returns The current version as a string.
   */
  async getVersion() {
    const info = await this.getMidazInfoUseCase.execute()
    return NextResponse.json(info)
  }
}
