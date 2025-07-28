// Mock all dependencies before any imports
jest.mock('../../../../utils/console-to-fee-engine-converter', () => ({
  convertConsoleToFeeEngine: jest.fn((data) => data)
}))

jest.mock('../../../../utils/fee-engine-response-transformer', () => ({
  transformFeeEngineResponse: jest.fn((response) => response)
}))

const mockCircuitBreaker = {
  execute: jest.fn(),
  isOpen: jest.fn(() => false)
}

jest.mock('../utils/circuit-breaker', () => ({
  CircuitBreaker: jest.fn(() => mockCircuitBreaker)
}))

const mockRetryPolicy = {
  execute: jest.fn((fn) => fn())
}

jest.mock('../utils/retry-policy', () => ({
  RetryPolicy: jest.fn(() => mockRetryPolicy)
}))

const mockFeePackageCache = {
  get: jest.fn(() => null),
  set: jest.fn()
}

jest.mock('../cache/fee-package-cache', () => ({
  FeePackageCache: jest.fn(() => mockFeePackageCache)
}))

// Mock the container registry and other infrastructure dependencies
jest.mock('../../container-registry/container-registry', () => ({
  container: {
    get: jest.fn()
  }
}))

jest.mock('../../next-auth/next-auth-provider', () => ({}))

import { HttpFeeRepository } from '../repositories/http-fee-repository'
import { MidazHttpService } from '@/core/infrastructure/midaz/services/midaz-http-service'
import {
  FeeCalculationRequest,
  FeeCalculationContext,
  FeeServiceUnavailableError,
  FeeConfigurationError
} from '@/core/domain/fee/fee-types'

