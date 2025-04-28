import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { OrganizationResponseDto } from '../../dto/organization-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchOrganizationById {
  execute: (organizationId: string) => Promise<OrganizationResponseDto>
}

@injectable()
export class FetchOrganizationByIdUseCase implements FetchOrganizationById {
  constructor(
    @inject(OrganizationRepository)
    private readonly organizationRepository: OrganizationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId: string): Promise<OrganizationResponseDto> {
    const organizationEntity =
      await this.organizationRepository.fetchById(organizationId)

    return OrganizationMapper.toResponseDto(organizationEntity)
  }
}
