import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { GetMidazConfigValidationUseCase } from '../use-cases/midaz-config/get-config-validation'
import { NextResponse } from 'next/server'
import { BaseController } from '@/lib/http/server/base-controller'
import { NextRequest } from 'next/server'

@LoggerInterceptor()
@Controller()
export class MidazConfigController extends BaseController {
  constructor(
    @inject(GetMidazConfigValidationUseCase)
    private readonly getMidazConfigValidationUseCase: GetMidazConfigValidationUseCase
  ) {
    super()
  }

  async getConfigValidation(request: NextRequest) {
    const { searchParams } = new URL(request.url)
    const organization = searchParams.get('organization')
    const ledger = searchParams.get('ledger')

    if (!organization || !ledger) {
      return NextResponse.json(
        { error: 'Organization and ledger are required' },
        { status: 400 }
      )
    }

    const validation = await this.getMidazConfigValidationUseCase.execute(organization, ledger)
    return NextResponse.json(validation)
  }
}