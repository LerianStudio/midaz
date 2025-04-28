import { DeleteAssetRepository } from '@/core/domain/repositories/assets/delete-asset-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(DeleteAssetRepository)
    private readonly deleteAssetRepository: DeleteAssetRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    assetId: string
  ): Promise<void> {
    await this.deleteAssetRepository.delete(organizationId, ledgerId, assetId)
  }
}
