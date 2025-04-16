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
    // Index to keep track of current middleware
    let i = 0

    /**
     * Internal function that manages the middleware chain execution
     * @param err - Optional error object passed from previous middleware
     * @returns Promise containing the result of the middleware chain
     */
    const next: NextHandler = async (err) => {
      // Clone request to prevent mutations between middleware
      const localReq = req.clone() as NextRequest

      // If there's an error or we've run all middleware, execute final handler
      if (err != null) {
        return await action(req, ...args)
      }

      if (i >= middlewares.length) {
        return await action(req, ...args)
      }

      // Get next middleware in the chain
      const layer = middlewares[i++]
      try {
        // Execute current middleware with cloned request and next function
        return await layer(localReq, next)
      } catch (error) {
        // Log any errors and continue chain with error
        console.error(error)
        return await next(error)
      }
    }

    // Start the middleware chain
    return await next()
  }
}
