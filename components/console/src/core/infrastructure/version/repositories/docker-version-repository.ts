import {
  VersionEntity,
  VersionStatus
} from '@/core/domain/entities/version-entity'
import { VersionRepository } from '@/core/domain/repositories/version-repository'
import { createQueryString } from '@/lib/search'
import { injectable } from 'inversify'
import {
  DockerTagDto,
  DockerListRepositoryTagsDto
} from '../dto/docker-tag-dto'
import { gt as semverGt } from 'semver'
import { NotFoundApiException } from '@/lib/http'

@injectable()
export class DockerVersionRepository implements VersionRepository {
  private baseUrl =
    'https://hub.docker.com/v2/repositories/lerianstudio/midaz-console'

  async getVersionStatus(latest: string, current: string) {
    if (!latest || !current) {
      return VersionStatus.UpToDate
    }

    if (latest === current) {
      return VersionStatus.UpToDate
    }

    return semverGt(latest, current)
      ? VersionStatus.Outdated
      : VersionStatus.UpToDate
  }

  /**
   * Returns the current version of the application.
   * @returns The current version as a string.
   */
  async getLatestVersion(): Promise<VersionEntity> {
    const latest = await this._batchFetch(
      `${this.baseUrl}/tags${createQueryString({
        page_size: 100
      })}`
    )

    if (!latest) {
      throw new NotFoundApiException('No latest version found')
    }

    return {
      latest: latest.name
    }
  }

  private async _batchFetch(url: string): Promise<DockerTagDto | null> {
    const response = await fetch(url, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json'
      }
    })

    const data: DockerListRepositoryTagsDto = await response.json()

    const latest = this._findLatest(data.results)

    if (latest) {
      return latest
    }

    if (!data.next) {
      return null
    }

    return await this._batchFetch(data.next || '')
  }

  private _findLatest(tags: DockerTagDto[]) {
    return tags.find(
      (tag) =>
        tag.tag_status === 'active' &&
        tag.name.match(`^(?:(?!latest|alpha|beta).)*$`)
    )
  }
}
