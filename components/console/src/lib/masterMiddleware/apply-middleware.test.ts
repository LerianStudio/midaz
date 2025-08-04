/**
 * @jest-environment node
 */

// Mock Next.js server utilities
jest.mock('next/server', () => ({
  NextRequest: jest.fn().mockImplementation((url) => ({
    url,
    method: 'GET',
    headers: new Map()
  })),
  NextResponse: Object.assign(
    jest.fn().mockImplementation((body, init) => ({
      status: init?.status || 200,
      body
    })),
    {
      next: jest.fn().mockReturnValue({ status: 200 }),
      json: jest.fn().mockReturnValue({ status: 200 })
    }
  ),
  NextFetchEvent: jest.fn()
}))

// Mock console.error to capture error logs
const mockConsoleError = jest
  .spyOn(console, 'error')
  .mockImplementation(() => {})

import { NextFetchEvent, NextRequest, NextResponse } from 'next/server'
import { applyMiddleware } from './apply-middleware'
import { MiddlewareHandler, RouteHandler } from './types'

describe('applyMiddleware', () => {
  let mockRequest: NextRequest
  let mockEvent: NextFetchEvent

  beforeEach(() => {
    mockRequest = new NextRequest('https://example.com')
    mockEvent = {} as NextFetchEvent
    mockConsoleError.mockClear()
  })

  afterAll(() => {
    mockConsoleError.mockRestore()
  })

  it('should execute a single middleware', async () => {
    const middlewareExecuted = jest.fn()
    const middleware: MiddlewareHandler = async (req, event, next) => {
      middlewareExecuted()
      return await next()
    }

    const action: RouteHandler = () => NextResponse.next()

    const handler = applyMiddleware([middleware], action)
    await handler(mockRequest, mockEvent)

    expect(middlewareExecuted).toHaveBeenCalledTimes(1)
  })

  it('should execute multiple middlewares in sequence', async () => {
    const sequence: number[] = []
    const middleware1: MiddlewareHandler = async (req, event, next) => {
      sequence.push(1)
      return await next()
    }
    const middleware2: MiddlewareHandler = async (req, event, next) => {
      sequence.push(2)
      return await next()
    }

    const action: RouteHandler = () => {
      sequence.push(3)
      return NextResponse.next()
    }

    const handler = applyMiddleware([middleware1, middleware2], action)
    await handler(mockRequest, mockEvent)

    expect(sequence).toEqual([1, 2, 3])
  })

  it('should handle errors in middleware', async () => {
    const errorMiddleware: MiddlewareHandler = async () => {
      throw new Error('Middleware error')
    }

    const action: RouteHandler = () => NextResponse.next()
    const handler = applyMiddleware([errorMiddleware], action)

    const result = await handler(mockRequest, mockEvent)

    // The action should still be called even when middleware throws an error
    expect(result).toBeDefined()
  })

  it('should execute response middlewares after action', async () => {
    const sequence: number[] = []

    const responseMiddleware: MiddlewareHandler = async (
      req,
      event,
      next,
      response
    ) => {
      sequence.push(3)
      expect(response).toBeDefined()
      return next()
    }

    const action: RouteHandler = () => {
      sequence.push(2)
      return NextResponse('test')
    }

    const handler = applyMiddleware([], action, [responseMiddleware])
    await handler(mockRequest, mockEvent)

    expect(sequence).toEqual([2, 3])
  })

  it('should work with empty middleware arrays', async () => {
    const actionExecuted = jest.fn()
    const action: RouteHandler = () => {
      actionExecuted()
      return NextResponse.next()
    }

    const handler = applyMiddleware([], action)
    await handler(mockRequest, mockEvent)

    expect(actionExecuted).toHaveBeenCalledTimes(1)
  })

  it('should handle errors in response middlewares', async () => {
    const errorResponseMiddleware: MiddlewareHandler = async () => {
      throw new Error('Response middleware error')
    }

    const action: RouteHandler = () => NextResponse.next()
    const handler = applyMiddleware([], action, [errorResponseMiddleware])

    const result = await handler(mockRequest, mockEvent)

    // Should return the original response when response middleware throws an error
    expect(result).toBeDefined()
  })

  it('should execute action handler when middleware passes error', async () => {
    const actionExecuted = jest.fn()
    const errorMiddleware: MiddlewareHandler = async (req, event, next) => {
      return await next(new Error('Test error'))
    }

    const action: RouteHandler = () => {
      actionExecuted()
      return NextResponse.next()
    }

    const handler = applyMiddleware([errorMiddleware], action)
    await handler(mockRequest, mockEvent)

    expect(actionExecuted).toHaveBeenCalledTimes(1)
  })

  it('should handle complex middleware chain with both request and response middlewares', async () => {
    const sequence: number[] = []
    const requestMiddleware1: MiddlewareHandler = async (req, event, next) => {
      sequence.push(1)
      return await next()
    }
    const requestMiddleware2: MiddlewareHandler = async (req, event, next) => {
      sequence.push(2)
      return await next()
    }
    const responseMiddleware1: MiddlewareHandler = async (
      req,
      event,
      next,
      _response
    ) => {
      sequence.push(4)
      return next()
    }
    const responseMiddleware2: MiddlewareHandler = async (
      req,
      event,
      next,
      _response
    ) => {
      sequence.push(5)
      return next()
    }

    const action: RouteHandler = () => {
      sequence.push(3)
      return NextResponse.next()
    }

    const handler = applyMiddleware(
      [requestMiddleware1, requestMiddleware2],
      action,
      [responseMiddleware1, responseMiddleware2]
    )
    await handler(mockRequest, mockEvent)

    expect(sequence).toEqual([1, 2, 3, 4, 5])
  })

  it('should pass modified request through middleware chain', async () => {
    const testHeader = 'test-header'
    const testValue = 'test-value'

    const middleware1: MiddlewareHandler = async (req, event, next) => {
      const newRequest = new NextRequest(req, {
        headers: new Headers(req.headers)
      })
      newRequest.headers.set(testHeader, testValue)

      Object.defineProperty(req, 'headers', {
        get: () => newRequest.headers
      })

      return await next()
    }

    const middleware2: MiddlewareHandler = async (req, event, next) => {
      expect(req.headers.get(testHeader)).toBe(testValue)
      return await next()
    }

    const action: RouteHandler = (request: NextRequest) => {
      expect(request.headers.get(testHeader)).toBe(testValue)
      return NextResponse.next()
    }

    const handler = applyMiddleware([middleware1, middleware2], action)
    await handler(mockRequest, mockEvent)
  })
})
