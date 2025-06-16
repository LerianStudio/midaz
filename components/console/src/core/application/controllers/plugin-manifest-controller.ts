import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { injectable, inject } from 'inversify'
import { NextResponse } from 'next/server'
import { AddPluginMenuUseCase } from '../use-cases/plugin-mainfest/add-plugin-menu-use-case'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'

@injectable()
@LoggerInterceptor()
@Controller()
export class PluginMenuController {
  constructor(
    @inject(AddPluginMenuUseCase)
    private readonly addPluginMenuUseCase: AddPluginMenuUseCase
  ) {}

  async addPluginManifest(request: Request) {
    try {
      const body = await request.json()
      const pluginMenu = await this.addPluginMenuUseCase.execute(body)

      return NextResponse.json(pluginMenu)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
}
