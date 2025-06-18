import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject, injectable } from 'inversify'

import { NextResponse } from 'next/server'
import {
  type FetchAllPluginMenus,
  FetchAllPluginMenusUseCase
} from '../use-cases/plugin-menu/fetch-all-plugin-menus-use-case'

@injectable()
@LoggerInterceptor()
@Controller()
export class PluginMenuController {
  constructor(
    @inject(FetchAllPluginMenusUseCase)
    private readonly fetchAllPluginMenusUseCase: FetchAllPluginMenus
  ) {}

  async fetchAllPluginMenus() {
    const pluginMenus = await this.fetchAllPluginMenusUseCase.execute()

    return NextResponse.json(pluginMenus)
  }
}
