import { LoggerAggregator } from '@/core/infrastructure/logger/logger-aggregator'
import { getServerSession } from 'next-auth'
import { MidazRequestContext } from '../logger/decorators/midaz-id'
import { OtelTracerProvider } from '../observability/otel-tracer-provider'
import { HttpFetchUtils } from './http-fetch-utils'
import { handleMidazError } from './midaz-error-handler'
import { HttpMethods } from '@/lib/http'

jest.mock('next-auth', () => ({
  getServerSession: jest.fn()
}))

jest.mock('./midaz-error-handler', () => ({
  handleMidazError: jest.fn(() => {
    throw new Error('Error occurred')
  })
}))

jest.mock('../next-auth/next-auth-provider')
jest.mock('../logger/decorators/midaz-id')

describe('MidazHttpFetchUtils', () => {
  let midazHttpFetchUtils: HttpFetchUtils
  let midazRequestContext: MidazRequestContext
  let midazLogger: LoggerAggregator
  let otelTracerProvider: OtelTracerProvider

  beforeEach(() => {
    midazRequestContext = new MidazRequestContext()

    midazLogger = {
      error: jest.fn(),
      info: jest.fn()
    } as unknown as LoggerAggregator

    otelTracerProvider = {
      startCustomSpan: jest.fn().mockImplementation(() => {
        return {
          setAttributes: jest.fn().mockReturnThis(),
          setStatus: jest.fn().mockReturnThis()
        }
      }),
      endCustomSpan: jest.fn()
    } as unknown as OtelTracerProvider

    midazHttpFetchUtils = new HttpFetchUtils(
      midazRequestContext,
      midazLogger,
      otelTracerProvider
    )
  })

  afterEach(() => {
    jest.clearAllMocks()
  })

  it('should make a successful fetch request', async () => {
    const mockResponse = { data: 'test' }
    const mockFetch = jest.fn().mockResolvedValue({
      ok: true,
      json: jest.fn().mockResolvedValue(mockResponse),
      body: true,
      status: 200
    })
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: jest.fn().mockResolvedValue(mockResponse),
      body: true,
      status: 200
    })
    ;(getServerSession as jest.Mock).mockResolvedValue({
      user: { access_token: 'test-token' }
    })

    const result = await midazHttpFetchUtils.httpMidazFetch({
      url: 'https://api.example.com/test',
      method: HttpMethods.GET,
      headers: {
        'Custom-Header': 'CustomValue'
      }
    })

    expect(result).toEqual(mockResponse)
    expect(midazLogger.info).toHaveBeenCalledWith('[INFO] - httpFetch ', {
      url: 'https://api.example.com/test',
      method: 'GET',
      status: 200
    })
  })

  it('should handle fetch request error', async () => {
    const mockErrorResponse = { error: 'test error' }
    const mockFetch = jest.fn().mockResolvedValue({
      ok: false,
      json: jest.fn().mockResolvedValue(mockErrorResponse),
      body: true,
      status: 400
    })
    global.fetch = mockFetch
    ;(getServerSession as jest.Mock).mockResolvedValue({
      user: { access_token: 'test-token' }
    })
    ;(handleMidazError as jest.Mock).mockImplementation(() => {
      throw new Error('Handled error')
    })

    await expect(
      midazHttpFetchUtils.httpMidazFetch({
        url: 'https://api.example.com/test',
        method: HttpMethods.GET
      })
    ).rejects.toThrow('Handled error')

    expect(midazLogger.error).toHaveBeenCalledWith('[ERROR] - httpFetch ', {
      url: 'https://api.example.com/test',
      method: 'GET',
      status: 400,
      response: mockErrorResponse
    })
  })

  it('should set the correct headers', async () => {
    const mockResponse = { data: 'test' }
    const mockFetch = jest.fn().mockResolvedValue({
      ok: true,
      json: jest.fn().mockResolvedValue(mockResponse),
      body: true,
      status: 200
    })
    global.fetch = mockFetch
    ;(getServerSession as jest.Mock).mockResolvedValue({
      user: { access_token: 'test-token' }
    })

    await midazHttpFetchUtils.httpMidazFetch({
      url: 'https://api.example.com/test',
      method: HttpMethods.GET,
      headers: {
        'Custom-Header': 'CustomValue',
        'X-Request-Id': 'test-request-id'
      }
    })

    if (process.env.PLUGIN_AUTH_ENABLED === 'true') {
      expect(mockFetch).toHaveBeenCalledWith('https://api.example.com/test', {
        method: HttpMethods.GET,
        body: undefined,
        headers: {
          'Custom-Header': 'CustomValue',
          'X-Request-Id': 'test-request-id',
          'Content-Type': 'application/json',
          Authorization: `test-token`
        }
      })
    } else {
      expect(mockFetch).toHaveBeenCalledWith('https://api.example.com/test', {
        method: HttpMethods.GET,
        body: undefined,
        headers: {
          'Custom-Header': 'CustomValue',
          'X-Request-Id': 'test-request-id',
          'Content-Type': 'application/json'
        }
      })
    }
  })

  it('should start and end a custom span', async () => {
    const mockResponse = { data: 'test' }
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: jest.fn().mockResolvedValue(mockResponse),
      body: true,
      status: 200
    })
    ;(getServerSession as jest.Mock).mockResolvedValue({
      user: { access_token: 'test-token' }
    })

    await midazHttpFetchUtils.httpMidazFetch({
      url: 'https://api.example.com/test',
      method: HttpMethods.GET
    })

    expect(otelTracerProvider.startCustomSpan).toHaveBeenCalledWith(
      'midaz-request'
    )
    expect(otelTracerProvider.endCustomSpan).toHaveBeenCalled()
  })
})
