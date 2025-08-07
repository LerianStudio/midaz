import { NextFetchEvent, NextRequest } from 'next/server'
import { MiddlewareHandler, NextHandler, RouteHandler } from './types'

export function applyMiddleware(
  middlewares: MiddlewareHandler[],
  action: RouteHandler,
  responseMiddlewares?: MiddlewareHandler[]
) {
  return async (req: NextRequest, event: NextFetchEvent) => {
    let i = 0

    const next: NextHandler = async (err: any) => {
      if (err != null) {
        return await action(req, event)
      }

      if (i >= middlewares.length) {
        return await action(req, event)
      }

      const layer = middlewares[i++]
      try {
        return await layer(req, event, next)
      } catch (error) {
        return await next(error)
      }
    }

    const response = await next()

    if (!responseMiddlewares) {
      return response
    }

    let j = 0
    const nextResponse: NextHandler = async (err: any) => {
      if (err != null) {
        return response
      }

      if (j >= responseMiddlewares.length) {
        return response
      }

      const layer = responseMiddlewares[j++]
      try {
        return await layer(req, event, nextResponse, response)
      } catch (error) {
        return await nextResponse(error)
      }
    }

    return await nextResponse()
  }
}
