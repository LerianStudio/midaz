import { ApplicationRepository } from '@/core/domain/repositories/identity/application-repository'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { inject, injectable } from 'inversify'

export interface DeleteApplication {
  execute: (applicationId: string) => Promise<void>
}

@injectable()
export class DeleteApplicationUseCase implements DeleteApplication {
  constructor(
    @inject(ApplicationRepository)
    private readonly applicationRepository: ApplicationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(applicationId: string): Promise<void> {
    await this.applicationRepository.delete(applicationId)
  }
}
