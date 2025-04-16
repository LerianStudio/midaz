import { MidazFetchAllAssetsRepository } from './midaz-fetch-all-assets-repository'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    POST: 'GET'
  }
}))

describe('MidazFetchAllAssetsRepository', () => {
  let repository: MidazFetchAllAssetsRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazFetchAllAssetsRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should fetch all assets successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const limit = 10
    const page = 1
    const response: PaginationEntity<AssetEntity> = {
      items: [
        {
          id: 'asset123',
          name: 'Asset Name 1',
          type: 'Asset Type',
          code: 'Asset Code',
          status: { code: 'active', description: 'Active' },
          metadata: { key: 'value' }
        },
        {
          id: 'asset1234',
          name: 'Asset Name 2',
          type: 'Asset Type',
          code: 'Asset Code',
          status: { code: 'active', description: 'Active' },
          metadata: { key: 'value' }
        }
      ],
      limit,
      page
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/assets?limit=${limit}&page=${page}&type=&code=`,
      method: HTTP_METHODS.GET
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when fetching all assets', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const limit = 10
    const page = 1
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.fetchAll(organizationId, ledgerId, limit, page)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/assets?limit=${limit}&page=${page}&type=&code=`,
      method: HTTP_METHODS.GET
    })
  })
})