describe('HttpFeeRepository', () => {
  let repository: HttpFeeRepository
  let mockHttpService: jest.Mocked<MidazHttpService>
  let originalEnv: NodeJS.ProcessEnv

  beforeEach(() => {
    // Save original env
    originalEnv = process.env

    // Mock environment variables
    process.env = {
      ...originalEnv,
      NEXT_PUBLIC_PLUGIN_FEES_ENABLED: 'true',
      PLUGIN_FEES_PATH: 'http://fee-service.local',
      NEXT_PUBLIC_PLUGIN_FEES_FRONTEND_URL: 'http://fee-frontend.local',
      NEXT_PUBLIC_PLUGIN_UI_BASE_PATH: '/plugin-fees-ui'
    }

    // Reset all mocks
    jest.clearAllMocks()

    // Set default mock behaviors
    mockCircuitBreaker.execute.mockImplementation((fn) => fn())
    mockCircuitBreaker.isOpen.mockReturnValue(false)
    mockRetryPolicy.execute.mockImplementation((fn) => fn())
    mockFeePackageCache.get.mockReturnValue(null)
    mockFeePackageCache.set.mockClear()

    // Create mock HTTP service
    mockHttpService = {
      post: jest.fn(),
      get: jest.fn()
    } as any

    // Create repository instance
    repository = new HttpFeeRepository(mockHttpService)
  })

  afterEach(() => {
    // Restore original env
    process.env = originalEnv
    jest.clearAllMocks()
  })

  describe('calculateFees', () => {
    const mockRequest: FeeCalculationRequest = {
      transaction: {
        description: 'Test transaction',
        send: {
          asset: 'USD',
          value: '100.00',
          source: {
            from: [
              {
                accountAlias: 'alice',
                amount: { asset: 'USD', value: '100.00' }
              }
            ]
          },
          distribute: {
            to: [
              {
                accountAlias: 'bob',
                amount: { asset: 'USD', value: '100.00' }
              }
            ]
          }
        }
      }
    }

    const mockContext: FeeCalculationContext = {
      organizationId: 'org-123',
      ledgerId: 'ledger-456',
      segmentId: 'segment-789'
    }

    it('should successfully calculate fees', async () => {
      const mockResponse = {
        transaction: {
          ...mockRequest.transaction,
          feeRules: [
            {
              feeId: 'fee-1',
              feeLabel: 'Processing Fee',
              isDeductibleFrom: false,
              creditAccount: 'fees-revenue',
              priority: 1
            }
          ]
        },
        feesApplied: true
      }

      mockHttpService.post.mockResolvedValueOnce(mockResponse)

      const result = await repository.calculateFees(mockRequest, mockContext)

      expect(result).toMatchObject({
        success: true,
        feesApplied: true,
        transaction: expect.any(Object)
      })

      expect(mockHttpService.post).toHaveBeenCalledWith(
        'http://fee-service.local/fees',
        expect.objectContaining({
          headers: expect.objectContaining({
            'X-Organization-Id': 'org-123'
          })
        })
      )
    })

    it('should throw configuration error when service not enabled', async () => {
      process.env.NEXT_PUBLIC_PLUGIN_FEES_ENABLED = 'false'

      await expect(
        repository.calculateFees(mockRequest, mockContext)
      ).rejects.toThrow(FeeConfigurationError)
    })

    it('should throw configuration error when URL not configured', async () => {
      delete process.env.PLUGIN_FEES_PATH

      await expect(
        repository.calculateFees(mockRequest, mockContext)
      ).rejects.toThrow(FeeConfigurationError)
    })

    it('should handle no fees response', async () => {
      const mockResponse = {
        feesApplied: false,
        message: 'No fees applicable',
        hasNoFees: true,
        transaction: mockRequest.transaction
      }

      mockHttpService.post.mockResolvedValueOnce(mockResponse)

      const result = await repository.calculateFees(mockRequest, mockContext)

      expect(result).toMatchObject({
        success: true,
        feesApplied: false,
        message: 'No fees applicable'
      })
    })

    it('should handle service errors with circuit breaker', async () => {
      // Mock circuit breaker to be open after failures
      mockCircuitBreaker.execute.mockRejectedValue(
        new Error('Service unavailable')
      )
      mockCircuitBreaker.isOpen.mockReturnValue(true)

      await expect(
        repository.calculateFees(mockRequest, mockContext)
      ).rejects.toThrow(FeeServiceUnavailableError)
    })
  })

  describe('getFeePackage', () => {
    const mockContext: FeeCalculationContext = {
      organizationId: 'org-123',
      ledgerId: 'ledger-456'
    }

    it('should fetch and cache fee package', async () => {
      const mockPackage = {
        id: 'pkg-123',
        name: 'Standard Package',
        description: 'Standard fee package',
        status: 'active',
        fees: {
          'fee-1': {
            feeId: 'fee-1',
            feeLabel: 'Processing Fee',
            isDeductibleFrom: false,
            creditAccount: 'fees-revenue',
            priority: 1
          }
        }
      }

      // Mock retry policy to return the fetch result directly
      mockRetryPolicy.execute.mockResolvedValueOnce(mockPackage)

      const result = await repository.getFeePackage('pkg-123', mockContext)

      expect(result).toMatchObject({
        id: 'pkg-123',
        name: 'Standard Package',
        status: 'active'
      })

      // Second call should use cache
      mockFeePackageCache.get.mockReturnValueOnce(result)

      const cachedResult = await repository.getFeePackage(
        'pkg-123',
        mockContext
      )
      expect(cachedResult).toEqual(result)
      expect(mockRetryPolicy.execute).toHaveBeenCalledTimes(1) // Should only call retry once, not twice
    })

    it('should return null for 404 responses', async () => {
      global.fetch = jest.fn().mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: 'Not Found'
      } as any)

      const result = await repository.getFeePackage('pkg-999', mockContext)

      expect(result).toBeNull()
    })

    it('should handle fetch errors gracefully', async () => {
      global.fetch = jest.fn().mockRejectedValueOnce(new Error('Network error'))

      const result = await repository.getFeePackage('pkg-123', mockContext)

      expect(result).toBeNull()
    })
  })

  describe('isHealthy', () => {
    it('should return true when service is healthy', async () => {
      mockHttpService.get.mockResolvedValueOnce({ status: 'healthy' })

      const result = await repository.isHealthy()

      expect(result).toBe(true)
      expect(mockHttpService.get).toHaveBeenCalledWith(
        'http://fee-service.local/health'
      )
    })

    it('should return false when service is unhealthy', async () => {
      mockHttpService.get.mockResolvedValueOnce({ status: 'unhealthy' })

      const result = await repository.isHealthy()

      expect(result).toBe(false)
    })

    it('should return false on error', async () => {
      mockHttpService.get.mockRejectedValueOnce(new Error('Connection failed'))

      const result = await repository.isHealthy()

      expect(result).toBe(false)
    })
  })

  describe('getServiceStatus', () => {
    it('should return enabled and configured status', async () => {
      mockHttpService.get.mockResolvedValueOnce({ status: 'healthy' })

      const result = await repository.getServiceStatus()

      expect(result).toEqual({
        enabled: true,
        configured: true,
        baseUrl: 'http://fee-service.local'
      })
    })

    it('should return disabled status', async () => {
      process.env.NEXT_PUBLIC_PLUGIN_FEES_ENABLED = 'false'

      const result = await repository.getServiceStatus()

      expect(result).toEqual({
        enabled: false,
        configured: false,
        message: 'Fee service is disabled'
      })
    })

    it('should return not configured status', async () => {
      delete process.env.PLUGIN_FEES_PATH

      const result = await repository.getServiceStatus()

      expect(result).toEqual({
        enabled: true,
        configured: false,
        message: 'Fee service URL not configured'
      })
    })

    it('should indicate when service is not responding', async () => {
      mockHttpService.get.mockRejectedValueOnce(new Error('Connection failed'))

      const result = await repository.getServiceStatus()

      expect(result).toEqual({
        enabled: true,
        configured: true,
        baseUrl: 'http://fee-service.local',
        message: 'Fee service is not responding'
      })
    })
  })
})
