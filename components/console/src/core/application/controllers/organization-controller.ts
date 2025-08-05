import { inject } from 'inversify'
import { Controller, Get, Query } from '@/lib/http/server'
import { BaseController } from '@/lib/http/server/base-controller'
import { type OrganizationSearchParamDto } from '../dto/organization-dto'
import { NextResponse } from 'next/server'
import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { FetchAllOrganizationsUseCase } from '../use-cases/organizations/fetch-all-organizations-use-case'

@LoggerInterceptor()
@Controller()
export class OrganizationController extends BaseController {
  constructor(
    @inject(FetchAllOrganizationsUseCase)
    private readonly fetchAllOrganizationsUseCase: FetchAllOrganizationsUseCase
  ) {
    super()
  }

  @Get()
  async fetchAll(@Query() query: OrganizationSearchParamDto) {
    const response = await this.fetchAllOrganizationsUseCase.execute(query)

    return NextResponse.json(response)
  }
}
