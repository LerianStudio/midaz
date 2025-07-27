import { NextRequest } from 'next/server'
import { MiddlewareHandler, NextHandler, RouteHandler } from './types'

/**
 * Applies an array of middleware functions to a route handler.
 * Implements a middleware chain pattern similar to Express.js middleware system.
 *
 * @param middlewares - Array of middleware functions to be executed in sequence
 * @param action - Final route handler to be executed after all middleware
 * @returns A function that handles the request through the middleware chain
 *
 * @example
 * const handler = applyMiddleware([
 *   authMiddleware,
 *   validationMiddleware
 * ], finalHandler);
 */
export function applyMiddleware(
  middlewares: MiddlewareHandler[],
  action: RouteHandler
) {
  return async (req: NextRequest, ...args: any) => {
    let i = 0

    /**
     * Internal function that manages the middleware chain execution
     * @param err - Optional error object passed from previous middleware
     * @returns Promise containing the result of the middleware chain
     */
    const next: NextHandler = async (err) => {
      const localReq = req.clone() as NextRequest

      if (err != null) {
        return await action(req, ...args)
      }

      if (i >= middlewares.length) {
        return await action(req, ...args)
      }

      const layer = middlewares[i++]
      try {
        return await layer(localReq, next)
      } catch (error) {
        console.error(error)
        return await next(error)
      }
    }

    return await next()
  }
}
