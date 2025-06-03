import { inject, injectable } from 'inversify'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { OrganizationDto } from '../../dto/organization-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchParentOrganizations {
  execute(organizationId?: string): Promise<OrganizationDto[]>
}

@injectable()
export class FetchParentOrganizationsUseCase
  implements FetchParentOrganizations
{
  constructor(
    @inject(OrganizationRepository)
    private readonly organizationRepository: OrganizationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId?: string): Promise<OrganizationDto[]> {
    const organizations = await this.organizationRepository.fetchAll(100, 1)

    const parentOrganizationsFiltered = organizations.items.filter(
      (organization) => organization.id !== organizationId
    )

    const parentOrganizations: OrganizationDto[] =
      parentOrganizationsFiltered.map((organization) => {
        return OrganizationMapper.toResponseDto(organization, undefined)
      })

    return parentOrganizations
  }
}
