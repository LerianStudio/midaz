import { inject, injectable } from 'inversify'
import { ApplicationResponseDto } from '../../dto/application-dto'
import { PaginationDto } from '../../dto/pagination-dto'
import { ApplicationRepository } from '@/core/domain/repositories/identity/application-repository'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { ApplicationMapper } from '../../mappers/application-mapper'

export interface FetchAllApplications {
  execute: () => Promise<PaginationDto<ApplicationResponseDto>>
}

@injectable()
export class FetchAllApplicationsUseCase implements FetchAllApplications {
  constructor(
    @inject(ApplicationRepository)
    private readonly applicationRepository: ApplicationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(): Promise<PaginationDto<ApplicationResponseDto>> {
    const applications = await this.applicationRepository.fetchAll()

    return ApplicationMapper.toPaginationResponseDto(applications)
  }
}
