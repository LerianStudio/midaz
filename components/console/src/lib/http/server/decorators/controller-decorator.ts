import { NextResponse } from 'next/server'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'

/**
 * A class decorator that wraps all methods in the class with error handling.
 *
 * @returns
 */
export function Controller(): ClassDecorator {
  return function (target: Function) {
    const prototype = target.prototype
    const methodNames = Object.getOwnPropertyNames(prototype).filter(
      (name) => typeof prototype[name] === 'function' && name !== 'constructor'
    )

    for (const methodName of methodNames) {
      const originalMethod = prototype[methodName]

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
