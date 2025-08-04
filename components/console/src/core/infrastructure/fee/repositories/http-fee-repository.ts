import { injectable, inject } from 'inversify'
import { MidazHttpService } from '@/core/infrastructure/midaz/services/midaz-http-service'
import { FeeRepository } from '@/core/domain/fee/fee-repository'
import {
  FeeCalculationRequest,
  FeeCalculationResponse,
  FeePackageDetails,
  FeeCalculationContext,
  FeeServiceError,
  FeeServiceUnavailableError,
  FeeConfigurationError
} from '@/core/domain/fee/fee-types'
import { convertConsoleToFeeEngine } from '@/utils/console-to-fee-engine-converter'
import { transformFeeEngineResponse } from '@/utils/fee-engine-response-transformer'
import { CircuitBreaker } from '../utils/circuit-breaker'
import { RetryPolicy } from '../utils/retry-policy'
import { FeePackageCache } from '../cache/fee-package-cache'

/**
 * HTTP implementation of the FeeRepository
 * Handles communication with the fee engine service
 */
@injectable()
export class HttpFeeRepository implements FeeRepository {
  private circuitBreaker: CircuitBreaker
  private retryPolicy: RetryPolicy
  private packageCache: FeePackageCache

  constructor(
    @inject(MidazHttpService) private readonly httpService: MidazHttpService
  ) {
    // Initialize circuit breaker with default settings
    this.circuitBreaker = new CircuitBreaker({
      failureThreshold: 5,
      recoveryTimeout: 60000, // 1 minute
      monitoringPeriod: 120000 // 2 minutes
    })

    // Initialize retry policy
    this.retryPolicy = new RetryPolicy({
      maxRetries: 3,
      initialDelay: 1000,
      maxDelay: 10000,
      backoffMultiplier: 2
    })

    // Initialize cache
    this.packageCache = new FeePackageCache({
      ttl: 300000, // 5 minutes
      maxEntries: 100
    })
  }

  async calculateFees(
    request: FeeCalculationRequest,
    context: FeeCalculationContext
  ): Promise<FeeCalculationResponse> {
    const { organizationId, ledgerId, segmentId } = context

    // Check if fee service is enabled
    const status = await this.getServiceStatus()
    if (!status.enabled || !status.configured) {
      throw new FeeConfigurationError(
        status.message || 'Fee service is not properly configured'
      )
    }

    // Prepare the fee engine request
    const feeEngineRequest = {
      ledgerId,
      transaction: convertConsoleToFeeEngine(request.transaction as any),
      ...(segmentId && { segmentId })
    }

    try {
      // Execute with circuit breaker and retry
      const response = await this.circuitBreaker.execute(() =>
        this.retryPolicy.execute(async () => {
          const baseUrl = process.env.PLUGIN_FEES_PATH
          if (!baseUrl) {
            throw new FeeConfigurationError('Fee service URL not configured')
          }

          const defaults = await this.httpService['createDefaults']()

          const result = await this.httpService.post<any>(`${baseUrl}/fees`, {
            headers: {
              ...defaults.headers,
              'X-Organization-Id': organizationId
            },
            body: JSON.stringify(feeEngineRequest)
          })

          return result
        })
      )

      // Transform the response back to console format
      const transformedResponse = transformFeeEngineResponse(
        response,
        request.transaction
      )

      // Fetch package details if available
      const packageId = this.extractPackageId(transformedResponse)
      if (packageId) {
        try {
          const packageDetails = await this.getFeePackage(packageId, context)
          if (packageDetails && this.hasTransaction(transformedResponse)) {
            this.enrichResponseWithPackageDetails(
              transformedResponse,
              packageDetails
            )
          }
        } catch (error) {
          console.warn('Failed to fetch fee package details:', error)
        }
      }

      return this.mapToFeeCalculationResponse(transformedResponse)
    } catch (error: any) {
      if (error instanceof FeeServiceError) {
        throw error
      }

      if (this.circuitBreaker.isOpen()) {
        throw new FeeServiceUnavailableError(
          'Fee service is temporarily unavailable due to multiple failures'
        )
      }

      // Handle HTTP response errors from MidazHttpService
      if (error.response?.status) {
        const responseData = error.response.data || {}
        throw new FeeServiceError(
          responseData.error || 'Fee calculation failed',
          responseData.code || 'FEE_CALCULATION_ERROR',
          error.response.status,
          {
            status: error.response.status,
            code: responseData.code,
            title: responseData.title || responseData.error,
            details: responseData.details
          }
        )
      }

      throw new FeeServiceError(
        'Fee calculation failed',
        'FEE_CALCULATION_ERROR',
        500,
        error
      )
    }
  }

  async getFeePackage(
    packageId: string,
    context: FeeCalculationContext
  ): Promise<FeePackageDetails | null> {
    // Check cache first
    const cachedPackage = this.packageCache.get(packageId)
    if (cachedPackage) {
      return cachedPackage
    }

    const pluginUrl = process.env.NEXT_PUBLIC_PLUGIN_FEES_FRONTEND_URL
    if (!pluginUrl) {
      return null
    }

    try {
      const response = await this.retryPolicy.execute(async () => {
        const pluginUIBasePath =
          process.env.NEXT_PUBLIC_PLUGIN_UI_BASE_PATH || '/plugin-fees-ui'
        const url = `${pluginUrl}${pluginUIBasePath}/api/fees/packages/${packageId}`

        const result = await fetch(url, {
          headers: {
            'Content-Type': 'application/json',
            'X-Organization-Id': context.organizationId
          }
        })

        if (!result.ok) {
          if (result.status === 404) {
            return null
          }
          throw new Error(`Failed to fetch package: ${result.statusText}`)
        }

        return result.json()
      })

      if (response) {
        const packageDetails = this.mapToFeePackageDetails(response)
        this.packageCache.set(packageId, packageDetails)
        return packageDetails
      }

      return null
    } catch (error) {
      console.error('Error fetching fee package:', error)
      return null
    }
  }

