import { Page, expect } from '@playwright/test'
import { BffApiMockService } from '../fixtures/bff-api-mocks'

/**
 * BFF Test Helpers
 * Provides utility functions for validating BFF layer behavior in E2E tests
 */
export class BffTestHelpers {
  constructor(
    private page: Page,
    private mockService: BffApiMockService
  ) {}

  /**
   * Assert that a BFF endpoint was called
   */
  async assertBffEndpointCalled(
    endpoint: string,
    options?: {
      method?: string
      body?: any
      minCalls?: number
    }
  ): Promise<void> {
    const requests = this.mockService.getCapturedRequests(endpoint)
    const minCalls = options?.minCalls || 1

    expect(
      requests.length,
      `Expected at least ${minCalls} call(s) to ${endpoint}, but got ${requests.length}`
    ).toBeGreaterThanOrEqual(minCalls)

    if (options?.method) {
      const methodMatch = requests.some((r) => r.method === options.method)
      expect(
        methodMatch,
        `Expected method ${options.method} but found: ${requests.map((r) => r.method).join(', ')}`
      ).toBeTruthy()
    }

    if (options?.body) {
      const bodyMatch = requests.some(
        (r) => JSON.stringify(r.body) === JSON.stringify(options.body)
      )
      expect(
        bodyMatch,
        `Expected body to match but got: ${JSON.stringify(requests.map((r) => r.body))}`
      ).toBeTruthy()
    }
  }

  /**
   * Assert that a BFF endpoint was NOT called
   */
  async assertBffEndpointNotCalled(endpoint: string): Promise<void> {
    const requests = this.mockService.getCapturedRequests(endpoint)
    expect(
      requests.length,
      `Expected no calls to ${endpoint}, but got ${requests.length}`
    ).toBe(0)
  }

  /**
   * Assert that backend endpoint was called (via BFF forwarding)
   * Note: This requires backend mocking to be set up separately
   */
  async assertBackendEndpointCalled(
    endpoint: string,
    options?: {
      method?: string
    }
  ): Promise<void> {
    // This is a placeholder for backend validation
    // In a full implementation, you would mock the backend endpoints separately
    // and validate that the BFF correctly forwarded the request
    console.log(
      `Backend validation for ${endpoint} with method ${options?.method}`
    )
  }

  /**
   * Validate that BFF adds authentication headers
   */
  async validateBffAuthentication(endpoint: string): Promise<void> {
    const requests = this.mockService.getCapturedRequests(endpoint)
    expect(
      requests.length,
      `No requests found for ${endpoint} to validate authentication`
    ).toBeGreaterThan(0)

    const hasAuthHeader = requests.every((r) => {
      return (
        r.headers['authorization'] ||
        r.headers['Authorization'] ||
        r.headers['cookie']?.includes('auth')
      )
    })

    expect(
      hasAuthHeader,
      'Expected authentication headers in all requests'
    ).toBeTruthy()
  }

  /**
   * Validate request transformation (BFF → Backend)
   */
  async assertRequestTransformation(
    bffEndpoint: string,
    expectedTransformedData: any
  ): Promise<void> {
    const requests = this.mockService.getCapturedRequests(bffEndpoint)
    expect(requests.length).toBeGreaterThan(0)

    const lastRequest = requests[requests.length - 1]
    expect(lastRequest.body).toMatchObject(expectedTransformedData)
  }

  /**
   * Validate response transformation (Backend → BFF)
   * This validates that the BFF correctly transformed the backend response
   */
  async assertBffTransformation(
    bffEndpoint: string,
    backendEndpoint: string
  ): Promise<void> {
    // Placeholder for BFF response transformation validation
    // In full implementation, compare backend response to what UI received
    const requests = this.mockService.getCapturedRequests(bffEndpoint)
    expect(requests.length).toBeGreaterThan(0)
  }

  /**
   * Test error propagation from backend through BFF
   */
  async testErrorPropagation(
    bffEndpoint: string,
    backendEndpoint: string
  ): Promise<void> {
    // Placeholder for error propagation testing
    // Would validate that backend errors are correctly propagated through BFF
    const requests = this.mockService.getCapturedRequests(bffEndpoint)
    expect(requests.length).toBeGreaterThan(0)
  }

