import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { NextResponse } from 'next/server'
import {
  type AddPluginMenu,
  AddPluginMenuUseCase
} from '../use-cases/plugin-mainfest/add-plugin-menu-use-case'
import z from 'zod'
import { ValidateZod } from '@/lib/zod/decorators/validate-zod'
import { BaseController } from '@/lib/http/server/base-controller'

const AddPluginManifestSchema = z.object({
  host: z.string().min(5).max(255)
})

@LoggerInterceptor()
@Controller()
export class PluginManifestController extends BaseController {
  constructor(
    @inject(AddPluginMenuUseCase)
    private readonly addPluginMenuUseCase: AddPluginMenu
  ) {
    super()
  }

  @ValidateZod(AddPluginManifestSchema)
  async addPluginManifest(request: Request) {
    const body = await request.json()
    const pluginMenu = await this.addPluginMenuUseCase.execute(body)

    return NextResponse.json(pluginMenu)
  }
}
