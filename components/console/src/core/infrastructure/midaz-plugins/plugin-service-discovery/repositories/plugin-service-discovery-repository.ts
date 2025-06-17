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
  private baseUrl: string = process.env.NGINX_BASE_PATH as string

  constructor(
    @inject(PluginManifestHttpService)
    private readonly httpService: HttpService
  ) {}

  async fetchPluginManifest(pluginHost: string): Promise<PluginManifestEntity> {
    let pluginUrl = ''
    if (process.env.NODE_ENV === 'development') {
      pluginUrl = `${pluginHost}/api/manifest`
    } else {
      pluginUrl = `${this.baseUrl}/${pluginHost}/api/manifest`
    }

    const response = await this.httpService.get<PluginManifestEntity>(pluginUrl)

    const manifestEntity = PluginManifestMapper.toEntity(response)

    return manifestEntity
  }
}
