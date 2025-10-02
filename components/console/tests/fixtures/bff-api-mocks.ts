import { Page, Request } from '@playwright/test'
import { testDataFactory } from './test-data.factory'

/**
 * Captured request information
 */
interface CapturedRequest {
  method: string
  url: string
  headers: Record<string, string>
  body: any
  timestamp: number
}

/**
 * Error scenario types
 */
export type ErrorScenarioType =
  | 'badRequest'
  | 'unauthorized'
  | 'forbidden'
  | 'notFound'
  | 'conflict'
  | 'serverError'
  | 'timeout'

/**
 * BFF API Mock Service
 * Provides comprehensive mocking for BFF and backend API calls
 * Captures requests for validation and testing
 */
export class BffApiMockService {
  private page: Page
  private capturedRequests: Map<string, CapturedRequest[]> = new Map()
  private mockEnabled: boolean = true

  constructor(page: Page) {
    this.page = page
  }

  /**
   * Setup all default mocks for common endpoints
   */
  async setupAllMocks(): Promise<void> {
    await this.mockOrganizations()
    await this.mockLedgers()
    await this.mockAccounts()
    await this.mockBalances()
    await this.mockAssets()
    await this.mockTransactions()
    await this.mockPortfolios()
    await this.mockSegments()
    await this.mockAccountTypes()
    await this.mockTransactionRoutes()
    await this.mockOperationRoutes()
  }

