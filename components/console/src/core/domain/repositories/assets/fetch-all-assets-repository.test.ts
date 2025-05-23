import { FetchAllAssetsRepository } from './fetch-all-assets-repository'
import { AssetEntity } from '../../entities/asset-entity'
import { PaginationEntity } from '../../entities/pagination-entity'

describe('FetchAllAssetsRepository', () => {
  let fetchAllAssetsRepository: FetchAllAssetsRepository

  beforeEach(() => {
    fetchAllAssetsRepository = {
      fetchAll: jest.fn()
    }
  })

  it('should fetch all assets with required parameters', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const limit = 10
    const page = 1
    const asset: AssetEntity[] = [
      {
        id: 'asset123',
        organizationId: 'org123',
        ledgerId: 'ledger123',
        name: 'Asset Name',
        type: 'Asset Type',
        code: 'Asset Code',
        status: { code: 'active', description: 'Active' },
        metadata: { key: 'value' },
        createdAt: new Date(),
        updatedAt: new Date(),
        deletedAt: null
      },
      {
        id: 'asset456',
        organizationId: 'org123',
        ledgerId: 'ledger123',
        name: 'Asset Name',
        type: 'Asset Type',
        code: 'Asset Code',
        status: { code: 'active', description: 'Active' },
        metadata: { key: 'value' },
        createdAt: new Date(),
        updatedAt: new Date(),
        deletedAt: null
      }
    ]

    const expectedResponse: PaginationEntity<AssetEntity> = {
      items: asset,
      limit,
      page
    }

    ;(fetchAllAssetsRepository.fetchAll as jest.Mock).mockResolvedValue(
      expectedResponse
    )

    const result = await fetchAllAssetsRepository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page
    )

    expect(fetchAllAssetsRepository.fetchAll).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      limit,
      page
    )
    expect(result).toEqual(expectedResponse)
  })

  it('should fetch all assets with optional parameters', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const limit = 10
    const page = 1
    const type = 'type1'
    const code = 'code1'
    const metadata = { key: 'value' }

    const expectedResponse: PaginationEntity<AssetEntity> = {
      items: [],
      limit,
      page
    }

    ;(fetchAllAssetsRepository.fetchAll as jest.Mock).mockResolvedValue(
      expectedResponse
    )

    const result = await fetchAllAssetsRepository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page,
      type,
      code,
      metadata
    )

    expect(fetchAllAssetsRepository.fetchAll).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      limit,
      page,
      type,
      code,
      metadata
    )
    expect(result).toEqual(expectedResponse)
  })
})
