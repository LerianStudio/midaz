import { FetchOrganizationByIdRepository } from '@/core/domain/repositories/organizations/fetch-organization-by-id-repository'
import { OrganizationResponseDto } from '../../dto/organization-response-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchOrganizationById {
  execute: (organizationId: string) => Promise<OrganizationResponseDto>
}

@injectable()
export class FetchOrganizationByIdUseCase implements FetchOrganizationById {
  constructor(
    @inject(FetchOrganizationByIdRepository)
    private readonly fetchOrganizationByIdRepository: FetchOrganizationByIdRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId: string): Promise<OrganizationResponseDto> {
    const organizationEntity =
      await this.fetchOrganizationByIdRepository.fetchById(organizationId)

    return OrganizationMapper.toResponseDto(organizationEntity)
  }
}
