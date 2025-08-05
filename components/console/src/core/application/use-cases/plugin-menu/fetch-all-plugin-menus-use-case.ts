import { inject } from 'inversify'
import { PluginManifestDto } from '../../dto/plugin-manifest-dto'
import { PluginManifestRepository } from '@/core/domain/repositories/plugin/plugin-manifest-repository'
import { PluginManifestMapper } from '../../mappers/plugin-manifest-mapper'

export interface FetchAllPluginMenus {
  execute: () => Promise<PluginManifestDto[]>
}

export class FetchAllPluginMenusUseCase implements FetchAllPluginMenus {
  constructor(
    @inject(PluginManifestRepository)
    private readonly pluginMenuRepository: PluginManifestRepository
  ) {}

  async execute(): Promise<PluginManifestDto[]> {
    const pluginMenus = await this.pluginMenuRepository.fetchAll()

    return pluginMenus.map(PluginManifestMapper.toResponseDto)
  }
}
