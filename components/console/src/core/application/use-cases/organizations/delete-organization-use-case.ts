import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { OrganizationAvatarRepository } from '@/core/domain/repositories/organization-avatar-repository'

export interface DeleteOrganization {
  execute(organizationId: string): Promise<void>
}

@injectable()
export class DeleteOrganizationUseCase implements DeleteOrganization {
  constructor(
    @inject(OrganizationRepository)
    private readonly organizationRepository: OrganizationRepository,
    @inject(OrganizationAvatarRepository)
    private readonly organizationAvatarRepository: OrganizationAvatarRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId: string): Promise<void> {
    await this.organizationRepository.delete(organizationId)
    await this.organizationAvatarRepository.delete(organizationId)
  }
}
