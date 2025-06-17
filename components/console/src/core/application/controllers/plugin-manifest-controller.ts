import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject, injectable } from 'inversify'
import { NextResponse } from 'next/server'
import {
  type AddPluginMenu,
  AddPluginMenuUseCase
} from '../use-cases/plugin-mainfest/add-plugin-menu-use-case'
import z from 'zod'
import { ValidateZod } from '@/lib/zod/decorators/validate-zod'

const AddPluginManifestSchema = z.object({
  host: z.string().url()
})

@injectable()
@LoggerInterceptor()
@Controller()
export class PluginManifestController {
  constructor(
    @inject(AddPluginMenuUseCase)
    private readonly addPluginMenuUseCase: AddPluginMenu
  ) {}

  @ValidateZod(AddPluginManifestSchema)
  async addPluginManifest(request: Request) {
    const body = await request.json()
    const pluginMenu = await this.addPluginMenuUseCase.execute(body)

    return NextResponse.json(pluginMenu)
  }
}
