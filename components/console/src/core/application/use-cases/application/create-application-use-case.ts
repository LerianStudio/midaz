import { ApplicationRepository } from '@/core/domain/repositories/identity/application-repository'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { inject, injectable } from 'inversify'
import type {
  ApplicationResponseDto,
  CreateApplicationDto
} from '../../dto/application-dto'
import { ApplicationMapper } from '../../mappers/application-mapper'

export interface CreateApplication {
  execute: (
    application: CreateApplicationDto
  ) => Promise<ApplicationResponseDto>
}

@injectable()
export class CreateApplicationUseCase implements CreateApplication {
  constructor(
    @inject(ApplicationRepository)
    private readonly applicationRepository: ApplicationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    application: CreateApplicationDto
  ): Promise<ApplicationResponseDto> {
    const applicationEntity = ApplicationMapper.toDomain(application)
    const applicationCreated =
      await this.applicationRepository.create(applicationEntity)
    return ApplicationMapper.toResponseDto(applicationCreated)
  }
}
