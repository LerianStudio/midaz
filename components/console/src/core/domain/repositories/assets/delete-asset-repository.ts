export abstract class DeleteAssetRepository {
  abstract delete: (
    organizationId: string,
    ledgerId: string,
    assetId: string
  ) => Promise<void>
}