  /**
   * Mock Organizations API endpoints
   */
  async mockOrganizations(): Promise<void> {
    await this.page.route('**/api/organizations**', async (route) => {
      await this.captureRequest('/api/organizations', route.request())

      const method = route.request().method()

      if (method === 'POST') {
        await route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify({
            data: testDataFactory.organization()
          })
        })
      } else if (method === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: testDataFactory.list(testDataFactory.organization, 5),
            pagination: {
              page: 1,
              pageSize: 10,
              total: 5
            }
          })
        })
      } else if (method === 'PUT' || method === 'PATCH') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: testDataFactory.organization()
          })
        })
      } else if (method === 'DELETE') {
        await route.fulfill({
          status: 204
        })
      }
    })
  }

  /**
   * Mock Ledgers API endpoints
   */
  async mockLedgers(): Promise<void> {
    await this.page.route(
      '**/api/organizations/*/ledgers**',
      async (route) => {
        await this.captureRequest('/api/ledgers', route.request())

        const method = route.request().method()

        if (method === 'POST') {
          await route.fulfill({
            status: 201,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.ledger()
            })
          })
        } else if (method === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.list(testDataFactory.ledger, 10),
              pagination: {
                page: 1,
                pageSize: 10,
                total: 10
              }
            })
          })
        } else if (method === 'PUT' || method === 'PATCH') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.ledger()
            })
          })
        } else if (method === 'DELETE') {
          await route.fulfill({
            status: 204
          })
        }
      }
    )
  }

  /**
   * Mock Accounts API endpoints
   */
  async mockAccounts(): Promise<void> {
    await this.page.route(
      '**/api/organizations/*/ledgers/*/accounts**',
      async (route) => {
        await this.captureRequest('/api/accounts', route.request())

        const method = route.request().method()

        if (method === 'POST') {
          await route.fulfill({
            status: 201,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.account()
            })
          })
        } else if (method === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.list(testDataFactory.account, 10)
            })
          })
        } else if (method === 'PUT' || method === 'PATCH') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.account()
            })
          })
        } else if (method === 'DELETE') {
          await route.fulfill({
            status: 204
          })
        }
      }
    )
  }

  /**
   * Mock Balances API endpoints
   */
  async mockBalances(): Promise<void> {
    await this.page.route(
      '**/api/organizations/*/ledgers/*/accounts/*/balances**',
      async (route) => {
        await this.captureRequest('/api/balances', route.request())

        const method = route.request().method()

        if (method === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              items: testDataFactory.list(testDataFactory.balance, 3)
            })
          })
        }
      }
    )
  }

  /**
   * Mock Assets API endpoints
   */
  async mockAssets(): Promise<void> {
    await this.page.route(
      '**/api/organizations/*/ledgers/*/assets**',
      async (route) => {
        await this.captureRequest('/api/assets', route.request())

        const method = route.request().method()

        if (method === 'POST') {
          await route.fulfill({
            status: 201,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.asset()
            })
          })
        } else if (method === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.list(testDataFactory.asset, 10)
            })
          })
        } else if (method === 'PUT' || method === 'PATCH') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.asset()
            })
          })
        } else if (method === 'DELETE') {
          await route.fulfill({
            status: 204
          })
        }
      }
    )
  }

  /**
   * Mock Transactions API endpoints
   */
  async mockTransactions(): Promise<void> {
    await this.page.route(
      '**/api/organizations/*/ledgers/*/transactions**',
      async (route) => {
        await this.captureRequest('/api/transactions', route.request())

        const method = route.request().method()

        if (method === 'POST') {
          await route.fulfill({
            status: 201,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.transaction()
            })
          })
        } else if (method === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.list(testDataFactory.transaction, 10)
            })
          })
        }
      }
    )
  }

  /**
   * Mock Portfolios API endpoints
   */
  async mockPortfolios(): Promise<void> {
    await this.page.route(
      '**/api/organizations/*/ledgers/*/portfolios**',
      async (route) => {
        await this.captureRequest('/api/portfolios', route.request())

        const method = route.request().method()

        if (method === 'POST') {
          await route.fulfill({
            status: 201,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.portfolio()
            })
          })
        } else if (method === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.list(testDataFactory.portfolio, 10)
            })
          })
        } else if (method === 'DELETE') {
          await route.fulfill({
            status: 204
          })
        }
      }
    )
  }

  /**
   * Mock Segments API endpoints
   */
  async mockSegments(): Promise<void> {
    await this.page.route(
      '**/api/organizations/*/ledgers/*/segments**',
      async (route) => {
        await this.captureRequest('/api/segments', route.request())

        const method = route.request().method()

        if (method === 'POST') {
          await route.fulfill({
            status: 201,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.segment()
            })
          })
        } else if (method === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.list(testDataFactory.segment, 10)
            })
          })
        } else if (method === 'DELETE') {
          await route.fulfill({
            status: 204
          })
        }
      }
    )
  }

  /**
   * Mock Account Types API endpoints
   */
  async mockAccountTypes(): Promise<void> {
    await this.page.route(
      '**/api/organizations/*/ledgers/*/account-types**',
      async (route) => {
        await this.captureRequest('/api/account-types', route.request())

        const method = route.request().method()

        if (method === 'POST') {
          await route.fulfill({
            status: 201,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.accountType()
            })
          })
        } else if (method === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.list(testDataFactory.accountType, 10)
            })
          })
        } else if (method === 'DELETE') {
          await route.fulfill({
            status: 204
          })
        }
      }
    )
  }

  /**
   * Mock Transaction Routes API endpoints
   */
  async mockTransactionRoutes(): Promise<void> {
    await this.page.route(
      '**/api/organizations/*/ledgers/*/transaction-routes**',
      async (route) => {
        await this.captureRequest('/api/transaction-routes', route.request())

        const method = route.request().method()

        if (method === 'POST') {
          await route.fulfill({
            status: 201,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.transactionRoute()
            })
          })
        } else if (method === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.list(testDataFactory.transactionRoute, 10)
            })
          })
        } else if (method === 'DELETE') {
          await route.fulfill({
            status: 204
          })
        }
      }
    )
  }

  /**
   * Mock Operation Routes API endpoints
   */
  async mockOperationRoutes(): Promise<void> {
    await this.page.route(
      '**/api/organizations/*/ledgers/*/operation-routes**',
      async (route) => {
        await this.captureRequest('/api/operation-routes', route.request())

        const method = route.request().method()

        if (method === 'POST') {
          await route.fulfill({
            status: 201,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.operationRoute()
            })
          })
        } else if (method === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              data: testDataFactory.list(testDataFactory.operationRoute, 10)
            })
          })
        } else if (method === 'DELETE') {
          await route.fulfill({
            status: 204
          })
        }
      }
    )
  }

  /**
   * Setup error scenario for specific endpoint
   */
  async setupErrorScenario(
    errorType: ErrorScenarioType,
    endpoint: string
  ): Promise<void> {
    const errorResponses: Record<
      ErrorScenarioType,
      { status: number; body: any }
    > = {
      badRequest: {
        status: 400,
        body: { error: 'Bad Request', message: 'Invalid input data' }
      },
      unauthorized: {
        status: 401,
        body: { error: 'Unauthorized', message: 'Authentication required' }
      },
      forbidden: {
        status: 403,
        body: {
          error: 'Forbidden',
          message: 'You do not have permission to perform this action'
        }
      },
      notFound: {
        status: 404,
        body: { error: 'Not Found', message: 'Resource not found' }
      },
      conflict: {
        status: 409,
        body: { error: 'Conflict', message: 'Resource already exists' }
      },
      serverError: {
        status: 500,
        body: {
          error: 'Internal Server Error',
          message: 'An unexpected error occurred'
        }
      },
      timeout: {
        status: 408,
        body: { error: 'Request Timeout', message: 'Request timed out' }
      }
    }

    const response = errorResponses[errorType]

    await this.page.route(`**${endpoint}**`, async (route) => {
      await this.captureRequest(endpoint, route.request())

      // Simulate timeout delay
      if (errorType === 'timeout') {
        await this.page.waitForTimeout(5000)
      }

      await route.fulfill({
        status: response.status,
        contentType: 'application/json',
        body: JSON.stringify(response.body)
      })
    })
  }

  /**
   * Capture request for validation
   */
  private async captureRequest(
    endpoint: string,
    request: Request
  ): Promise<void> {
    if (!this.capturedRequests.has(endpoint)) {
      this.capturedRequests.set(endpoint, [])
    }

    const captured: CapturedRequest = {
      method: request.method(),
      url: request.url(),
      headers: await request.allHeaders(),
      body: request.postDataJSON(),
      timestamp: Date.now()
    }

    this.capturedRequests.get(endpoint)!.push(captured)
  }

  /**
   * Get captured requests for an endpoint
   */
  getCapturedRequests(endpoint: string): CapturedRequest[] {
    return this.capturedRequests.get(endpoint) || []
  }

  /**
   * Get all captured requests
   */
  getAllCapturedRequests(): Map<string, CapturedRequest[]> {
    return this.capturedRequests
  }

  /**
   * Clear captured requests
   */
  clearCapturedRequests(): void {
    this.capturedRequests.clear()
  }

  /**
   * Clear captured requests for specific endpoint
   */
  clearEndpointRequests(endpoint: string): void {
    this.capturedRequests.delete(endpoint)
  }

  /**
   * Disable all mocks (fallback to real API)
   */
  disableMocks(): void {
    this.mockEnabled = false
  }

  /**
   * Enable mocks
   */
  enableMocks(): void {
    this.mockEnabled = true
  }
}
