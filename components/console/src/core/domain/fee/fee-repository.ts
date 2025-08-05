import {
  FeeCalculationRequest,
  FeeCalculationResponse,
  FeePackageDetails,
  FeeCalculationContext
} from './fee-types'

/**
 * Token for FeeRepository dependency injection
 */
export const FeeRepositoryToken = Symbol('FeeRepository')

/**
 * Repository interface for fee-related operations
 * Abstracts the fee service communication layer
 */
export interface FeeRepository {
  /**
   * Calculate fees for a transaction
   * @param request The fee calculation request
   * @param context Additional context (organization, ledger, etc.)
   * @returns The fee calculation response
   * @throws FeeServiceError if the calculation fails
   */
  calculateFees(
    request: FeeCalculationRequest,
    context: FeeCalculationContext
  ): Promise<FeeCalculationResponse>

  /**
   * Fetch fee package details
   * @param packageId The fee package identifier
   * @param context Additional context
   * @returns The fee package details or null if not found
   */
  getFeePackage(
    packageId: string,
    context: FeeCalculationContext
  ): Promise<FeePackageDetails | null>

  /**
   * Validate if fee service is available
   * @returns true if the service is healthy
   */
  isHealthy(): Promise<boolean>

  /**
   * Get fee service configuration status
   * @returns Configuration status and details
   */
  getServiceStatus(): Promise<{
    enabled: boolean
    configured: boolean
    baseUrl?: string
    message?: string
  }>
}
