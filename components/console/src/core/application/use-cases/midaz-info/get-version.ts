import { VersionRepository } from '@/core/domain/repositories/version-repository'
import { inject, injectable } from 'inversify'
import { MidazInfoDto } from '../../dto/midaz-info-dto'
import { LogOperation } from '@/core/infrastructure/logger/decorators'

export interface GetMidazInfo {
  execute: () => Promise<MidazInfoDto>
}

@injectable()
export class GetMidazInfoUseCase implements GetMidazInfo {
  constructor(
    @inject(VersionRepository)
    private readonly versionRepository: VersionRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(): Promise<MidazInfoDto> {
    const currentVersion = process.env.VERSION || '1.0.0'
    const versionEntity = await this.versionRepository.getLatestVersion()

    return {
      currentVersion,
      latestVersion: versionEntity.latest,
      versionStatus: await this.versionRepository.getVersionStatus(
        versionEntity.latest,
        currentVersion
      )
    }
  }
}