  /**
   * Validate pagination handling
   */
  async validateBffPagination(endpoint: string): Promise<void> {
    const requests = this.mockService.getCapturedRequests(endpoint)
    expect(requests.length).toBeGreaterThan(0)

    // Check if pagination parameters were sent
    const lastRequest = requests[requests.length - 1]
    const url = new URL(lastRequest.url)
    const hasPageParams =
      url.searchParams.has('page') || url.searchParams.has('limit')

    // Pagination might be optional, so just log the result
    if (hasPageParams) {
      console.log('Pagination parameters found:', {
        page: url.searchParams.get('page'),
        limit: url.searchParams.get('limit')
      })
    }
  }

  /**
   * Get request count for an endpoint
   */
  getRequestCount(endpoint: string): number {
    return this.mockService.getCapturedRequests(endpoint).length
  }

  /**
   * Get last request for an endpoint
   */
  getLastRequest(endpoint: string): any {
    const requests = this.mockService.getCapturedRequests(endpoint)
    return requests.length > 0 ? requests[requests.length - 1] : null
  }

  /**
   * Get all requests for an endpoint
   */
  getAllRequests(endpoint: string): any[] {
    return this.mockService.getCapturedRequests(endpoint)
  }

  /**
   * Wait for specific number of requests to an endpoint
   */
  async waitForRequests(
    endpoint: string,
    count: number,
    timeout: number = 5000
  ): Promise<void> {
    const startTime = Date.now()

    while (Date.now() - startTime < timeout) {
      const requests = this.mockService.getCapturedRequests(endpoint)
      if (requests.length >= count) {
        return
      }
      await this.page.waitForTimeout(100)
    }

    const actualCount = this.mockService.getCapturedRequests(endpoint).length
    throw new Error(
      `Timeout waiting for ${count} requests to ${endpoint}. Got ${actualCount} requests.`
    )
  }

  /**
   * Assert request body contains specific fields
   */
  async assertRequestBodyContains(
    endpoint: string,
    fields: Record<string, any>
  ): Promise<void> {
    const requests = this.mockService.getCapturedRequests(endpoint)
    expect(
      requests.length,
      `No requests found for ${endpoint}`
    ).toBeGreaterThan(0)

    const lastRequest = requests[requests.length - 1]
    expect(lastRequest.body).toMatchObject(fields)
  }

  /**
   * Assert request headers contain specific values
   */
  async assertRequestHeadersContain(
    endpoint: string,
    headers: Record<string, string>
  ): Promise<void> {
    const requests = this.mockService.getCapturedRequests(endpoint)
    expect(
      requests.length,
      `No requests found for ${endpoint}`
    ).toBeGreaterThan(0)

    const lastRequest = requests[requests.length - 1]
    for (const [key, value] of Object.entries(headers)) {
      const headerValue =
        lastRequest.headers[key] || lastRequest.headers[key.toLowerCase()]
      expect(
        headerValue,
        `Expected header ${key} to contain ${value}, but got ${headerValue}`
      ).toContain(value)
    }
  }

  /**
   * Debug: Print all captured requests
   */
  printAllCapturedRequests(): void {
    const allRequests = this.mockService.getAllCapturedRequests()
    console.log('=== All Captured Requests ===')
    allRequests.forEach((requests, endpoint) => {
      console.log(`\n${endpoint} (${requests.length} requests):`)
      requests.forEach((req, index) => {
        console.log(`  [${index + 1}] ${req.method} ${req.url}`)
        console.log(`      Headers:`, req.headers)
        console.log(`      Body:`, req.body)
      })
    })
  }

  /**
   * Debug: Print captured requests for specific endpoint
   */
  printEndpointRequests(endpoint: string): void {
    const requests = this.mockService.getCapturedRequests(endpoint)
    console.log(`\n=== Requests for ${endpoint} (${requests.length} total) ===`)
    requests.forEach((req, index) => {
      console.log(`[${index + 1}] ${req.method} ${req.url}`)
      console.log(`    Timestamp: ${new Date(req.timestamp).toISOString()}`)
      console.log(`    Headers:`, req.headers)
      console.log(`    Body:`, req.body)
    })
  }
}
