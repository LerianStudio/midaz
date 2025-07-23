import { Controller, Get } from '@/lib/http/server'
import { BaseController } from '@/lib/http/server/base-controller'
import { inject } from 'inversify'
import { AuthPermissionUseCase } from '../use-cases/auth/auth-permission-use-case'
import { NextResponse } from 'next/server'
import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'

@LoggerInterceptor()
@Controller()
export class PermissionController extends BaseController {
  constructor(
    @inject(AuthPermissionUseCase)
    private readonly authPermissionUseCase: AuthPermissionUseCase
  ) {
    super()
  }

  @Get()
  async fetch() {
    const permissions = await this.authPermissionUseCase.execute()

    return NextResponse.json(permissions)
  }
}