  async isHealthy(): Promise<boolean> {
    try {
      const baseUrl = process.env.PLUGIN_FEES_PATH
      if (!baseUrl) {
        return false
      }

      const response = await this.httpService.get<any>(`${baseUrl}/health`)

      return response.status === 'healthy'
    } catch {
      return false
    }
  }

  async getServiceStatus(): Promise<{
    enabled: boolean
    configured: boolean
    baseUrl?: string
    message?: string
  }> {
    const enabled = process.env.NEXT_PUBLIC_PLUGIN_FEES_ENABLED === 'true'
    const baseUrl = process.env.PLUGIN_FEES_PATH

    if (!enabled) {
      return {
        enabled: false,
        configured: false,
        message: 'Fee service is disabled'
      }
    }

    if (!baseUrl) {
      return {
        enabled: true,
        configured: false,
        message: 'Fee service URL not configured'
      }
    }

    const healthy = await this.isHealthy()
    if (!healthy) {
      return {
        enabled: true,
        configured: true,
        baseUrl,
        message: 'Fee service is not responding'
      }
    }

    return {
      enabled: true,
      configured: true,
      baseUrl
    }
  }

  private enrichResponseWithPackageDetails(
    response: any,
    packageDetails: FeePackageDetails
  ): void {
    if (response.transaction && packageDetails.fees) {
      const feeRules = Object.entries(packageDetails.fees).map(
        ([feeId, fee]: [string, any]) => ({
          feeId,
          feeLabel: fee.feeLabel,
          isDeductibleFrom: fee.isDeductibleFrom || false,
          creditAccount: fee.creditAccount,
          priority: fee.priority,
          referenceAmount: fee.referenceAmount || 'originalAmount',
          applicationRule: fee.applicationRule || 'percentual',
          calculations: fee.calculations
        })
      )

      response.transaction.feeRules = feeRules
      response.transaction.isDeductibleFrom = feeRules.some(
        (rule) => rule.isDeductibleFrom === true
      )
    }
  }

  private mapToFeeCalculationResponse(response: any): FeeCalculationResponse {
    // Handle different response types
    if (response.feesApplied === false || response.hasNoFees) {
      return {
        success: true,
        feesApplied: false,
        message: response.message,
        transaction: response.transaction
      }
    }

    if (response.transaction) {
      return {
        success: true,
        feesApplied: true,
        transaction: response.transaction,
        packageId: response.transaction.metadata?.packageAppliedID,
        fees: response.transaction.feeRules?.map((rule: any) => ({
          feeId: rule.feeId,
          feeLabel: rule.feeLabel,
          calculatedAmount: rule.calculatedAmount || 0,
          isDeductibleFrom: rule.isDeductibleFrom,
          creditAccount: rule.creditAccount,
          priority: rule.priority,
          type: rule.applicationRule === 'percentual' ? 'percentage' : 'fixed'
        }))
      }
    }

    return {
      success: false,
      feesApplied: false,
      errors: [
        {
          code: 'UNKNOWN_RESPONSE_FORMAT',
          message: 'Unexpected response format from fee service',
          severity: 'error'
        }
      ]
    }
  }

  private mapToFeePackageDetails(data: any): FeePackageDetails {
    return {
      id: data.id,
      name: data.name,
      description: data.description,
      status: data.status || 'active',
      fees: data.fees || {},
      metadata: data.metadata,
      createdAt: new Date(data.createdAt || Date.now()),
      updatedAt: new Date(data.updatedAt || Date.now())
    }
  }

  private generateCorrelationId(): string {
    return `fee-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`
  }

  private extractPackageId(response: any): string | null {
    if (
      'transaction' in response &&
      response.transaction?.metadata?.packageAppliedID
    ) {
      return response.transaction.metadata.packageAppliedID
    }
    if ('packageId' in response) {
      return response.packageId
    }
    return null
  }

  private hasTransaction(response: any): boolean {
    return 'transaction' in response && response.transaction !== null
  }

  private mapToConsoleTransaction(transaction: any): any {
    // Map from FeeTransaction to CurrentConsoleTransaction format
    const sourceAccounts = transaction.send?.source?.from || []
    const destinationAccounts = transaction.send?.distribute?.to || []

    // Calculate total value from source accounts
    const totalValue = sourceAccounts.reduce((sum: number, account: any) => {
      return sum + parseFloat(account.amount?.value || '0')
    }, 0)

    return {
      description: transaction.description || 'Transaction',
      chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
      value: transaction.send?.value || totalValue.toString(),
      asset: transaction.send?.asset || 'USD',
      source: sourceAccounts.map((account: any) => ({
        accountAlias: account.accountAlias,
        value: account.amount?.value || '0',
        asset: account.amount?.asset || transaction.send?.asset || 'USD',
        metadata: account.metadata || {}
      })),
      destination: destinationAccounts.map((account: any) => ({
        accountAlias: account.accountAlias,
        value: account.amount?.value || '0',
        asset: account.amount?.asset || transaction.send?.asset || 'USD',
        metadata: account.metadata || {}
      })),
      metadata: {
        ...transaction.metadata,
        route: transaction.route
      }
    }
  }
}
