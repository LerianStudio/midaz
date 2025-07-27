import { inject, injectable } from 'inversify'
import { ApplicationResponseDto } from '../../dto/application-dto'
import { ApplicationRepository } from '@/core/domain/repositories/identity/application-repository'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { ApplicationMapper } from '../../mappers/application-mapper'

export interface FetchAllApplications {
  execute: () => Promise<ApplicationResponseDto[]>
}

@injectable()
export class FetchAllApplicationsUseCase implements FetchAllApplications {
  constructor(
    @inject(ApplicationRepository)
    private readonly applicationRepository: ApplicationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(): Promise<ApplicationResponseDto[]> {
    const applications = await this.applicationRepository.fetchAll()

    const applicationsResponseDto: ApplicationResponseDto[] = applications.map(
      ApplicationMapper.toResponseDto
    )

    return applicationsResponseDto
  }
}
