import { VersionRepository } from '@/core/domain/repositories/version-repository'
import { inject, injectable } from 'inversify'
import { VersionDto } from '../../dto/version-dto'
import { LogOperation } from '@/core/infrastructure/logger/decorators'

export interface GetVersion {
  execute: () => Promise<VersionDto>
}

@injectable()
export class GetVersionUseCase implements GetVersion {
  constructor(
    @inject(VersionRepository)
    private readonly versionRepository: VersionRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(): Promise<VersionDto> {
    const current = process.env.VERSION || ''
    const versionEntity = await this.versionRepository.getLatestVersion()

    return {
      current,
      latest: versionEntity.latest,
      status: await this.versionRepository.getVersionStatus(
        versionEntity.latest,
        current
      )
    }
  }
}
