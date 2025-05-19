import { inject, injectable } from 'inversify'
import { ApplicationResponseDto } from '../../dto/application-dto'
import { ApplicationRepository } from '@/core/domain/repositories/identity/application-repository'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { ApplicationMapper } from '../../mappers/application-mapper'

export interface FetchApplicationById {
  execute: (applicationId: string) => Promise<ApplicationResponseDto>
}

@injectable()
export class FetchApplicationByIdUseCase implements FetchApplicationById {
  constructor(
    @inject(ApplicationRepository)
    private readonly applicationRepository: ApplicationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(applicationId: string): Promise<ApplicationResponseDto> {
    const application =
      await this.applicationRepository.fetchById(applicationId)

    return ApplicationMapper.toResponseDto(application)
  }
}
