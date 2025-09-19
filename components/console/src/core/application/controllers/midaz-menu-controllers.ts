import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { inject } from 'inversify'
import { GetMidazMenusUseCase } from '../use-cases/midaz-menu/get-midaz-menus-use-case'
import { Controller, Get } from '@/lib/http/server'
import { NextResponse } from 'next/server'
import { BaseController } from '@/lib/http/server/base-controller'

@LoggerInterceptor()
@Controller()
export class MidazMenuController extends BaseController {
  constructor(
    @inject(GetMidazMenusUseCase)
    private readonly getMidazMenusUseCase: GetMidazMenusUseCase
  ) {
    super()
  }

  @Get()
  async getMidazMenus() {
    const midazMenus = await this.getMidazMenusUseCase.execute()
    return NextResponse.json(midazMenus)
  }
}
