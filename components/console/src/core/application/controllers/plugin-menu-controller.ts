import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { NextResponse } from 'next/server'
import {
  type FetchAllPluginMenus,
  FetchAllPluginMenusUseCase
} from '../use-cases/plugin-menu/fetch-all-plugin-menus-use-case'
import { BaseController } from '@/lib/http/server/base-controller'

@LoggerInterceptor()
@Controller()
export class PluginMenuController extends BaseController {
  constructor(
    @inject(FetchAllPluginMenusUseCase)
    private readonly fetchAllPluginMenusUseCase: FetchAllPluginMenus
  ) {
    super()
  }

  async fetchAllPluginMenus() {
    const pluginMenus = await this.fetchAllPluginMenusUseCase.execute()

    return NextResponse.json(pluginMenus)
  }
}
