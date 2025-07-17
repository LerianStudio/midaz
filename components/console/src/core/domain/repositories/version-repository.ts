import { VersionEntity, VersionStatus } from '../entities/version-entity'

export abstract class VersionRepository {
  /**
   * Returns the current version of the application.
   * @returns The current version as a string.
   */
  abstract getLatestVersion(): Promise<VersionEntity>

  abstract getVersionStatus(
    latest: string,
    current: string
  ): Promise<VersionStatus>
}
