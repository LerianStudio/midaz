import { PluginManifestEntity } from '@/core/domain/entities/plugin-manifest-entity'
import { ServiceDiscoveryRepository } from '@/core/domain/repositories/plugin/service-discovery-repository'
import { inject, injectable } from 'inversify'
import { PluginManifestMapper } from '../mappers/plugin-manifest-mapper'
import { PluginManifestHttpService } from '../services/plugin-manifest-http-service'
import { HttpService } from '@/lib/http'

@injectable()
export class PluginServiceDiscoveryRepository
  implements ServiceDiscoveryRepository
{
  constructor(
    @inject(PluginManifestHttpService)
    private readonly httpService: HttpService
  ) {}

  async fetchPluginManifest(
    pluginManifestUrl: string
  ): Promise<PluginManifestEntity> {
    const response =
      await this.httpService.get<PluginManifestEntity>(pluginManifestUrl)

    const manifestEntity = PluginManifestMapper.toEntity(response)
    return manifestEntity
  }
}
