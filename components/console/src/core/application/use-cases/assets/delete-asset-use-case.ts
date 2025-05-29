import { AssetRepository } from '@/core/domain/repositories/asset-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface DeleteAsset {
  execute: (
    organizationId: string,
    ledgerId: string,
    assetId: string
  ) => Promise<void>
}

@injectable()
export class DeleteAssetUseCase implements DeleteAsset {
  constructor(
    @inject(AssetRepository)
    private readonly assetRepository: AssetRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    assetId: string
  ): Promise<void> {
    await this.assetRepository.delete(organizationId, ledgerId, assetId)
  }
}
