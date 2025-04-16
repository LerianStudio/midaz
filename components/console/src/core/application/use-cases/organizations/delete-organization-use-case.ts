import { DeleteOrganizationRepository } from '@/core/domain/repositories/organizations/delete-organization-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface DeleteOrganization {
  execute(organizationId: string): Promise<void>
}

@injectable()
export class DeleteOrganizationUseCase implements DeleteOrganization {
  constructor(
    @inject(DeleteOrganizationRepository)
    private readonly deleteOrganizationRepository: DeleteOrganizationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId: string): Promise<void> {
    await this.deleteOrganizationRepository.deleteOrganization(organizationId)
  }
}
