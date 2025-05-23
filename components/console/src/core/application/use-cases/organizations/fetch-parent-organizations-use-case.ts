import { FetchAllOrganizationsRepository } from '@/core/domain/repositories/organizations/fetch-all-organizations-repository'
import { OrganizationResponseDto } from '../../dto/organization-response-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchParentOrganizations {
  execute(organizationId?: string): Promise<OrganizationResponseDto[]>
}

@injectable()
export class FetchParentOrganizationsUseCase
  implements FetchParentOrganizations
{
  constructor(
    @inject(FetchAllOrganizationsRepository)
    private readonly fetchAllOrganizationsRepository: FetchAllOrganizationsRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId?: string): Promise<OrganizationResponseDto[]> {
    const organizations = await this.fetchAllOrganizationsRepository.fetchAll(
      100,
      1
    )

    const parentOrganizationsFiltered = organizations.items.filter(
      (organization) => organization.id !== organizationId
    )

    const parentOrganizations: OrganizationResponseDto[] =
      parentOrganizationsFiltered.map(OrganizationMapper.toResponseDto)

    return parentOrganizations
  }
}
