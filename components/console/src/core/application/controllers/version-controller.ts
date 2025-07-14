import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject, injectable } from 'inversify'
import { GetVersionUseCase } from '../use-cases/version/get-version'
import { NextResponse } from 'next/server'

@injectable()
@LoggerInterceptor()
@Controller()
export class VersionController {
  constructor(
    @inject(GetVersionUseCase)
    private readonly getVersionUseCase: GetVersionUseCase
  ) {}

  /**
   * Returns the current version of the application.
   * @returns The current version as a string.
   */
  async getVersion() {
    const version = await this.getVersionUseCase.execute()
    return NextResponse.json(version)
  }
}
