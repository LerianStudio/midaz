import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface DeleteOrganization {
  execute(organizationId: string): Promise<void>
}

@injectable()
export class DeleteOrganizationUseCase implements DeleteOrganization {
  constructor(
    @inject(OrganizationRepository)
    private readonly organizationRepository: OrganizationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId: string): Promise<void> {
    await this.organizationRepository.delete(organizationId)
  }
}
