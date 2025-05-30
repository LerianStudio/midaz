import { NextResponse } from 'next/server'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'

/**
 * A class decorator that wraps all methods in the class with error handling.
 *
 * @returns
 */
export function Controller(): ClassDecorator {
  return function (target: Function) {
    // Get all method names from the prototype
    const prototype = target.prototype
    const methodNames = Object.getOwnPropertyNames(prototype).filter(
      (name) => typeof prototype[name] === 'function' && name !== 'constructor'
    )

    // Replace each method with a wrapped version
    for (const methodName of methodNames) {
      const originalMethod = prototype[methodName]

      // Replace with wrapped method
      prototype[methodName] = async function (...args: any[]) {
        try {
          return await originalMethod.apply(this, args)
        } catch (error: any) {
          const { message, status } = await apiErrorHandler(error)

          return NextResponse.json({ message }, { status })
        }
      }
    }
  }
}
