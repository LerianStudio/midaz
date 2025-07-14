/**
 * @jest-environment node
 */

import { NextFetchEvent, NextRequest, NextResponse } from 'next/server'
import { applyMiddleware } from './apply-middleware'
import { MiddlewareHandler, RouteHandler } from './types'

describe('applyMiddleware', () => {
  let mockRequest: NextRequest
  let mockEvent: NextFetchEvent

  beforeEach(() => {
    mockRequest = new NextRequest(new Request('https://example.com'))
    mockEvent = {} as NextFetchEvent
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

    const consoleSpy = jest.spyOn(console, 'log').mockImplementation()
    await handler(mockRequest, mockEvent)

    expect(consoleSpy).toHaveBeenCalledWith(expect.any(Error))
    consoleSpy.mockRestore()
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
      return new NextResponse('test')
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

    const consoleSpy = jest.spyOn(console, 'log').mockImplementation()
    await handler(mockRequest, mockEvent)

    expect(consoleSpy).toHaveBeenCalledWith(expect.any(Error))
    consoleSpy.mockRestore()
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
      response
    ) => {
      sequence.push(4)
      return next()
    }
    const responseMiddleware2: MiddlewareHandler = async (
      req,
      event,
      next,
      response
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
      // Create a new request with the added header
      const newRequest = new NextRequest(req, {
        headers: new Headers(req.headers)
      })
      newRequest.headers.set(testHeader, testValue)

      // Mock replacing the request in the chain
      Object.defineProperty(req, 'headers', {
        get: () => newRequest.headers
      })

      return await next()
    }

    const middleware2: MiddlewareHandler = async (req, event, next) => {
      // Verify the header is present
      expect(req.headers.get(testHeader)).toBe(testValue)
      return await next()
    }

    const action: RouteHandler = (request: NextRequest) => {
      // Verify the header is still present at the action
      expect(request.headers.get(testHeader)).toBe(testValue)
      return NextResponse.next()
    }

    const handler = applyMiddleware([middleware1, middleware2], action)
    await handler(mockRequest, mockEvent)
  })
})
