import { PluginManifestEntity } from '@/core/domain/entities/plugin-manifest-entity'
import { PluginManifestRepository } from '@/core/domain/repositories/plugin/plugin-manifest-repository'
import { ServiceDiscoveryRepository } from '@/core/domain/repositories/plugin/service-discovery-repository'
import { LogOperation } from '@/core/infrastructure/logger/decorators'
import { inject } from 'inversify'
import { PluginManifestDto } from '@/core/infrastructure/midaz-plugins/plugin-service-discovery/dto/plugin-manifest-dto'
import type { CreatePluginManifestDto } from '@/core/application/dto/plugin-manifest-dto'
import { PluginManifestMapper } from '../../mappers/plugin-manifest-mapper'

export interface AddPluginMenu {
  execute: (pluginMenu: CreatePluginManifestDto) => Promise<PluginManifestDto>
}

export class AddPluginMenuUseCase implements AddPluginMenu {
  constructor(
    @inject(ServiceDiscoveryRepository)
    private readonly serviceDiscoveryRepository: ServiceDiscoveryRepository,
    @inject(PluginManifestRepository)
    private readonly pluginMenuRepository: PluginManifestRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    pluginMenu: CreatePluginManifestDto
  ): Promise<PluginManifestDto> {
    const pluginManifest: PluginManifestEntity =
      await this.serviceDiscoveryRepository.fetchPluginManifest(pluginMenu.host)

    const pluginMenuCreated =
      await this.pluginMenuRepository.create(pluginManifest)

    const pluginMenuDto = PluginManifestMapper.toResponseDto(pluginMenuCreated)

    return pluginMenuDto
  }
}
